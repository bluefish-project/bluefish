package main

import (
	"sort"
	"strings"

	"bluefish/rvfs"
)

// Completer provides tab completion for the shell
type Completer struct {
	nav *Navigator
}

// NewCompleter creates a new completer
func NewCompleter(nav *Navigator) *Completer {
	return &Completer{nav: nav}
}

// Do implements readline.AutoCompleter interface
func (c *Completer) Do(line []rune, pos int) ([][]rune, int) {
	text := string(line[:pos])
	words := strings.Fields(text)

	// Command completion
	if len(words) == 0 || (len(words) == 1 && !strings.HasSuffix(text, " ")) {
		return c.completeCommand(words)
	}

	// Argument completion
	cmd := words[0]
	partial := ""
	if !strings.HasSuffix(text, " ") && len(words) > 1 {
		partial = words[len(words)-1]
	}

	switch cmd {
	case "cd", "ls", "ll", "dump", "open":
		return c.completePath(partial)
	case "tree":
		return c.completeTreeDepth()
	case "cache":
		return c.completeCacheCommand()
	}

	return nil, 0
}

// completeCommand completes command names
func (c *Completer) completeCommand(words []string) ([][]rune, int) {
	commands := []string{
		"cd", "ls", "ll", "pwd", "dump", "tree", "find", "open",
		"flat", "cache", "clear", "help", "exit", "quit",
	}

	prefix := ""
	if len(words) == 1 {
		prefix = words[0]
	}

	var matches []string
	for _, cmd := range commands {
		if strings.HasPrefix(cmd, prefix) {
			matches = append(matches, cmd)
		}
	}

	return toRuneSlices(matches, len(prefix)), len(prefix)
}

// completePath completes paths using ResolveTarget
func (c *Completer) completePath(partial string) ([][]rune, int) {
	var completions []string

	// Handle absolute path completion from cache
	if strings.HasPrefix(partial, "/") {
		knownPaths := c.nav.vfs.GetKnownPaths()
		for _, p := range knownPaths {
			if strings.HasPrefix(p, partial) {
				completions = append(completions, p)
			}
		}
		return toRuneSlices(completions, len(partial)), len(partial)
	}

	// Split partial into base path, separator type, and prefix to complete
	base, separator, prefix := c.splitForCompletion(partial)

	// Resolve the base path (or use cwd if empty)
	var entries []*rvfs.Entry
	var err error

	if base == "" {
		// Complete at current location
		entries, err = c.nav.vfs.ListAll(c.nav.cwd)
		if err != nil {
			return nil, 0
		}
		// Add special paths for resource navigation
		if strings.HasPrefix("..", prefix) {
			completions = append(completions, "..")
		}
		if strings.HasPrefix("~", prefix) {
			completions = append(completions, "~")
		}
	} else {
		// Resolve the base path
		target, err := c.nav.vfs.ResolveTarget(c.nav.cwd, base)
		if err != nil {
			return nil, 0
		}

		// Get entries based on target type and separator
		entries, err = c.getEntriesFromTarget(target, separator)
		if err != nil {
			return nil, 0
		}
	}

	// Filter entries by prefix
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name, prefix) {
			completions = append(completions, entry.Name)
		}
	}

	sort.Strings(completions)
	return toRuneSlices(completions, len(prefix)), len(prefix)
}

// splitForCompletion splits a partial path into base, separator, and prefix
// The separator indicates what kind of completion is expected:
//   '/' → resource children
//   ':' → property children (named fields)
//   '[' → array indices
// Examples:
//   "Status:Hea" → ("Status", ':', "Hea")
//   "Boot:" → ("Boot", ':', "")
//   "Boot:BootOrder[" → ("Boot:BootOrder", '[', "")
//   "Systems/1" → ("Systems", '/', "1")
//   "Status" → ("", 0, "Status")
//   "" → ("", 0, "")
func (c *Completer) splitForCompletion(partial string) (base string, separator rune, prefix string) {
	// Find the last separator (: / or [)
	lastColon := strings.LastIndex(partial, ":")
	lastSlash := strings.LastIndex(partial, "/")
	lastBracket := strings.LastIndex(partial, "[")

	// Find the rightmost separator
	lastSep := -1
	var sep rune
	if lastColon > lastSep {
		lastSep = lastColon
		sep = ':'
	}
	if lastSlash > lastSep {
		lastSep = lastSlash
		sep = '/'
	}
	if lastBracket > lastSep {
		lastSep = lastBracket
		sep = '['
	}

	if lastSep == -1 {
		// No separator - completing at current level
		return "", 0, partial
	}

	// Split at the separator
	return partial[:lastSep], sep, partial[lastSep+1:]
}

// getEntriesFromTarget gets completable entries from a resolved target
func (c *Completer) getEntriesFromTarget(target *rvfs.Target, separator rune) ([]*rvfs.Entry, error) {
	switch target.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		// For resources and links, list the resource entries
		return c.nav.vfs.ListAll(target.ResourcePath)

	case rvfs.TargetProperty:
		// For properties, create entries from the property's structure
		// The separator tells us what kind of entries to return
		return c.createEntriesFromProperty(target.Property, separator), nil

	default:
		return nil, nil
	}
}

// createEntriesFromProperty creates Entry objects from a Property's children/elements
// The separator indicates what kind of completion is expected:
//   ':' → property children (only valid for PropertyObject)
//   '[' → array indices (only valid for PropertyArray)
//   0   → no separator, return appropriate entries for property type
func (c *Completer) createEntriesFromProperty(prop *rvfs.Property, separator rune) []*rvfs.Entry {
	var entries []*rvfs.Entry

	switch prop.Type {
	case rvfs.PropertyObject:
		// Objects have named child properties
		// Only return them if separator is ':' or no separator
		if separator == '[' {
			// Can't use [ on an object
			return nil
		}

		for name, child := range prop.Children {
			entryType := rvfs.EntryProperty
			if child.Type == rvfs.PropertyLink {
				entryType = rvfs.EntrySymlink
			} else if child.Type != rvfs.PropertySimple {
				entryType = rvfs.EntryComplex
			}
			entries = append(entries, &rvfs.Entry{
				Name: name,
				Type: entryType,
			})
		}

	case rvfs.PropertyArray:
		// Arrays have indexed elements
		// Only return them if separator is '[' or no separator
		if separator == ':' {
			// Can't use : on an array - must use [
			return nil
		}

		// Return array indices as bare numbers (strip brackets from [0], [1], etc.)
		for _, elem := range prop.Elements {
			entryType := rvfs.EntryProperty
			if elem.Type != rvfs.PropertySimple {
				entryType = rvfs.EntryComplex
			}

			// Strip brackets from element name to get bare index
			name := elem.Name
			if strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]") {
				name = name[1 : len(name)-1]
			}

			entries = append(entries, &rvfs.Entry{
				Name: name,
				Type: entryType,
			})
		}
	}

	return entries
}

// completeTreeDepth completes tree depth arguments
func (c *Completer) completeTreeDepth() ([][]rune, int) {
	depths := []string{"1", "2", "3", "4", "5"}
	return toRuneSlices(depths, 0), 0
}

// completeCacheCommand completes cache subcommands
func (c *Completer) completeCacheCommand() ([][]rune, int) {
	cmds := []string{"clear", "list"}
	return toRuneSlices(cmds, 0), 0
}

// toRuneSlices converts string completions to rune slices
func toRuneSlices(strs []string, prefixLen int) [][]rune {
	result := make([][]rune, len(strs))
	for i, s := range strs {
		result[i] = []rune(s[prefixLen:])
	}
	return result
}
