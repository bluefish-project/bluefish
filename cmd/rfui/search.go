package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// SearchModel manages the search overlay
type SearchModel struct {
	input    textinput.Model
	paths    []string // All known paths
	results  []string // Filtered results
	cursor   int
	maxShow  int // Max results to display
	height   int
	width    int
}

func NewSearchModel() SearchModel {
	ti := textinput.New()
	ti.Placeholder = "Search paths..."
	ti.CharLimit = 256
	return SearchModel{
		input:   ti,
		maxShow: 10,
	}
}

// Open activates search mode with current known paths
func (s *SearchModel) Open(paths []string) {
	s.paths = paths
	s.input.SetValue("")
	s.input.Focus()
	s.cursor = 0
	s.results = nil
}

// Close deactivates search mode
func (s *SearchModel) Close() {
	s.input.Blur()
}

// Selected returns the currently highlighted result path, or empty
func (s *SearchModel) Selected() string {
	if s.cursor >= 0 && s.cursor < len(s.results) {
		return s.results[s.cursor]
	}
	return ""
}

func (s *SearchModel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)

	// Re-filter on every keystroke
	query := s.input.Value()
	s.filter(query)

	return cmd
}

func (s *SearchModel) filter(query string) {
	if query == "" {
		s.results = nil
		s.cursor = 0
		return
	}

	lower := strings.ToLower(query)
	s.results = nil

	for _, p := range s.paths {
		if fuzzyMatch(strings.ToLower(p), lower) {
			s.results = append(s.results, p)
			if len(s.results) >= 50 {
				break
			}
		}
	}

	if s.cursor >= len(s.results) {
		s.cursor = len(s.results) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
}

// fuzzyMatch checks if all characters of pattern appear in order in text
func fuzzyMatch(text, pattern string) bool {
	pi := 0
	for ti := 0; ti < len(text) && pi < len(pattern); ti++ {
		if text[ti] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

func (s *SearchModel) MoveUp() {
	if s.cursor > 0 {
		s.cursor--
	}
}

func (s *SearchModel) MoveDown() {
	if s.cursor < len(s.results)-1 {
		s.cursor++
	}
}

func (s *SearchModel) View() string {
	var b strings.Builder

	b.WriteString(searchPromptStyle.Render("Search: "))
	b.WriteString(s.input.View())
	b.WriteString("\n")

	if len(s.results) == 0 && s.input.Value() != "" {
		b.WriteString(helpDescStyle.Render("  No matches"))
		b.WriteString("\n")
	}

	show := s.maxShow
	if show > len(s.results) {
		show = len(s.results)
	}

	// Window around cursor
	start := 0
	if s.cursor >= show {
		start = s.cursor - show + 1
	}
	end := start + show
	if end > len(s.results) {
		end = len(s.results)
		start = end - show
		if start < 0 {
			start = 0
		}
	}

	for i := start; i < end; i++ {
		if i == s.cursor {
			b.WriteString(cursorStyle.Render("  " + s.results[i]))
		} else {
			b.WriteString(searchMatchStyle.Render("  " + s.results[i]))
		}
		b.WriteString("\n")
	}

	b.WriteString(helpDescStyle.Render("  enter:go  esc:cancel  ctrl+j/k:nav"))
	return b.String()
}
