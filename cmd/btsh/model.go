package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Mode represents the shell state
type Mode int

const (
	ModeReady   Mode = iota // Accepting input
	ModeRunning             // Command executing, spinner visible
	ModeAction              // Action mode prompt
	ModeConfirm             // Awaiting y/N for action POST
)

// findQueueEntry tracks a resource to search and its display prefix
type findQueueEntry struct {
	path   string
	prefix string
}

// Completion menu styles
var (
	compSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(14))
	compNormalStyle   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))
)

// shellState holds mutable state shared between model and program.
type shellState struct {
	nav     *Navigator
	history *History

	// Scrape state
	scrapeQueue     []string
	scrapeVisited   map[string]bool
	scrapeDone      int
	scrapeTotal     int
	scrapeErrors    []string
	scrapeCancelled bool
	scrapeStart     time.Time
	spinnerLabel    string

	// Find state
	findQueue     []findQueueEntry
	findVisited   map[string]bool
	findPattern   *regexp.Regexp
	findResults   int
	findSearched  int
	findTotal     int
	findCancelled bool
	findStart     time.Time

	// Export state
	exportQueue     []string
	exportVisited   map[string]bool
	exportCollected map[string]json.RawMessage
	exportDone      int
	exportTotal     int
	exportErrors    []string
	exportCancelled bool
	exportStart     time.Time
	exportFilename  string

	// Track if we were in action mode before a command
	inActionMode bool

	// Action confirm state
	pendingAction *ActionInfo
	pendingBody   []byte
}

// model is the bubbletea model for the inline shell
type model struct {
	state   *shellState
	input   textinput.Model
	spinner spinner.Model
	mode    Mode

	// For tracking suggestion updates
	lastInput string

	// Completion menu state
	completions   []string // full-line completions matching current input
	completionIdx int      // -1 = not cycling, 0+ = highlighted index
}

func newModel(state *shellState) model {
	ti := textinput.New()
	ti.Prompt = promptPathStyle.Render(state.nav.cwd) + "> "
	ti.Focus()
	ti.CharLimit = 512
	ti.ShowSuggestions = true

	// We handle Tab/Shift+Tab ourselves for cycling.
	// Right arrow accepts ghost text via textinput.
	ti.KeyMap.AcceptSuggestion = key.NewBinding(key.WithKeys("right"))
	ti.KeyMap.NextSuggestion.SetEnabled(false)
	ti.KeyMap.PrevSuggestion.SetEnabled(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := model{
		state:         state,
		input:         ti,
		spinner:       sp,
		mode:          ModeReady,
		completionIdx: -1,
	}

	// Initial suggestions
	m.updateSuggestions()

	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case commandResultMsg:
		return m.handleCommandResult(msg)

	case scrapeDoneMsg:
		return m.handleScrapeDone(msg)

	case findStepMsg:
		return m.handleFindStep(msg)

	case exportStepMsg:
		return m.handleExportStep(msg)

	case actionDiscoveredMsg:
		return m.handleActionDiscovered(msg)

	case actionResultMsg:
		return m.handleActionResult(msg)

	case spinner.TickMsg:
		// Always process spinner ticks so it doesn't stop.
		// View() only shows the spinner in ModeRunning.
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case ModeReady:
		return m.handleReadyKey(msg)
	case ModeRunning:
		return m.handleRunningKey(msg)
	case ModeAction:
		return m.handleActionKey(msg)
	case ModeConfirm:
		return m.handleConfirmKey(msg)
	}
	return m, nil
}

