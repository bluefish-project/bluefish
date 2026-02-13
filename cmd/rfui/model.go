package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"bluefish/rvfs"
)

// Mode represents the current UI mode
type Mode int

const (
	ModeNormal Mode = iota
	ModeSearch
	ModeAction
	ModeHelp
	ModeScrape
)

// Model is the root Bubble Tea model
type Model struct {
	vfs       rvfs.VFS
	basePath  string
	rootStack []string

	tree       TreeModel
	details    DetailsModel
	breadcrumb BreadcrumbModel
	search     SearchModel
	action     ActionModel
	scrape     ScrapeModel

	width, height    int
	mode             Mode
	statusMsg        string
	loading          bool
	currentFetchedAt time.Time
}

// NewModel creates a new root model
func NewModel(vfs rvfs.VFS) Model {
	return Model{
		vfs:        vfs,
		basePath:   rvfs.RedfishRoot,
		tree:       NewTreeModel(),
		details:    NewDetailsModel(),
		breadcrumb: NewBreadcrumbModel(),
		search:     NewSearchModel(),
		action:     NewActionModel(),
		scrape:     NewScrapeModel(vfs),
	}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		resource, err := m.vfs.Get(m.basePath)
		return ResourceLoadedMsg{Path: m.basePath, Resource: resource, Err: err}
	}
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil

	case ResourceLoadedMsg:
		return m.handleResourceLoaded(msg)

	case fetchResourceMsg:
		path := msg.Path
		return m, func() tea.Msg {
			resource, err := m.vfs.Get(path)
			return ResourceLoadedMsg{Path: path, Resource: resource, Err: err}
		}

	case ActionsDiscoveredMsg:
		return m.handleActionsDiscovered(msg)

	case ActionResultMsg:
		m.action.SetResult(msg.StatusCode, msg.Body, msg.Err)
		return m, nil

	case scrapeTickMsg:
		cmd := m.scrape.HandleTick(msg.Path)
		return m, cmd

	case scrapeDoneMsg:
		cmd := m.scrape.HandleDone(msg)
		return m, cmd

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleResourceLoaded(msg ResourceLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.statusMsg = fmt.Sprintf("Error: %v", msg.Err)
		m.loading = false
		return m, nil
	}

	if msg.Path == m.basePath && m.tree.root == nil {
		// Initial load
		m.tree.Init(msg.Resource, msg.Path)
		m.recalcLayout()
		m.statusMsg = ""
		m.loading = false
		m.currentFetchedAt = msg.Resource.FetchedAt

		item := m.tree.Current()
		if item != nil {
			m.details.SetItem(item)
		}
		return m, nil
	}

	// Async child load
	m.tree.HandleResourceLoaded(msg.Path, msg.Resource)
	m.loading = false

	// Track age of the resource at cursor
	if msg.Resource != nil {
		m.currentFetchedAt = msg.Resource.FetchedAt
	}

	// Update details if cursor is on this item
	item := m.tree.Current()
	if item != nil {
		m.details.SetItem(item)
	}
	return m, nil
}

