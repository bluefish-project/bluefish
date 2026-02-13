package main

import (
	"strings"

	"bluefish/rvfs"
)

// CompletionListener preprocesses Tab key presses
type CompletionListener struct {
	nav *Navigator
}

// NewCompletionListener creates a listener that pre-fetches resources on Tab
func NewCompletionListener(nav *Navigator) *CompletionListener {
	return &CompletionListener{nav: nav}
}

// OnChange is called on every keystroke
func (c *CompletionListener) OnChange(line []rune, pos int, key rune) ([]rune, int, bool) {
	// Only intercept Tab key
	if key != '\t' {
		return line, pos, false
	}

	// Parse the input
	text := string(line[:pos])
	words := strings.Fields(text)

	// Only handle path completion commands
	if len(words) < 1 {
		return line, pos, false
	}

	cmd := words[0]
	if cmd != "cd" && cmd != "ls" && cmd != "ll" && cmd != "dump" {
		return line, pos, false
	}

	// Extract partial path
	var partial string
	if len(words) > 1 && !strings.HasSuffix(text, " ") {
		partial = words[len(words)-1]
	}

	// Pre-fetch using shared resolution logic
	c.prefetch(partial)

	// Return false to let readline continue with completion
	return line, pos, false
}

// prefetch pre-fetches the target resource
func (c *CompletionListener) prefetch(partial string) {
	// Absolute paths - try direct fetch
	if strings.HasPrefix(partial, "/") {
		c.nav.vfs.Get(partial)
		return
	}

	// Split partial to find the base path
	base, _, _ := splitForCompletion(partial)

	// If we have a base path, try to resolve and fetch it
	if base != "" {
		target, err := c.nav.vfs.ResolveTarget(c.nav.cwd, base)
		if err != nil {
			return
		}

		// Only prefetch if it's a resource or link (properties are already in memory)
		if target.Type == rvfs.TargetResource || target.Type == rvfs.TargetLink {
			c.nav.vfs.Get(target.ResourcePath)
		}
	}
}
