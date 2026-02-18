package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bluefish-project/bluefish/rvfs"
)

// exportTickMsg triggers a single resource fetch during export
type exportTickMsg struct {
	Path string
}

// exportDoneMsg is sent after one resource is fetched (or fails)
type exportDoneMsg struct {
	Path        string
	RawJSON     []byte
	Err         error
	NewChildren []string
}

// exportWrittenMsg is sent after the JSON file is written
type exportWrittenMsg struct {
	Filename string
	Count    int
	Err      error
}

// ExportModel manages the export overlay
type ExportModel struct {
	vfs       rvfs.VFS
	root      string
	filename  string
	queue     []string
	visited   map[string]bool
	collected map[string]json.RawMessage
	done      int
	total     int
	current   string
	errors    []string
	active    bool
	written   bool
	result    string
	width     int
	height    int
}

func NewExportModel(vfs rvfs.VFS) ExportModel {
	return ExportModel{vfs: vfs}
}

// Start begins an export from a root path
func (e *ExportModel) Start(rootPath, filename string) tea.Cmd {
	e.active = true
	e.root = rootPath
	e.filename = filename
	e.queue = nil
	e.visited = make(map[string]bool)
	e.collected = make(map[string]json.RawMessage)
	e.done = 0
	e.total = 0
	e.current = ""
	e.errors = nil
	e.written = false
	e.result = ""

	// BFS from rootPath to discover all reachable paths
	cached := make(map[string]bool)
	for _, p := range e.vfs.GetKnownPaths() {
		cached[p] = true
	}

	frontier := []string{rootPath}
	var uncached []string

	for len(frontier) > 0 {
		path := frontier[0]
		frontier = frontier[1:]
		if e.visited[path] {
			continue
		}
		e.visited[path] = true

		if !cached[path] {
			uncached = append(uncached, path)
			continue
		}

		// Pre-collect cached resources
		res, err := e.vfs.Get(path)
		if err != nil {
			continue
		}
		if len(res.RawJSON) > 0 {
			e.collected[path] = json.RawMessage(res.RawJSON)
		}
		for _, child := range res.Children {
			if !e.visited[child.Target] {
				frontier = append(frontier, child.Target)
			}
		}
	}

	e.queue = uncached
	e.total = len(uncached)

	if e.total == 0 {
		// Everything cached, write immediately
		return e.writeFile()
	}

	return e.fetchNext()
}

func (e *ExportModel) fetchNext() tea.Cmd {
	if len(e.queue) == 0 {
		return nil
	}
	path := e.queue[0]
	e.current = path
	return func() tea.Msg {
		return exportTickMsg{Path: path}
	}
}

// HandleTick fetches one resource and returns the result
func (e *ExportModel) HandleTick(path string) tea.Cmd {
	vfs := e.vfs
	return func() tea.Msg {
		res, err := vfs.Get(path)
		var rawJSON []byte
		var newChildren []string
		if err == nil {
			rawJSON = res.RawJSON
			for _, child := range res.Children {
				newChildren = append(newChildren, child.Target)
			}
		}
		return exportDoneMsg{Path: path, RawJSON: rawJSON, Err: err, NewChildren: newChildren}
	}
}

// HandleDone processes the result of a single fetch and queues more work
func (e *ExportModel) HandleDone(msg exportDoneMsg) tea.Cmd {
	if len(e.queue) > 0 && e.queue[0] == msg.Path {
		e.queue = e.queue[1:]
	}
	e.done++

	if msg.Err != nil {
		e.errors = append(e.errors, fmt.Sprintf("%s: %v", msg.Path, msg.Err))
	} else {
		if len(msg.RawJSON) > 0 {
			e.collected[msg.Path] = json.RawMessage(msg.RawJSON)
		}
		// Add newly discovered uncached children to queue
		cached := make(map[string]bool)
		for _, p := range e.vfs.GetKnownPaths() {
			cached[p] = true
		}
		for _, child := range msg.NewChildren {
			if !e.visited[child] {
				e.visited[child] = true
				if !cached[child] {
					e.queue = append(e.queue, child)
					e.total++
				} else {
					// Pre-collect newly discovered cached resource
					res, err := e.vfs.Get(child)
					if err == nil && len(res.RawJSON) > 0 {
						e.collected[child] = json.RawMessage(res.RawJSON)
					}
				}
			}
		}
	}

	if len(e.queue) == 0 {
		return e.writeFile()
	}

	return e.fetchNext()
}