func (m Model) handleActionsDiscovered(msg ActionsDiscoveredMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.statusMsg = fmt.Sprintf("Action error: %v", msg.Err)
		return m, nil
	}
	if len(msg.Actions) == 0 {
		m.statusMsg = "No actions on current resource"
		return m, nil
	}
	m.mode = ModeAction
	m.action.Open(msg.Actions)
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case ModeNormal:
		return m.handleNormalKey(msg)
	case ModeSearch:
		return m.handleSearchKey(msg)
	case ModeAction:
		return m.handleActionKey(msg)
	case ModeHelp:
		return m.handleHelpKey(msg)
	case ModeScrape:
		return m.handleScrapeKey(msg)
	}
	return m, nil
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, normalKeys.Quit):
		return m, tea.Quit

	case key.Matches(msg, normalKeys.Down):
		item := m.tree.MoveDown()
		if item != nil {
			m.details.SetItem(item)
		}

	case key.Matches(msg, normalKeys.Up):
		item := m.tree.MoveUp()
		if item != nil {
			m.details.SetItem(item)
		}

	case key.Matches(msg, normalKeys.Expand):
		cmd := m.tree.Expand()
		item := m.tree.Current()
		if item != nil {
			m.details.SetItem(item)
		}
		return m, cmd

	case key.Matches(msg, normalKeys.Collapse):
		m.tree.Collapse()
		item := m.tree.Current()
		if item != nil {
			m.details.SetItem(item)
		}

	case key.Matches(msg, normalKeys.Toggle):
		cmd := m.tree.Toggle()
		return m, cmd

	case key.Matches(msg, normalKeys.Enter):
		return m.handleEnter()

	case key.Matches(msg, normalKeys.Back):
		return m.handleBack()

	case key.Matches(msg, normalKeys.GoUp):
		return m.handleGoUp()

	case key.Matches(msg, normalKeys.Home):
		return m.handleHome()

	case key.Matches(msg, normalKeys.Refresh):
		return m.handleRefresh()

	case key.Matches(msg, normalKeys.Scrape):
		return m.handleScrape()

	case key.Matches(msg, normalKeys.ScrollDown):
		m.details.ScrollDown()

	case key.Matches(msg, normalKeys.ScrollUp):
		m.details.ScrollUp()

	case key.Matches(msg, normalKeys.Search):
		m.mode = ModeSearch
		m.recalcLayout()
		paths := m.vfs.GetKnownPaths()
		m.search.Open(paths)

	case key.Matches(msg, normalKeys.Action):
		return m.handleActionMode()

	case key.Matches(msg, normalKeys.Help):
		m.mode = ModeHelp
		m.recalcLayout()
	}

	return m, nil
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, searchKeys.Cancel):
		m.mode = ModeNormal
		m.search.Close()
		m.recalcLayout()
		return m, nil

	case key.Matches(msg, searchKeys.Confirm):
		path := m.search.Selected()
		m.mode = ModeNormal
		m.search.Close()
		m.recalcLayout()
		if path != "" {
			return m.navigateTo(path)
		}
		return m, nil

	case key.Matches(msg, searchKeys.NextItem):
		m.search.MoveDown()
		return m, nil

	case key.Matches(msg, searchKeys.PrevItem):
		m.search.MoveUp()
		return m, nil
	}

	// Pass to text input
	cmd := m.search.Update(msg)
	return m, cmd
}

func (m Model) handleActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.action.phase {
	case PhaseSelect:
		switch {
		case key.Matches(msg, actionKeys.Cancel):
			m.mode = ModeNormal
			m.action.Close()
			m.recalcLayout()
		case key.Matches(msg, actionKeys.Up):
			m.action.MoveUp()
		case key.Matches(msg, actionKeys.Down):
			m.action.MoveDown()
		case key.Matches(msg, actionKeys.Confirm):
			m.action.SelectAction()
		}

	case PhaseParams:
		switch {
		case key.Matches(msg, actionKeys.Cancel):
			if !m.action.BackPhase() {
				m.mode = ModeNormal
				m.action.Close()
				m.recalcLayout()
			}
		case key.Matches(msg, actionKeys.Tab):
			m.action.CycleAllowable()
		case key.Matches(msg, actionKeys.Confirm):
			m.action.ConfirmParams()
		default:
			cmd := m.action.Update(msg)
			return m, cmd
		}

	case PhaseConfirm:
		switch {
		case key.Matches(msg, actionKeys.Yes):
			return m.executeAction()
		case key.Matches(msg, actionKeys.No), key.Matches(msg, actionKeys.Cancel):
			if !m.action.BackPhase() {
				m.mode = ModeNormal
				m.action.Close()
				m.recalcLayout()
			}
		}

	case PhaseResult:
		if key.Matches(msg, actionKeys.Cancel) {
			m.mode = ModeNormal
			m.action.Close()
			m.recalcLayout()
		}
	}

	return m, nil
}

func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, overlayKeys.Cancel) {
		m.mode = ModeNormal
		m.recalcLayout()
	}
	return m, nil
}

func (m Model) handleScrapeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, overlayKeys.Cancel) {
		m.mode = ModeNormal
		m.scrape.Close()
		m.recalcLayout()
	}
	return m, nil
}

// handleEnter: navigate into the selected item, pushing current root
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	item := m.tree.Current()
	if item == nil {
		return m, nil
	}

	switch item.Kind {
	case KindLink:
		m.rootStack = append(m.rootStack, m.basePath)
		return m.navigateTo(item.LinkTarget)
	case KindChild, KindResource:
		m.rootStack = append(m.rootStack, m.basePath)
		return m.navigateTo(item.Path)
	default:
		// For properties, toggle expand/collapse
		cmd := m.tree.Toggle()
		return m, cmd
	}
}

func (m Model) handleBack() (tea.Model, tea.Cmd) {
	if len(m.rootStack) == 0 {
		m.statusMsg = "At root of navigation stack"
		return m, nil
	}

	prev := m.rootStack[len(m.rootStack)-1]
	m.rootStack = m.rootStack[:len(m.rootStack)-1]
	return m.navigateTo(prev)
}