func (m model) handleReadyKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyTab:
		return m.handleTab(), nil

	case tea.KeyShiftTab:
		return m.handleShiftTab(), nil

	case tea.KeyEscape:
		if m.completionIdx >= 0 {
			m.completionIdx = -1
			m.syncGhostText()
		}
		return m, nil

	case tea.KeyEnter:
		// If cycling through completions, accept the selection
		if m.completionIdx >= 0 && m.completionIdx < len(m.completions) {
			return m.acceptCompletion(), nil
		}

		line := strings.TrimSpace(m.input.Value())
		if line == "" {
			// Empty enter: print blank prompt, scroll down
			return m, tea.Println(promptPathStyle.Render(m.state.nav.cwd) + "> ")
		}

		// Echo the command
		echo := promptPathStyle.Render(m.state.nav.cwd) + "> " + line

		m.state.history.Add(line)
		m.state.history.Reset()
		m.input.SetValue("")
		m.lastInput = ""
		m.completionIdx = -1

		// Handle ! to enter action mode
		if line == "!" {
			m2, cmd := m.enterActionMode()
			return m2, tea.Batch(tea.Println(echo), cmd)
		}

		// Handle scrape specially (needs state)
		if line == "scrape" {
			m.mode = ModeRunning
			m.state.spinnerLabel = "Starting scrape..."
			cmd := startScrape(m.state)
			return m, tea.Batch(tea.Println(echo), cmd)
		}

		// Handle export specially (needs state)
		if line == "export" || strings.HasPrefix(line, "export ") {
			filename := ""
			if strings.HasPrefix(line, "export ") {
				filename = strings.TrimSpace(line[7:])
			}
			m.mode = ModeRunning
			m.state.spinnerLabel = "Starting export..."
			cmd := startExport(m.state, filename)
			return m, tea.Batch(tea.Println(echo), cmd)
		}

		// Handle clear directly
		if line == "clear" {
			m.completionIdx = -1
			return m, tea.ClearScreen
		}

		// Handle find specially (stepped operation like scrape)
		if strings.HasPrefix(line, "find ") {
			pattern := strings.TrimSpace(line[5:])
			if pattern == "" {
				return m, tea.Batch(tea.Println(echo), tea.Println("Error: usage: find <pattern>"))
			}
			cmd, err := startFind(m.state, pattern)
			if err != nil {
				return m, tea.Batch(tea.Println(echo), tea.Println(fmt.Sprintf("Error: %v", err)))
			}
			m.mode = ModeRunning
			m.state.spinnerLabel = "Starting search..."
			return m, tea.Batch(tea.Println(echo), cmd)
		}

		// Parse and execute
		parts := strings.Fields(line)
		cmd := parts[0]
		args := parts[1:]

		m.mode = ModeRunning
		m.state.spinnerLabel = "Running..."
		return m, tea.Batch(tea.Println(echo), executeCommandAsync(m.state.nav, cmd, args))

	case tea.KeyCtrlL:
		return m, tea.ClearScreen

	case tea.KeyCtrlC:
		if m.completionIdx >= 0 {
			m.completionIdx = -1
			m.syncGhostText()
			return m, nil
		}
		if m.input.Value() != "" {
			m.input.SetValue("")
			m.lastInput = ""
			m.updateSuggestions()
			return m, nil
		}
		return m, tea.Quit

	case tea.KeyCtrlD:
		if m.input.Value() == "" {
			return m, tea.Quit
		}
		// Pass through to textinput
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case tea.KeyUp:
		m.completionIdx = -1
		entry, ok := m.state.history.Up(m.input.Value())
		if ok {
			m.input.SetValue(entry)
			m.input.CursorEnd()
			m.lastInput = entry
			m.updateSuggestions()
		}
		return m, nil

	case tea.KeyDown:
		m.completionIdx = -1
		entry, ok := m.state.history.Down(m.input.Value())
		if ok {
			m.input.SetValue(entry)
			m.input.CursorEnd()
			m.lastInput = entry
			m.updateSuggestions()
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)

		// Update suggestions if input changed
		current := m.input.Value()
		if current != m.lastInput {
			m.lastInput = current
			m.updateSuggestions()
		}

		return m, cmd
	}
}

func (m model) handleRunningKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		if len(m.state.scrapeQueue) > 0 {
			m.state.scrapeCancelled = true
		}
		if len(m.state.findQueue) > 0 {
			m.state.findCancelled = true
		}
		if len(m.state.exportQueue) > 0 {
			m.state.exportCancelled = true
		}
	}
	return m, nil
}

