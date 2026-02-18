package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bluefish-project/bluefish/rvfs"
)

// executeCommandAsync returns a tea.Cmd that runs the given shell command asynchronously
func executeCommandAsync(nav *Navigator, cmd string, args []string) tea.Cmd {
	switch cmd {
	case "cd":
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return func() tea.Msg {
			output, err := nav.cd(target)
			return commandResultMsg{output: output, err: err, newCwd: nav.cwd}
		}

	case "open":
		if len(args) == 0 {
			return func() tea.Msg {
				return commandResultMsg{err: fmt.Errorf("usage: open <path>")}
			}
		}
		target := args[0]
		return func() tea.Msg {
			output, err := nav.open(target)
			return commandResultMsg{output: output, err: err, newCwd: nav.cwd}
		}

	case "ls":
		target := ""
		if len(args) > 0 {
			target = strings.Join(args, " ")
		}
		return func() tea.Msg {
			output, err := nav.ls(target)
			return commandResultMsg{output: output, err: err}
		}

	case "ll":
		target := ""
		if len(args) > 0 {
			target = strings.Join(args, " ")
		}
		return func() tea.Msg {
			output, err := nav.ll(target)
			return commandResultMsg{output: output, err: err}
		}

	case "pwd":
		return func() tea.Msg {
			return commandResultMsg{output: nav.cwd}
		}

	case "dump":
		target := ""
		if len(args) > 0 {
			target = strings.Join(args, " ")
		}
		return func() tea.Msg {
			output, err := nav.dump(target)
			return commandResultMsg{output: output, err: err}
		}

	case "tree":
		depth := 2
		if len(args) > 0 {
			if d, err := strconv.Atoi(args[0]); err == nil {
				depth = d
			}
		}
		return func() tea.Msg {
			output, err := nav.tree(depth)
			return commandResultMsg{output: output, err: err}
		}

	case "export":
		// Handled as a stepped operation in handleReadyKey
		return nil

	case "find":
		if len(args) == 0 {
			return func() tea.Msg {
				return commandResultMsg{err: fmt.Errorf("usage: find <pattern>")}
			}
		}
		// Find is handled as a stepped operation (like scrape)
		// so it needs access to state — handled in handleReadyKey
		return nil

	case "refresh":
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return func() tea.Msg {
			output, err := nav.refresh(target)
			return commandResultMsg{output: output, err: err}
		}

	case "cache":
		return func() tea.Msg {
			output, err := nav.cache(args)
			return commandResultMsg{output: output, err: err}
		}

	case "clear":
		// Handled directly in handleReadyKey
		return nil

	case "help", "?":
		return func() tea.Msg {
			return commandResultMsg{output: formatHelp()}
		}

	case "exit", "quit", "q":
		return tea.Quit

	default:
		return func() tea.Msg {
			return commandResultMsg{err: fmt.Errorf("unknown command: %s (type 'help' for commands)", cmd)}
		}
	}
}

// executeActionCommandAsync handles commands in action mode
func executeActionCommandAsync(nav *Navigator, cmd string, args []string) tea.Cmd {
	switch cmd {
	case "!":
		return func() tea.Msg {
			return commandResultMsg{output: "Exited action mode"}
		}

	case "ls":
		return func() tea.Msg {
			actions, err := discoverActions(nav)
			if err != nil {
				return commandResultMsg{err: err}
			}
			if len(args) > 0 {
				action := matchAction(actions, args[0])
				if action == nil {
					return commandResultMsg{err: fmt.Errorf("unknown action: %s", args[0])}
				}
				return commandResultMsg{output: formatActionList([]ActionInfo{*action})}
			}
			return commandResultMsg{output: formatActionList(actions)}
		}

	case "ll":
		return func() tea.Msg {
			actions, err := discoverActions(nav)
			if err != nil {
				return commandResultMsg{err: err}
			}
			if len(args) > 0 {
				action := matchAction(actions, args[0])
				if action == nil {
					return commandResultMsg{err: fmt.Errorf("unknown action: %s", args[0])}
				}
				return commandResultMsg{output: formatActionDetail(nav, action)}
			}
			var b strings.Builder
			for i := range actions {
				b.WriteString(formatActionDetail(nav, &actions[i]))
			}
			return commandResultMsg{output: b.String()}
		}

	case "help", "?":
		return func() tea.Msg {
			return commandResultMsg{output: formatActionHelp()}
		}

	case "exit", "quit", "q":
		return tea.Quit

	default:
		// Try to match as action invocation
		return func() tea.Msg {
			actions, err := discoverActions(nav)
			if err != nil {
				return commandResultMsg{err: err}
			}
			action := matchAction(actions, cmd)
			if action == nil {
				return commandResultMsg{err: fmt.Errorf("unknown action: %s (type 'help' for commands)", cmd)}
			}

			// Parse body
			jsonBody, err := parseActionBody(action, args)
			if err != nil {
				return commandResultMsg{err: err}
			}

			// Return confirmation prompt — model will handle ModeConfirm
			return actionDiscoveredMsg{
				actions: []ActionInfo{*action},
				output:  formatActionConfirm(action, jsonBody),
				confirm: true,
				body:    jsonBody,
			}
		}
	}
}

