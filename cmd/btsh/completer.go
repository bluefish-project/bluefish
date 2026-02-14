package main

import (
	"sort"
	"strings"

	"github.com/bluefish-project/bluefish/rvfs"
)

// commands that take a path argument
var pathCommands = map[string]bool{
	"cd": true, "ls": true, "ll": true, "dump": true, "open": true, "refresh": true,
}

// all commands for command-position completion
var allCommands = []string{
	"cd", "ls", "ll", "pwd", "dump", "tree", "find", "open",
	"scrape", "refresh",
	"cache", "clear", "help", "exit", "quit",
}

// computeSuggestions returns full-line suggestions for the textinput.
// Each suggestion is a complete line that replaces the entire input.
func computeSuggestions(nav *Navigator, line string, actionMode bool) []string {
	if actionMode {
		return computeActionSuggestions(nav, line)
	}

	words := strings.Fields(line)

	// Command completion
	if len(words) == 0 || (len(words) == 1 && !strings.HasSuffix(line, " ")) {
		prefix := ""
		if len(words) == 1 {
			prefix = words[0]
		}
		var suggestions []string
		for _, cmd := range allCommands {
			if strings.HasPrefix(cmd, prefix) && cmd != prefix {
				suggestions = append(suggestions, cmd)
			}
		}
		return suggestions
	}

	cmd := words[0]
	partial := ""
	if !strings.HasSuffix(line, " ") && len(words) > 1 {
		partial = words[len(words)-1]
	}

	// Path argument completion
	if pathCommands[cmd] {
		completions := completePath(nav, partial)
		// Build full-line suggestions
		linePrefix := cmd + " "
		var suggestions []string
		for _, c := range completions {
			suggestions = append(suggestions, linePrefix+c)
		}
		return suggestions
	}

	// tree depth completion
	if cmd == "tree" {
		var suggestions []string
		for _, d := range []string{"1", "2", "3", "4", "5"} {
			if strings.HasPrefix(d, partial) && d != partial {
				suggestions = append(suggestions, cmd+" "+d)
			}
		}
		return suggestions
	}

	// cache subcommand completion
	if cmd == "cache" {
		var suggestions []string
		for _, sub := range []string{"clear", "list"} {
			if strings.HasPrefix(sub, partial) && sub != partial {
				suggestions = append(suggestions, cmd+" "+sub)
			}
		}
		return suggestions
	}

	return nil
}

// completePath completes a path fragment, returning full path strings
func completePath(nav *Navigator, partial string) []string {
	// Absolute path completion from cache
	if strings.HasPrefix(partial, "/") {
		knownPaths := nav.vfs.GetKnownPaths()
		var completions []string
		for _, p := range knownPaths {
			if strings.HasPrefix(p, partial) && p+"/" != partial {
				completions = append(completions, p+"/")
			}
		}
		sort.Strings(completions)
		return completions
	}

	base, separator, prefix := splitForCompletion(partial)

	var entries []*rvfs.Entry
	var completions []string

	if base == "" {
		resolved, err := nav.vfs.ResolveTarget(rvfs.RedfishRoot, nav.cwd)
		if err != nil {
			return nil
		}
		entries = listResolved(nav.vfs, resolved)

		if strings.HasPrefix("..", prefix) {
			completions = append(completions, "../")
		}
		if strings.HasPrefix("~", prefix) {
			completions = append(completions, "~")
		}
	} else {
		target, err := nav.vfs.ResolveTarget(nav.cwd, base)
		if err != nil {
			return nil
		}
		entries = getEntriesFromTarget(nav.vfs, target, separator)
	}

	// Build base prefix for full completion strings
	basePrefix := ""
	if base != "" {
		basePrefix = base + string(separator)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name, prefix) {
			name := entry.Name + completionSuffix(entry, separator)
			completions = append(completions, basePrefix+name)
		}
	}

	sort.Strings(completions)
	return completions
}

// completionSuffix returns the appropriate suffix for tab completion
func completionSuffix(entry *rvfs.Entry, separator rune) string {
	if separator == '[' {
		return "]"
	}
	switch entry.Type {
	case rvfs.EntryLink, rvfs.EntrySymlink, rvfs.EntryComplex:
		return "/"
	case rvfs.EntryArray:
		return "["
	}
	return ""
}