func (m model) handleActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyTab:
		return m.handleTab(), nil

	case tea.KeyShiftTab:
		return m.handleShiftTab(), nil

	case tea.KeyEscape:
		if m.completionIdx >= 0 {
			m.completionIdx = -1
			m.syncGhostText()
		}
		return m, nil

	case tea.KeyEnter:
		// If cycling through completions, accept the selection
		if m.completionIdx >= 0 && m.completionIdx < len(m.completions) {
			return m.acceptCompletion(), nil
		}

		line := strings.TrimSpace(m.input.Value())
		if line == "" {
			return m, tea.Println(promptActStyle.Render("action> "))
		}

		// Echo
		echo := promptActStyle.Render("action> ") + line

		m.state.history.Add(line)
		m.state.history.Reset()
		m.input.SetValue("")
		m.lastInput = ""
		m.completionIdx = -1

		parts := strings.Fields(line)
		cmd := parts[0]
		args := parts[1:]

		// Exit action mode
		if cmd == "!" {
			m.mode = ModeReady
			m.input.Prompt = promptPathStyle.Render(m.state.nav.cwd) + "> "
			m.updateSuggestions()
			return m, tea.Println(echo + "\n" + "Exited action mode")
		}

		m.mode = ModeRunning
		m.state.inActionMode = true
		m.state.spinnerLabel = "Running..."
		return m, tea.Batch(tea.Println(echo), executeActionCommandAsync(m.state.nav, cmd, args))

	case tea.KeyCtrlL:
		return m, tea.ClearScreen

	case tea.KeyCtrlC:
		m.mode = ModeReady
		m.input.Prompt = promptPathStyle.Render(m.state.nav.cwd) + "> "
		m.input.SetValue("")
		m.lastInput = ""
		m.completionIdx = -1
		m.updateSuggestions()
		return m, tea.Println("Exited action mode")

	case tea.KeyCtrlD:
		m.mode = ModeReady
		m.input.Prompt = promptPathStyle.Render(m.state.nav.cwd) + "> "
		m.input.SetValue("")
		m.lastInput = ""
		m.completionIdx = -1
		m.updateSuggestions()
		return m, tea.Println("Exited action mode")

	case tea.KeyUp:
		m.completionIdx = -1
		entry, ok := m.state.history.Up(m.input.Value())
		if ok {
			m.input.SetValue(entry)
			m.input.CursorEnd()
			m.lastInput = entry
			m.updateSuggestions()
		}
		return m, nil

	case tea.KeyDown:
		m.completionIdx = -1
		entry, ok := m.state.history.Down(m.input.Value())
		if ok {
			m.input.SetValue(entry)
			m.input.CursorEnd()
			m.lastInput = entry
			m.updateSuggestions()
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)

		current := m.input.Value()
		if current != m.lastInput {
			m.lastInput = current
			m.updateSuggestions()
		}

		return m, cmd
	}
}

func (m model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		action := m.state.pendingAction
		body := m.state.pendingBody
		m.mode = ModeRunning
		m.state.spinnerLabel = "Executing..."
		target := action.Target
		vfs := m.state.nav.vfs
		return m, func() tea.Msg {
			data, status, err := vfs.Post(target, body)
			var bodyStr string
			if err == nil {
				bodyStr = formatActionResult(status, data)
			}
			return actionResultMsg{status: status, body: bodyStr, err: err}
		}

	case "n", "N", "ctrl+c", "escape":
		m.state.pendingAction = nil
		m.state.pendingBody = nil
		m.mode = ModeAction
		m.input.Prompt = promptActStyle.Render("action> ")
		m.input.Focus()
		return m, tea.Println("Cancelled")
	}
	return m, nil
}