// startScrape initiates the scrape process
func startScrape(state *shellState) tea.Cmd {
	nav := state.nav

	// Build set of already cached paths
	cached := make(map[string]bool)
	for _, p := range nav.vfs.GetKnownPaths() {
		cached[p] = true
	}

	// BFS from cwd to discover uncached frontiers
	visited := make(map[string]bool)
	frontier := []string{nav.cwd}
	var queue []string

	for len(frontier) > 0 {
		p := frontier[0]
		frontier = frontier[1:]
		if visited[p] {
			continue
		}
		visited[p] = true

		if !cached[p] {
			queue = append(queue, p)
			continue
		}

		res, err := nav.vfs.Get(p)
		if err != nil {
			continue
		}
		for _, child := range res.Children {
			if !visited[child.Target] {
				frontier = append(frontier, child.Target)
			}
		}
	}

	if len(queue) == 0 {
		return func() tea.Msg {
			return commandResultMsg{output: "Everything is cached"}
		}
	}

	state.scrapeQueue = queue
	state.scrapeVisited = visited
	state.scrapeDone = 0
	state.scrapeTotal = len(queue)
	state.scrapeErrors = nil
	state.scrapeCancelled = false
	state.scrapeStart = time.Now()

	// Fetch first item
	path := state.scrapeQueue[0]
	return func() tea.Msg {
		return scrapeDoneMsg{path: path}
	}
}

// handleScrapeDone processes one scrape fetch result and chains the next
func handleScrapeDone(state *shellState, msg scrapeDoneMsg) tea.Cmd {
	if state.scrapeCancelled {
		return finishScrape(state)
	}

	// Actually fetch the resource
	nav := state.nav
	res, err := nav.vfs.Get(msg.path)

	// Remove from front of queue
	if len(state.scrapeQueue) > 0 && state.scrapeQueue[0] == msg.path {
		state.scrapeQueue = state.scrapeQueue[1:]
	}
	state.scrapeDone++

	if err != nil {
		state.scrapeErrors = append(state.scrapeErrors, fmt.Sprintf("  %s: %s", msg.path, err.Error()))
	} else {
		// Discover new children
		for _, child := range res.Children {
			if !state.scrapeVisited[child.Target] {
				state.scrapeVisited[child.Target] = true
				state.scrapeQueue = append(state.scrapeQueue, child.Target)
				state.scrapeTotal++
			}
		}
	}

	// Update spinner label
	errPart := ""
	if len(state.scrapeErrors) > 0 {
		errPart = fmt.Sprintf(", %d errors", len(state.scrapeErrors))
	}
	state.spinnerLabel = fmt.Sprintf("Fetching %s  (%d/%d%s)", msg.path, state.scrapeDone, state.scrapeTotal, errPart)

	// Chain next fetch or finish
	if len(state.scrapeQueue) == 0 {
		return finishScrape(state)
	}

	nextPath := state.scrapeQueue[0]
	return func() tea.Msg {
		return scrapeDoneMsg{path: nextPath}
	}
}

// startFind initiates a stepped find operation
func startFind(state *shellState, pattern string) (tea.Cmd, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %v", err)
	}

	resolved, err := state.nav.vfs.ResolveTarget(rvfs.RedfishRoot, state.nav.cwd)
	if err != nil {
		return nil, err
	}

	// For property targets, search synchronously (in-memory, fast)
	if resolved.Type == rvfs.TargetProperty {
		var results []string
		findInProperty(resolved.Property, "", re, &results)
		if len(results) == 0 {
			return func() tea.Msg {
				return commandResultMsg{output: fmt.Sprintf("No matches for '%s'", pattern)}
			}, nil
		}
		output := strings.Join(results, "\n")
		return func() tea.Msg {
			return commandResultMsg{output: output}
		}, nil
	}

	// For resource targets, use stepped BFS
	startPath := resolved.ResourcePath
	state.findQueue = []findQueueEntry{{path: startPath, prefix: ""}}
	state.findVisited = map[string]bool{startPath: true}
	state.findPattern = re
	state.findResults = 0
	state.findSearched = 0
	state.findTotal = 1
	state.findCancelled = false
	state.findStart = time.Now()

	return func() tea.Msg {
		return findStepMsg{path: startPath}
	}, nil
}

