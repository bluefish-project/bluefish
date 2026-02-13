package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"bluefish/rvfs"
)

// ScrapeModel manages the resource crawl overlay
type ScrapeModel struct {
	vfs     rvfs.VFS
	queue   []string // Paths still to fetch
	done    int      // Count of fetched paths
	total   int      // Total discovered paths
	current string   // Path currently being fetched
	errors  []string // Errors encountered
	active  bool
	width   int
	height  int
}

func NewScrapeModel(vfs rvfs.VFS) ScrapeModel {
	return ScrapeModel{vfs: vfs}
}

// Start begins a scrape from a root path, queueing all uncached children
func (s *ScrapeModel) Start(rootPath string) tea.Cmd {
	s.active = true
	s.queue = nil
	s.done = 0
	s.total = 0
	s.current = ""
	s.errors = nil

	// Seed: collect all known children recursively from cached resources,
	// find the ones that aren't cached yet
	cached := make(map[string]bool)
	for _, p := range s.vfs.GetKnownPaths() {
		cached[p] = true
	}

	// BFS from rootPath to discover all reachable children
	visited := make(map[string]bool)
	frontier := []string{rootPath}
	var uncached []string

	for len(frontier) > 0 {
		path := frontier[0]
		frontier = frontier[1:]
		if visited[path] {
			continue
		}
		visited[path] = true

		if !cached[path] {
			uncached = append(uncached, path)
			continue // Can't inspect children of uncached resources yet
		}

		// Inspect cached resource for children
		res, err := s.vfs.Get(path)
		if err != nil {
			continue
		}
		for _, child := range res.Children {
			if !visited[child.Target] {
				frontier = append(frontier, child.Target)
			}
		}
	}

	s.queue = uncached
	s.total = len(uncached)

	if s.total == 0 {
		s.current = "Everything is cached"
		return nil
	}

	return s.fetchNext()
}

// fetchNext returns a Cmd that fetches the next item in the queue
func (s *ScrapeModel) fetchNext() tea.Cmd {
	if len(s.queue) == 0 {
		return nil
	}
	path := s.queue[0]
	s.current = path
	return func() tea.Msg {
		return scrapeTickMsg{Path: path}
	}
}

// scrapeTickMsg triggers a single resource fetch during scraping
type scrapeTickMsg struct {
	Path string
}

// scrapeDoneMsg is sent after one resource is fetched (or fails)
type scrapeDoneMsg struct {
	Path         string
	Resource     *rvfs.Resource
	Err          error
	NewChildren  []string // Newly discovered child paths
}

// HandleTick fetches one resource and returns the result
func (s *ScrapeModel) HandleTick(path string) tea.Cmd {
	vfs := s.vfs
	return func() tea.Msg {
		res, err := vfs.Get(path)
		var newChildren []string
		if err == nil {
			// Discover children we haven't seen
			for _, child := range res.Children {
				newChildren = append(newChildren, child.Target)
			}
		}
		return scrapeDoneMsg{Path: path, Resource: res, Err: err, NewChildren: newChildren}
	}
}

// HandleDone processes the result of a single fetch and queues more work
func (s *ScrapeModel) HandleDone(msg scrapeDoneMsg) tea.Cmd {
	// Remove from front of queue
	if len(s.queue) > 0 && s.queue[0] == msg.Path {
		s.queue = s.queue[1:]
	}
	s.done++

	if msg.Err != nil {
		s.errors = append(s.errors, fmt.Sprintf("%s: %v", msg.Path, msg.Err))
	} else {
		// Add newly discovered uncached children to queue
		queued := make(map[string]bool)
		for _, p := range s.queue {
			queued[p] = true
		}
		cached := make(map[string]bool)
		for _, p := range s.vfs.GetKnownPaths() {
			cached[p] = true
		}
		for _, child := range msg.NewChildren {
			if !cached[child] && !queued[child] {
				s.queue = append(s.queue, child)
				s.total++
			}
		}
	}

	return s.fetchNext()
}

func (s *ScrapeModel) IsActive() bool {
	return s.active
}

func (s *ScrapeModel) IsDone() bool {
	return s.active && len(s.queue) == 0
}

func (s *ScrapeModel) Close() {
	s.active = false
	s.queue = nil
}

func (s *ScrapeModel) View() string {
	var b strings.Builder

	b.WriteString(detailLabelStyle.Render("Scrape"))
	b.WriteString("\n\n")

	if s.total == 0 {
		b.WriteString(actionSuccessStyle.Render("  All reachable resources are cached."))
		b.WriteString("\n\n")
		b.WriteString(helpDescStyle.Render("  esc: close"))
		return b.String()
	}

	// Progress fraction
	b.WriteString(fmt.Sprintf("  %s %d / %d",
		detailLabelStyle.Render("Progress:"),
		s.done, s.total))
	b.WriteString("\n")

	// Progress bar
	barWidth := s.width - 4
	if barWidth < 10 {
		barWidth = 10
	}
	filled := 0
	if s.total > 0 {
		filled = barWidth * s.done / s.total
	}
	if filled > barWidth {
		filled = barWidth
	}
	bar := lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)).Render(strings.Repeat("█", filled))
	empty := lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8)).Render(strings.Repeat("░", barWidth-filled))
	b.WriteString("  " + bar + empty)
	b.WriteString("\n\n")

	// Current path
	if s.current != "" {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			helpDescStyle.Render("Fetching:"),
			childStyle.Render(s.current)))
	}

	// Remaining
	remaining := len(s.queue)
	if remaining > 0 {
		b.WriteString(fmt.Sprintf("  %s %d\n",
			helpDescStyle.Render("Remaining:"),
			remaining))
	}

	// Errors
	if len(s.errors) > 0 {
		b.WriteString(fmt.Sprintf("\n  %s %d\n",
			actionErrorStyle.Render("Errors:"),
			len(s.errors)))
		show := len(s.errors)
		if show > 3 {
			show = 3
		}
		for _, e := range s.errors[len(s.errors)-show:] {
			b.WriteString("    " + actionErrorStyle.Render(e) + "\n")
		}
	}

	if s.IsDone() {
		b.WriteString("\n")
		b.WriteString(actionSuccessStyle.Render("  Done!"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpDescStyle.Render("  esc: close"))

	return b.String()
}