func (m model) handleCommandResult(msg commandResultMsg) (tea.Model, tea.Cmd) {
	var output string
	if msg.err != nil {
		output = fmt.Sprintf("Error: %v", msg.err)
	} else if msg.output != "" {
		output = msg.output
	}

	// Update cwd if changed (cd, open)
	if msg.newCwd != "" {
		m.input.Prompt = promptPathStyle.Render(msg.newCwd) + "> "
		if m.mode == ModeAction {
			m.input.Prompt = promptActStyle.Render("action> ")
		}
	}

	if m.state.inActionMode {
		m.mode = ModeAction
		m.input.Prompt = promptActStyle.Render("action> ")
		m.state.inActionMode = false
	} else {
		m.mode = ModeReady
		m.input.Prompt = promptPathStyle.Render(m.state.nav.cwd) + "> "
	}
	m.input.Focus()
	m.state.spinnerLabel = ""
	m.updateSuggestions()

	if output != "" {
		return m, tea.Println(output)
	}
	return m, nil
}

func (m model) handleScrapeDone(msg scrapeDoneMsg) (tea.Model, tea.Cmd) {
	cmd := handleScrapeDone(m.state, msg)
	return m, cmd
}

func (m model) handleFindStep(msg findStepMsg) (tea.Model, tea.Cmd) {
	output, cmd := handleFindStep(m.state, msg)
	if cmd == nil {
		// Find finished — clean up and transition back to ready
		m.state.findQueue = nil
		m.state.findVisited = nil
		m.state.findPattern = nil
		m.mode = ModeReady
		m.input.Prompt = promptPathStyle.Render(m.state.nav.cwd) + "> "
		m.input.Focus()
		m.state.spinnerLabel = ""
		m.updateSuggestions()
	}
	if output != "" {
		return m, tea.Batch(tea.Println(output), cmd)
	}
	return m, cmd
}

func (m model) handleExportStep(msg exportStepMsg) (tea.Model, tea.Cmd) {
	cmd := handleExportStep(m.state, msg)
	if cmd == nil {
		// Export finished — clean up and transition back to ready
		m.mode = ModeReady
		m.input.Prompt = promptPathStyle.Render(m.state.nav.cwd) + "> "
		m.input.Focus()
		m.state.spinnerLabel = ""
		m.updateSuggestions()
	}
	return m, cmd
}

func (m model) handleActionDiscovered(msg actionDiscoveredMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.mode = ModeReady
		m.input.Prompt = promptPathStyle.Render(m.state.nav.cwd) + "> "
		m.input.Focus()
		return m, tea.Println(fmt.Sprintf("Error: %v", msg.err))
	}

	if msg.confirm {
		// Action invocation needing confirmation
		output := msg.output
		if output != "" {
			output += "\nConfirm? [y/N]"
		} else {
			output = "Confirm? [y/N]"
		}
		if len(msg.actions) > 0 {
			action := msg.actions[0]
			m.state.pendingAction = &action
			m.state.pendingBody = msg.body
		}
		m.mode = ModeConfirm
		m.input.Blur()
		return m, tea.Println(output)
	}

	// Entering action mode
	m.mode = ModeAction
	m.input.Prompt = promptActStyle.Render("action> ")
	m.input.Focus()
	m.updateSuggestions()
	if msg.output != "" {
		return m, tea.Println(msg.output)
	}
	return m, nil
}

func (m model) handleActionResult(msg actionResultMsg) (tea.Model, tea.Cmd) {
	var output string
	if msg.err != nil {
		output = fmt.Sprintf("Error: %v", msg.err)
	} else if msg.body != "" {
		output = msg.body
	}

	m.state.pendingAction = nil
	m.state.pendingBody = nil
	m.mode = ModeAction
	m.input.Prompt = promptActStyle.Render("action> ")
	m.input.Focus()
	m.state.spinnerLabel = ""

	if output != "" {
		return m, tea.Println(output)
	}
	return m, nil
}