// handleFindStep processes one resource in the find search and chains the next.
// Returns output to print (may be empty) and the next cmd (nil when done).
func handleFindStep(state *shellState, msg findStepMsg) (string, tea.Cmd) {
	if state.findCancelled {
		return finishFind(state), nil
	}

	nav := state.nav

	// Find the queue entry for this path to get its prefix
	var prefix string
	for i, entry := range state.findQueue {
		if entry.path == msg.path {
			prefix = entry.prefix
			state.findQueue = append(state.findQueue[:i], state.findQueue[i+1:]...)
			break
		}
	}

	// Fetch and search the resource
	resource, err := nav.vfs.Get(msg.path)
	state.findSearched++

	if err != nil {
		// Update spinner and continue
		state.spinnerLabel = fmt.Sprintf("Searching  (%d found, %d/%d searched)",
			state.findResults, state.findSearched, state.findTotal)

		if len(state.findQueue) == 0 {
			return finishFind(state), nil
		}
		next := state.findQueue[0]
		return "", func() tea.Msg {
			return findStepMsg{path: next.path}
		}
	}

	// Search all properties in this resource
	var results []string
	for _, prop := range resource.Properties {
		findInProperty(prop, prefix, state.findPattern, &results)
	}
	state.findResults += len(results)

	// Enqueue children (respecting depth limit via prefix depth)
	prefixDepth := 0
	if prefix != "" {
		prefixDepth = strings.Count(prefix, "/") + 1
	}
	if prefixDepth < 5 {
		for _, child := range resource.Children {
			if !state.findVisited[child.Target] {
				state.findVisited[child.Target] = true
				childPrefix := child.Name
				if prefix != "" {
					childPrefix = prefix + "/" + child.Name
				}
				state.findQueue = append(state.findQueue, findQueueEntry{
					path:   child.Target,
					prefix: childPrefix,
				})
				state.findTotal++
			}
		}
	}

	// Update spinner
	state.spinnerLabel = fmt.Sprintf("Searching  (%d found, %d/%d searched)",
		state.findResults, state.findSearched, state.findTotal)

	// Format results from this step
	var output string
	if len(results) > 0 {
		output = strings.Join(results, "\n")
	}

	// Chain next or finish
	if len(state.findQueue) == 0 {
		summary := finishFind(state)
		if output != "" {
			output += "\n" + summary
		} else {
			output = summary
		}
		return output, nil
	}

	next := state.findQueue[0]
	return output, func() tea.Msg {
		return findStepMsg{path: next.path}
	}
}

func finishFind(state *shellState) string {
	elapsed := time.Since(state.findStart)
	if state.findCancelled {
		return fmt.Sprintf("Cancelled: %d matches, %d/%d resources searched, %s",
			state.findResults, state.findSearched, state.findTotal, elapsed.Round(time.Millisecond))
	}
	if state.findResults == 0 {
		return fmt.Sprintf("No matches (%d resources searched, %s)",
			state.findSearched, elapsed.Round(time.Millisecond))
	}
	return fmt.Sprintf("%d matches (%d resources searched, %s)",
		state.findResults, state.findSearched, elapsed.Round(time.Millisecond))
}

// startExport initiates the export process
func startExport(state *shellState, filename string) tea.Cmd {
	nav := state.nav

	if filename == "" {
		filename = "export_" + time.Now().Format("20060102T150405") + ".json"
	}

	// Build set of already cached paths
	cached := make(map[string]bool)
	for _, p := range nav.vfs.GetKnownPaths() {
		cached[p] = true
	}

	// BFS from cwd to discover all reachable paths
	visited := make(map[string]bool)
	frontier := []string{nav.cwd}
	collected := make(map[string]json.RawMessage)
	var queue []string

	for len(frontier) > 0 {
		p := frontier[0]
		frontier = frontier[1:]
		if visited[p] {
			continue
		}
		visited[p] = true

		if !cached[p] {
			queue = append(queue, p)
			continue
		}

		// Pre-collect cached resources
		res, err := nav.vfs.Get(p)
		if err != nil {
			continue
		}
		if len(res.RawJSON) > 0 {
			collected[p] = json.RawMessage(res.RawJSON)
		}
		for _, child := range res.Children {
			if !visited[child.Target] {
				frontier = append(frontier, child.Target)
			}
		}
	}

	state.exportQueue = queue
	state.exportVisited = visited
	state.exportCollected = collected
	state.exportDone = 0
	state.exportTotal = len(queue)
	state.exportErrors = nil
	state.exportCancelled = false
	state.exportStart = time.Now()
	state.exportFilename = filename

	if len(queue) == 0 {
		return finishExport(state)
	}

	// Fetch first item
	path := state.exportQueue[0]
	return func() tea.Msg {
		return exportStepMsg{path: path}
	}
}