func (m Model) handleGoUp() (tea.Model, tea.Cmd) {
	parent := m.vfs.Parent(m.basePath)
	if parent == m.basePath {
		m.statusMsg = "Already at top"
		return m, nil
	}
	m.rootStack = append(m.rootStack, m.basePath)
	return m.navigateTo(parent)
}

func (m Model) handleHome() (tea.Model, tea.Cmd) {
	if m.basePath == rvfs.RedfishRoot {
		m.statusMsg = "Already at root"
		return m, nil
	}
	m.rootStack = nil
	return m.navigateTo(rvfs.RedfishRoot)
}

func (m Model) handleRefresh() (tea.Model, tea.Cmd) {
	item := m.tree.Current()
	if item == nil {
		return m, nil
	}

	// Only resource-backed items (Child, Resource, Link) can be refreshed
	path := item.Path
	switch item.Kind {
	case KindChild, KindResource:
		// Refresh this resource
	case KindLink:
		path = item.LinkTarget
	default:
		m.statusMsg = "Nothing to refresh (select a resource)"
		return m, nil
	}

	m.vfs.Invalidate(path)
	m.statusMsg = fmt.Sprintf("Refreshing %s...", path)
	return m, func() tea.Msg {
		resource, err := m.vfs.Get(path)
		return ResourceLoadedMsg{Path: path, Resource: resource, Err: err}
	}
}

func (m Model) handleScrape() (tea.Model, tea.Cmd) {
	m.mode = ModeScrape
	m.recalcLayout()
	cmd := m.scrape.Start(m.basePath)
	return m, cmd
}

func (m Model) handleActionMode() (tea.Model, tea.Cmd) {
	item := m.tree.Current()
	if item == nil {
		return m, nil
	}

	var resource *rvfs.Resource
	if item.Resource != nil {
		resource = item.Resource
	} else {
		res, err := m.vfs.Get(m.basePath)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			return m, nil
		}
		resource = res
	}

	actions := discoverActions(resource)
	if len(actions) == 0 {
		m.statusMsg = "No actions on current resource"
		return m, nil
	}
	m.mode = ModeAction
	m.recalcLayout()
	m.action.Open(actions)
	return m, nil
}

func (m Model) executeAction() (tea.Model, tea.Cmd) {
	action := m.action.selected
	body, err := m.action.BuildBody()
	if err != nil {
		m.action.SetResult(0, "", err)
		return m, nil
	}

	target := action.Target
	return m, func() tea.Msg {
		data, status, err := m.vfs.Post(target, body)
		var bodyStr string
		if len(data) > 0 {
			var buf bytes.Buffer
			if json.Indent(&buf, data, "", "  ") == nil {
				bodyStr = buf.String()
			} else {
				bodyStr = string(data)
			}
		}
		return ActionResultMsg{StatusCode: status, Body: bodyStr, Err: err}
	}
}

func (m Model) navigateTo(path string) (tea.Model, tea.Cmd) {
	m.basePath = path
	m.breadcrumb.SetPath(path)
	m.tree = NewTreeModel()
	m.loading = true
	m.statusMsg = ""
	m.currentFetchedAt = time.Time{}

	return m, func() tea.Msg {
		resource, err := m.vfs.Get(path)
		return ResourceLoadedMsg{Path: path, Resource: resource, Err: err}
	}
}

func (m *Model) recalcLayout() {
	// Measure chrome heights from actual renders
	statusHeight := lipgloss.Height(m.viewStatusBar())
	breadcrumbHeight := lipgloss.Height(m.breadcrumb.View())
	helpHeight := lipgloss.Height(m.viewHelpBar())
	chrome := statusHeight + breadcrumbHeight + helpHeight

	// Content area is everything between chrome — full height regardless of overlay
	contentHeight := m.height - chrome
	if contentHeight < 3 {
		contentHeight = 3
	}

	// Measure separator
	sep := separatorStyle.Render(" │ ")
	sepWidth := lipgloss.Width(sep)

	// Tree gets 40%, details gets the rest minus separator
	treeWidth := m.width * 2 / 5
	detailsWidth := m.width - treeWidth - sepWidth

	m.tree.width = treeWidth
	m.tree.height = contentHeight
	m.tree.ensureVisible()

	m.details.SetSize(detailsWidth, contentHeight)
	m.breadcrumb.SetWidth(m.width)

	// Overlay inner dimensions (60% wide, 50% tall, centered)
	if m.mode != ModeNormal {
		frameW, frameH := overlayStyle.GetFrameSize()
		innerW := m.width*3/5 - frameW
		innerH := contentHeight/2 - frameH
		if innerW < 20 {
			innerW = 20
		}
		if innerH < 5 {
			innerH = 5
		}
		m.search.width = innerW
		m.search.height = innerH
		m.action.width = innerW
		m.action.height = innerH
		m.scrape.width = innerW
		m.scrape.height = innerH
	}
}