func (e *ExportModel) writeFile() tea.Cmd {
	collected := e.collected
	filename := e.filename
	return func() tea.Msg {
		data, err := json.MarshalIndent(collected, "", "  ")
		if err != nil {
			return exportWrittenMsg{Filename: filename, Err: fmt.Errorf("marshal: %v", err)}
		}
		err = os.WriteFile(filename, data, 0644)
		return exportWrittenMsg{Filename: filename, Count: len(collected), Err: err}
	}
}

// HandleWritten processes the file write result
func (e *ExportModel) HandleWritten(msg exportWrittenMsg) {
	e.written = true
	if msg.Err != nil {
		e.result = fmt.Sprintf("Error: %v", msg.Err)
	} else {
		e.result = fmt.Sprintf("Exported %d resources to %s", msg.Count, msg.Filename)
	}
}

func (e *ExportModel) IsActive() bool {
	return e.active
}

func (e *ExportModel) IsDone() bool {
	return e.active && e.written
}

func (e *ExportModel) Close() {
	e.active = false
	e.queue = nil
	e.collected = nil
	e.visited = nil
}

func (e *ExportModel) View() string {
	var b strings.Builder

	b.WriteString(detailLabelStyle.Render("Export"))
	b.WriteString("\n\n")

	if e.written {
		if strings.HasPrefix(e.result, "Error") {
			b.WriteString("  " + actionErrorStyle.Render(e.result))
		} else {
			b.WriteString("  " + actionSuccessStyle.Render(e.result))
		}
		b.WriteString("\n\n")
		b.WriteString(helpDescStyle.Render("  esc: close"))
		return b.String()
	}

	if e.total == 0 && len(e.collected) > 0 {
		b.WriteString(fmt.Sprintf("  %s Writing %d resources...\n",
			detailLabelStyle.Render("Progress:"), len(e.collected)))
		b.WriteString("\n")
		b.WriteString(helpDescStyle.Render("  esc: close"))
		return b.String()
	}

	// Progress fraction
	b.WriteString(fmt.Sprintf("  %s %d / %d",
		detailLabelStyle.Render("Progress:"),
		e.done, e.total))
	b.WriteString("\n")

	// Progress bar
	barWidth := e.width - 4
	if barWidth < 10 {
		barWidth = 10
	}
	filled := 0
	if e.total > 0 {
		filled = barWidth * e.done / e.total
	}
	if filled > barWidth {
		filled = barWidth
	}
	bar := lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)).Render(strings.Repeat("█", filled))
	empty := lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8)).Render(strings.Repeat("░", barWidth-filled))
	b.WriteString("  " + bar + empty)
	b.WriteString("\n\n")

	// Current path
	if e.current != "" {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			helpDescStyle.Render("Fetching:"),
			childStyle.Render(e.current)))
	}

	// Collected count
	b.WriteString(fmt.Sprintf("  %s %d\n",
		helpDescStyle.Render("Collected:"),
		len(e.collected)))

	// Remaining
	remaining := len(e.queue)
	if remaining > 0 {
		b.WriteString(fmt.Sprintf("  %s %d\n",
			helpDescStyle.Render("Remaining:"),
			remaining))
	}

	// Errors
	if len(e.errors) > 0 {
		b.WriteString(fmt.Sprintf("\n  %s %d\n",
			actionErrorStyle.Render("Errors:"),
			len(e.errors)))
		show := len(e.errors)
		if show > 3 {
			show = 3
		}
		for _, err := range e.errors[len(e.errors)-show:] {
			b.WriteString("    " + actionErrorStyle.Render(err) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpDescStyle.Render("  esc: close"))

	return b.String()
}