// handleExportStep processes one export fetch and chains the next.
// Returns a tea.Cmd (nil when done).
func handleExportStep(state *shellState, msg exportStepMsg) tea.Cmd {
	if state.exportCancelled {
		return finishExport(state)
	}

	nav := state.nav
	res, err := nav.vfs.Get(msg.path)

	// Remove from front of queue
	if len(state.exportQueue) > 0 && state.exportQueue[0] == msg.path {
		state.exportQueue = state.exportQueue[1:]
	}
	state.exportDone++

	if err != nil {
		state.exportErrors = append(state.exportErrors, fmt.Sprintf("  %s: %s", msg.path, err.Error()))
	} else {
		// Collect the raw JSON
		if len(res.RawJSON) > 0 {
			state.exportCollected[msg.path] = json.RawMessage(res.RawJSON)
		}
		// Discover new children
		for _, child := range res.Children {
			if !state.exportVisited[child.Target] {
				state.exportVisited[child.Target] = true
				state.exportQueue = append(state.exportQueue, child.Target)
				state.exportTotal++
			}
		}
	}

	// Update spinner label
	errPart := ""
	if len(state.exportErrors) > 0 {
		errPart = fmt.Sprintf(", %d errors", len(state.exportErrors))
	}
	state.spinnerLabel = fmt.Sprintf("Exporting %s  (%d/%d%s)", msg.path, state.exportDone, state.exportTotal, errPart)

	// Chain next fetch or finish
	if len(state.exportQueue) == 0 {
		return finishExport(state)
	}

	nextPath := state.exportQueue[0]
	return func() tea.Msg {
		return exportStepMsg{path: nextPath}
	}
}

// finishExport writes collected data to a JSON file and returns a result message
func finishExport(state *shellState) tea.Cmd {
	elapsed := time.Since(state.exportStart)

	if state.exportCancelled {
		// Clean up state, no file written
		output := fmt.Sprintf("Export cancelled: %d fetched, %d collected, %s",
			state.exportDone, len(state.exportCollected), elapsed.Round(time.Millisecond))
		state.exportQueue = nil
		state.exportVisited = nil
		state.exportCollected = nil
		state.exportErrors = nil
		return func() tea.Msg {
			return commandResultMsg{output: output}
		}
	}

	// Write JSON file
	data, err := json.MarshalIndent(state.exportCollected, "", "  ")
	if err != nil {
		state.exportQueue = nil
		state.exportVisited = nil
		state.exportCollected = nil
		state.exportErrors = nil
		return func() tea.Msg {
			return commandResultMsg{err: fmt.Errorf("marshal failed: %v", err)}
		}
	}

	filename := state.exportFilename
	writeErr := os.WriteFile(filename, data, 0644)

	var b strings.Builder
	if writeErr != nil {
		fmt.Fprintf(&b, "Error writing %s: %v", filename, writeErr)
	} else {
		fmt.Fprintf(&b, "Exported %d resources to %s (%s)", len(state.exportCollected), filename, elapsed.Round(time.Millisecond))
	}
	for _, msg := range state.exportErrors {
		b.WriteString("\n")
		b.WriteString(msg)
	}

	// Clean up export state
	state.exportQueue = nil
	state.exportVisited = nil
	state.exportCollected = nil
	state.exportErrors = nil

	output := b.String()
	return func() tea.Msg {
		return commandResultMsg{output: output}
	}
}

func finishScrape(state *shellState) tea.Cmd {
	elapsed := time.Since(state.scrapeStart)
	var b strings.Builder
	if state.scrapeCancelled {
		fmt.Fprintf(&b, "Cancelled: %d fetched, %d errors, %s", state.scrapeDone, len(state.scrapeErrors), elapsed.Round(time.Millisecond))
	} else {
		fmt.Fprintf(&b, "Done: %d fetched, %d errors, %s", state.scrapeDone, len(state.scrapeErrors), elapsed.Round(time.Millisecond))
	}
	for _, msg := range state.scrapeErrors {
		b.WriteString("\n")
		b.WriteString(msg)
	}

	// Clean up scrape state so stale queue doesn't trigger false cancellations
	state.scrapeQueue = nil
	state.scrapeVisited = nil
	state.scrapeErrors = nil

	output := b.String()
	return func() tea.Msg {
		return commandResultMsg{output: output}
	}
}