// View implements tea.Model
func (m Model) View() string {
	if m.width == 0 {
		return "Starting..."
	}

	var sections []string

	// Status bar
	sections = append(sections, m.viewStatusBar())

	// Breadcrumb
	sections = append(sections, m.breadcrumb.View())

	// Main content: tree | separator | details — always rendered at full height
	sep := separatorStyle.Render(" │ ")
	sepWidth := lipgloss.Width(sep)
	treeWidth := m.width * 2 / 5
	detailsWidth := m.width - treeWidth - sepWidth

	treePanel := lipgloss.NewStyle().
		Width(treeWidth).
		Height(m.tree.height).
		MaxHeight(m.tree.height).
		Render(m.tree.View())

	detailsPanel := lipgloss.NewStyle().
		Width(detailsWidth).
		Height(m.tree.height).
		MaxHeight(m.tree.height).
		Render(m.details.View())

	content := lipgloss.JoinHorizontal(lipgloss.Top, treePanel, sep, detailsPanel)

	// Composite overlay on top of content
	if overlay, ok := m.renderOverlay(); ok {
		content = placeOverlay(m.width, m.tree.height, overlay, content)
	}

	sections = append(sections, content)

	// Help bar
	sections = append(sections, m.viewHelpBar())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderOverlay returns the rendered overlay string and true if an overlay is active
func (m Model) renderOverlay() (string, bool) {
	var inner string
	var w, h int

	switch m.mode {
	case ModeSearch:
		inner = m.search.View()
		w, h = m.search.width, m.search.height
	case ModeAction:
		inner = m.action.View()
		w, h = m.action.width, m.action.height
	case ModeHelp:
		inner = helpContent()
		w, h = m.search.width, m.search.height // same overlay size
	case ModeScrape:
		inner = m.scrape.View()
		w, h = m.scrape.width, m.scrape.height
	default:
		return "", false
	}

	rendered := overlayStyle.
		Width(w).
		Height(h).
		Render(inner)
	return rendered, true
}

// placeOverlay composites a foreground panel centered on top of a background.
// The overlay replaces background lines — it is fully opaque.
func placeOverlay(bgWidth, bgHeight int, overlay, background string) string {
	bgLines := strings.Split(background, "\n")
	fgLines := strings.Split(overlay, "\n")

	fgWidth := lipgloss.Width(overlay)
	fgHeight := len(fgLines)

	// Center
	startX := (bgWidth - fgWidth) / 2
	startY := (bgHeight - fgHeight) / 2
	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}

	for i, fgLine := range fgLines {
		row := startY + i
		if row >= len(bgLines) {
			break
		}
		leftPad := strings.Repeat(" ", startX)
		rightPad := bgWidth - startX - lipgloss.Width(fgLine)
		right := ""
		if rightPad > 0 {
			right = strings.Repeat(" ", rightPad)
		}
		bgLines[row] = leftPad + fgLine + right
	}

	return strings.Join(bgLines, "\n")
}

func (m Model) viewStatusBar() string {
	title := statusStyle.Render("RFUI")

	var info string
	if m.statusMsg != "" {
		info = "  " + m.statusMsg
	} else if m.basePath == rvfs.RedfishRoot {
		info = "  Tree: Full"
	} else {
		info = fmt.Sprintf("  Subtree: %s", m.basePath)
	}

	var age string
	if !m.currentFetchedAt.IsZero() {
		age = "  " + helpDescStyle.Render(formatAge(m.currentFetchedAt))
	}

	return title + info + age
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func (m Model) viewHelpBar() string {
	var pairs []string
	switch m.mode {
	case ModeNormal:
		pairs = []string{
			"enter", "open",
			"h/j/k/l", "nav",
			"bs", "back",
			"/", "search",
			"!", "action",
			"s", "scrape",
			"?", "help",
		}
	case ModeSearch:
		pairs = []string{
			"enter", "go",
			"esc", "cancel",
			"ctrl+j/k", "nav",
		}
	case ModeAction:
		pairs = []string{
			"esc", "back",
		}
	case ModeHelp, ModeScrape:
		pairs = []string{
			"esc", "close",
		}
	}

	var parts []string
	for i := 0; i < len(pairs)-1; i += 2 {
		parts = append(parts, helpKeyStyle.Render(pairs[i])+":"+helpDescStyle.Render(pairs[i+1]))
	}

	return strings.Join(parts, "  ")
}