// splitForCompletion splits a partial path into base, separator, and prefix
func splitForCompletion(partial string) (base string, separator rune, prefix string) {
	lastSlash := strings.LastIndex(partial, "/")
	lastBracket := strings.LastIndex(partial, "[")

	lastSep := -1
	var sep rune
	if lastSlash > lastSep {
		lastSep = lastSlash
		sep = '/'
	}
	if lastBracket > lastSep {
		lastSep = lastBracket
		sep = '['
	}

	if lastSep == -1 {
		return "", 0, partial
	}

	return partial[:lastSep], sep, partial[lastSep+1:]
}

// getEntriesFromTarget gets completable entries from a resolved target
func getEntriesFromTarget(vfs rvfs.VFS, target *rvfs.Target, separator rune) []*rvfs.Entry {
	switch target.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		entries, _ := vfs.ListAll(target.ResourcePath)
		return entries

	case rvfs.TargetProperty:
		return createEntriesFromProperty(target.Property, separator)
	}
	return nil
}

// createEntriesFromProperty creates Entry objects from a Property's children/elements
func createEntriesFromProperty(prop *rvfs.Property, separator rune) []*rvfs.Entry {
	var entries []*rvfs.Entry

	switch prop.Type {
	case rvfs.PropertyObject:
		if separator == '[' {
			return nil
		}
		for name, child := range prop.Children {
			entries = append(entries, &rvfs.Entry{
				Name: name,
				Type: entryTypeForProperty(child),
			})
		}

	case rvfs.PropertyArray:
		if separator == '/' {
			return nil
		}
		for _, elem := range prop.Elements {
			name := elem.Name
			if strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]") {
				name = name[1 : len(name)-1]
			}
			entries = append(entries, &rvfs.Entry{
				Name: name,
				Type: entryTypeForProperty(elem),
			})
		}
	}

	return entries
}

// computeActionSuggestions generates suggestions in action mode
func computeActionSuggestions(nav *Navigator, line string) []string {
	actions, _ := discoverActions(nav)

	words := strings.Fields(line)

	// Command position
	if len(words) == 0 || (len(words) == 1 && !strings.HasSuffix(line, " ")) {
		prefix := ""
		if len(words) == 1 {
			prefix = words[0]
		}
		var suggestions []string
		for _, cmd := range []string{"ls", "ll", "help", "!"} {
			if strings.HasPrefix(cmd, prefix) && cmd != prefix {
				suggestions = append(suggestions, cmd)
			}
		}
		for _, a := range actions {
			if strings.HasPrefix(a.ShortName, prefix) && a.ShortName != prefix {
				suggestions = append(suggestions, a.ShortName)
			}
		}
		sort.Strings(suggestions)
		return suggestions
	}

	// ls/ll complete action names
	if words[0] == "ls" || words[0] == "ll" {
		partial := ""
		if !strings.HasSuffix(line, " ") && len(words) > 1 {
			partial = words[len(words)-1]
		}
		var suggestions []string
		for _, a := range actions {
			if strings.HasPrefix(a.ShortName, partial) && a.ShortName != partial {
				suggestions = append(suggestions, words[0]+" "+a.ShortName)
			}
		}
		sort.Strings(suggestions)
		return suggestions
	}

	// After action name: complete parameter names and values
	action := matchAction(actions, words[0])
	if action == nil {
		return nil
	}

	partial := ""
	if !strings.HasSuffix(line, " ") && len(words) > 1 {
		partial = words[len(words)-1]
	}

	linePrefix := strings.Join(words[:len(words)-1], " ")
	if strings.HasSuffix(line, " ") {
		linePrefix = strings.TrimRight(line, " ")
	}

	// Completing a value (after =)
	if idx := strings.Index(partial, "="); idx != -1 {
		paramName := partial[:idx]
		valuePrefix := partial[idx+1:]
		if vals, ok := action.Allowable[paramName]; ok {
			var suggestions []string
			for _, v := range vals {
				if strings.HasPrefix(v, valuePrefix) && v != valuePrefix {
					suggestions = append(suggestions, linePrefix+" "+paramName+"="+v)
				}
			}
			sort.Strings(suggestions)
			return suggestions
		}
		return nil
	}

	// Complete parameter names as key=
	var suggestions []string
	for param := range action.Allowable {
		candidate := param + "="
		if strings.HasPrefix(candidate, partial) {
			suggestions = append(suggestions, linePrefix+" "+candidate)
		}
	}
	sort.Strings(suggestions)
	return suggestions
}
