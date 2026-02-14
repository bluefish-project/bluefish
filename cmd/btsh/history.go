package main

import (
	"os"
	"strings"
)

const maxHistoryEntries = 1000

// History manages command history with Up/Down navigation and file persistence
type History struct {
	lines  []string
	cursor int
	saved  string // Input saved before navigating
	file   string
}

// NewHistory creates a history, loading from file if it exists
func NewHistory(file string) *History {
	h := &History{file: file}
	h.load()
	h.cursor = len(h.lines)
	return h
}

// Add appends a line and saves to file
func (h *History) Add(line string) {
	if line == "" {
		return
	}
	// Deduplicate consecutive entries
	if len(h.lines) > 0 && h.lines[len(h.lines)-1] == line {
		h.cursor = len(h.lines)
		return
	}
	h.lines = append(h.lines, line)
	if len(h.lines) > maxHistoryEntries {
		h.lines = h.lines[len(h.lines)-maxHistoryEntries:]
	}
	h.cursor = len(h.lines)
	h.save()
}

// Up navigates backward in history. Returns the history entry, or "" if at the top.
// On the first Up press, saves the current input.
func (h *History) Up(currentInput string) (string, bool) {
	if len(h.lines) == 0 {
		return "", false
	}
	if h.cursor == len(h.lines) {
		h.saved = currentInput
	}
	if h.cursor > 0 {
		h.cursor--
		return h.lines[h.cursor], true
	}
	return "", false
}

// Down navigates forward in history. Returns the history entry or the saved input.
func (h *History) Down(currentInput string) (string, bool) {
	if h.cursor >= len(h.lines) {
		return "", false
	}
	h.cursor++
	if h.cursor == len(h.lines) {
		return h.saved, true
	}
	return h.lines[h.cursor], true
}

// Reset resets the cursor to the end (called after a command is executed)
func (h *History) Reset() {
	h.cursor = len(h.lines)
	h.saved = ""
}

func (h *History) load() {
	if h.file == "" {
		return
	}
	data, err := os.ReadFile(h.file)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	for _, line := range lines {
		if line != "" {
			h.lines = append(h.lines, line)
		}
	}
	if len(h.lines) > maxHistoryEntries {
		h.lines = h.lines[len(h.lines)-maxHistoryEntries:]
	}
}

func (h *History) save() {
	if h.file == "" {
		return
	}
	data := strings.Join(h.lines, "\n") + "\n"
	os.WriteFile(h.file, []byte(data), 0600)
}