func (m model) enterActionMode() (model, tea.Cmd) {
	m.mode = ModeRunning
	m.state.spinnerLabel = "Discovering actions..."
	nav := m.state.nav
	return m, func() tea.Msg {
		actions, err := discoverActions(nav)
		if err != nil {
			return commandResultMsg{err: err}
		}
		if len(actions) == 0 {
			return commandResultMsg{output: "No actions on current resource"}
		}
		return actionDiscoveredMsg{
			actions: actions,
			output:  formatActionList(actions),
		}
	}
}

// --- Completion menu logic ---

// handleTab cycles forward through completions
func (m model) handleTab() model {
	if len(m.completions) == 0 {
		return m
	}
	// Single match: fill immediately
	if len(m.completions) == 1 {
		m.input.SetValue(m.completions[0])
		m.input.CursorEnd()
		m.lastInput = m.completions[0]
		m.updateSuggestions()
		return m
	}
	// Multiple matches: cycle
	m.completionIdx = (m.completionIdx + 1) % len(m.completions)
	m.syncGhostText()
	return m
}

// handleShiftTab cycles backward through completions
func (m model) handleShiftTab() model {
	if len(m.completions) <= 1 || m.completionIdx < 0 {
		return m
	}
	m.completionIdx--
	if m.completionIdx < 0 {
		m.completionIdx = len(m.completions) - 1
	}
	m.syncGhostText()
	return m
}

// acceptCompletion fills the selected completion into the input
func (m model) acceptCompletion() model {
	if m.completionIdx < 0 || m.completionIdx >= len(m.completions) {
		return m
	}
	m.input.SetValue(m.completions[m.completionIdx])
	m.input.CursorEnd()
	m.lastInput = m.completions[m.completionIdx]
	m.completionIdx = -1
	m.updateSuggestions()
	return m
}

// syncGhostText sets textinput suggestions to show only the currently
// highlighted completion as ghost text.
func (m *model) syncGhostText() {
	if m.completionIdx >= 0 && m.completionIdx < len(m.completions) {
		m.input.SetSuggestions([]string{m.completions[m.completionIdx]})
	} else if m.input.Value() == "" {
		m.input.SetSuggestions(nil)
	} else {
		m.input.SetSuggestions(m.completions)
	}
}

// updateSuggestions recomputes suggestions for the current input
func (m *model) updateSuggestions() {
	m.completions = computeSuggestions(m.state.nav, m.input.Value(), m.mode == ModeAction)
	m.completionIdx = -1
	// Only show ghost text when there's actual input
	if m.input.Value() == "" {
		m.input.SetSuggestions(nil)
	} else {
		m.input.SetSuggestions(m.completions)
	}
}

// completionMenuDisplay returns the display label for a completion entry.
// Extracts just the varying part (last argument) from a full-line completion.
func completionMenuDisplay(c string) string {
	words := strings.Fields(c)
	if len(words) > 1 {
		return words[len(words)-1]
	}
	return c
}

// renderCompletionMenu renders completions in columns that fit the terminal,
// with the currently selected item highlighted.
func (m model) renderCompletionMenu() string {
	labels := make([]string, len(m.completions))
	for i, c := range m.completions {
		labels[i] = completionMenuDisplay(c)
	}
	return formatCompletionColumns(labels, m.completionIdx)
}

// View renders only the prompt line (inline mode)
func (m model) View() string {
	switch m.mode {
	case ModeRunning:
		label := m.state.spinnerLabel
		if label == "" {
			label = "Running..."
		}
		return m.spinner.View() + " " + label
	case ModeConfirm:
		return ""
	default:
		v := m.input.View()
		showMenu := len(m.completions) > 1 && (m.input.Value() != "" || m.completionIdx >= 0)
		if showMenu {
			// Trailing space differentiates this line from the no-menu render,
			// preventing bubbletea's inline renderer from skipping it (canSkip)
			// and then erasing it with EraseScreenBelow when the view shrinks.
			v += " \n" + m.renderCompletionMenu()
		}
		return v
	}
}
