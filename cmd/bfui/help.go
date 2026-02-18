package main

import (
	"fmt"
	"strings"
)

// helpContent builds the help modal text from the actual key bindings
func helpContent() string {
	var b strings.Builder

	title := actionTitleStyle.Render("Keybindings")
	b.WriteString(title)
	b.WriteString("\n\n")

	section := func(name string) {
		b.WriteString(detailLabelStyle.Render(name))
		b.WriteString("\n")
	}
	row := func(kb string, desc string) {
		b.WriteString(fmt.Sprintf("  %s  %s\n",
			helpKeyStyle.Render(fmt.Sprintf("%-14s", kb)),
			helpDescStyle.Render(desc)))
	}

	section("Navigation")
	row("j / ↓", "Move cursor down")
	row("k / ↑", "Move cursor up")
	row("h / ←", "Collapse node or move to parent")
	row("l / →", "Expand node")
	row("space", "Toggle expand / collapse")
	row("enter", "Open: rebase tree on child/link")
	row("backspace", "Back to previous root")
	row("u", "Go up to parent resource")
	row("~", "Go to root (/redfish/v1)")
	b.WriteString("\n")

	section("Details")
	row("J", "Scroll details panel down")
	row("K", "Scroll details panel up")
	b.WriteString("\n")

	section("Overlays")
	row("/", "Search cached paths (fuzzy)")
	row("!", "Action mode (POST operations)")
	row("?", "This help screen")
	b.WriteString("\n")

	section("Other")
	row("r", "Refresh current resource")
	row("s", "Scrape (crawl uncached resources)")
	row("x", "Export resources to JSON file")
	row("q / ctrl+c", "Quit")
	b.WriteString("\n")

	section("Search Mode")
	row("type", "Filter paths")
	row("ctrl+j / ↓", "Next result")
	row("ctrl+k / ↑", "Previous result")
	row("enter", "Navigate to selection")
	row("esc", "Cancel search")
	b.WriteString("\n")

	section("Action Mode")
	row("j/k", "Select action")
	row("enter", "Choose action / confirm params")
	row("tab", "Cycle allowable values")
	row("y", "Confirm POST")
	row("n / esc", "Cancel / go back")

	return b.String()
}
