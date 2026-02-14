package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bluefish-project/bluefish/rvfs"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Styles using ANSI colors 0–15 (follow terminal theme)
var (
	childStyle      = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(12)).Bold(true)
	linkStyle       = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(6))
	objectStyle     = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(5))
	propStyle       = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))
	dimStyle        = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))
	boldStyle       = lipgloss.NewStyle().Bold(true)
	warnStyle       = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3))
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)).Bold(true)
	promptPathStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(12)).Bold(true)
	promptActStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)).Bold(true)

	// Value styles
	stringValStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))
	numberValStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(4))
	trueValStyle   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(10))
	falseValStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1))
	nullValStyle   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))

	// Health-semantic styles
	healthOKStyle       = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(10))
	healthWarnStyle     = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(11))
	healthCriticalStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(9)).Bold(true)
)

// healthKeys are property names that get semantic coloring
var healthKeys = map[string]bool{
	"Health":       true,
	"HealthRollup": true,
	"State":        true,
	"Status":       true,
}

func formatEntry(entry *rvfs.Entry) string {
	switch entry.Type {
	case rvfs.EntryLink:
		return childStyle.Render(entry.Name + "/")
	case rvfs.EntrySymlink:
		return linkStyle.Render(entry.Name + "@")
	case rvfs.EntryComplex:
		return objectStyle.Render(entry.Name + "/")
	case rvfs.EntryArray:
		return objectStyle.Render(entry.Name + "[]")
	case rvfs.EntryProperty:
		return propStyle.Render(entry.Name)
	default:
		return entry.Name
	}
}

func formatColumns(items []string) string {
	if len(items) == 0 {
		return ""
	}

	width := 100
	if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil {
			width = w
		}
	}

	maxLen := 0
	for _, item := range items {
		stripped := stripAnsi(item)
		if len(stripped) > maxLen {
			maxLen = len(stripped)
		}
	}

	colWidth := maxLen + 2
	numCols := width / colWidth
	if numCols < 1 {
		numCols = 1
	}

	var result strings.Builder
	for i, item := range items {
		result.WriteString(item)
		if (i+1)%numCols == 0 {
			result.WriteString("\n")
		} else if i < len(items)-1 {
			stripped := stripAnsi(item)
			padding := colWidth - len(stripped)
			result.WriteString(strings.Repeat(" ", padding))
		}
	}

	return result.String()
}

// formatCompletionColumns lays out completion labels in terminal-width-aware
// columns, highlighting the item at selectedIdx (or none if -1).
func formatCompletionColumns(labels []string, selectedIdx int) string {
	if len(labels) == 0 {
		return ""
	}

	width := 80
	if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil {
			width = w
		}
	}

	// Find max label length for uniform column width
	maxLen := 0
	for _, label := range labels {
		if len(label) > maxLen {
			maxLen = len(label)
		}
	}

	colWidth := maxLen + 2
	numCols := (width - 2) / colWidth // -2 for leading indent
	if numCols < 1 {
		numCols = 1
	}

	var result strings.Builder
	for i, label := range labels {
		if i%numCols == 0 {
			if i > 0 {
				result.WriteString("\n")
			}
			result.WriteString("  ") // indent
		}

		// Style the label
		var styled string
		if i == selectedIdx {
			styled = compSelectedStyle.Render(label)
		} else {
			styled = compNormalStyle.Render(label)
		}
		result.WriteString(styled)

		// Pad to column width (unless last in row or last item)
		if (i+1)%numCols != 0 && i < len(labels)-1 {
			padding := colWidth - len(label)
			if padding > 0 {
				result.WriteString(strings.Repeat(" ", padding))
			}
		}
	}

	return result.String()
}

func stripAnsi(text string) string {
	var result strings.Builder
	inCode := false
	for _, ch := range text {
		if ch == '\033' {
			inCode = true
		} else if inCode {
			if ch == 'm' {
				inCode = false
			}
		} else {
			result.WriteRune(ch)
		}
	}
	return result.String()
}

func formatHealthValue(name string, value any) string {
	if healthKeys[name] {
		s, ok := value.(string)
		if ok {
			upper := strings.ToUpper(s)
			switch {
			case upper == "OK" || upper == "ENABLED" || upper == "UP":
				return healthOKStyle.Render(s)
			case upper == "WARNING" || upper == "STANDBYOFFLINE" || upper == "STARTING":
				return healthWarnStyle.Render(s)
			case upper == "CRITICAL" || upper == "DISABLED" || upper == "ABSENT":
				return healthCriticalStyle.Render(s)
			}
		}
	}
	return formatTypedValue(value)
}

func formatTypedValue(value any) string {
	switch v := value.(type) {
	case string:
		return stringValStyle.Render(v)
	case bool:
		if v {
			return trueValStyle.Render("true")
		}
		return falseValStyle.Render("false")
	case float64:
		if v == float64(int64(v)) {
			return numberValStyle.Render(fmt.Sprintf("%d", int64(v)))
		}
		return numberValStyle.Render(fmt.Sprintf("%g", v))
	case nil:
		return nullValStyle.Render("null")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatPropertyValue(prop *rvfs.Property) string {
	switch v := prop.Value.(type) {
	case string:
		if len(v) > 60 {
			return v[:57] + "..."
		}
		return v
	case bool:
		return fmt.Sprintf("%v", v)
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case nil:
		return "null"
	case []byte:
		return "{...}"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func getEntriesSummary(entries []*rvfs.Entry) string {
	children := 0
	links := 0
	properties := 0

	for _, entry := range entries {
		switch entry.Type {
		case rvfs.EntryLink:
			children++
		case rvfs.EntrySymlink:
			links++
		case rvfs.EntryProperty, rvfs.EntryComplex, rvfs.EntryArray:
			properties++
		}
	}

	var parts []string
	if children > 0 {
		parts = append(parts, fmt.Sprintf("%d children", children))
	}
	if links > 0 {
		parts = append(parts, fmt.Sprintf("%d links", links))
	}
	if properties > 0 {
		parts = append(parts, fmt.Sprintf("%d props", properties))
	}

	if len(parts) == 0 {
		return "empty"
	}
	return strings.Join(parts, ", ")
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

// showProperty writes a property in YAML-style to a builder
func showProperty(b *strings.Builder, prop *rvfs.Property, indent int, isArrayElement bool) {
	var propertyIndent string
	if isArrayElement {
		propertyIndent = ""
	} else {
		propertyIndent = strings.Repeat(" ", indent)
	}
	childIndent := strings.Repeat(" ", indent+2)

	switch prop.Type {
	case rvfs.PropertySimple:
		fmt.Fprintf(b, "%s%s: %s\n", propertyIndent, propStyle.Render(prop.Name), formatHealthValue(prop.Name, prop.Value))

	case rvfs.PropertyLink:
		fmt.Fprintf(b, "%s%s: %s → %s\n", propertyIndent, propStyle.Render(prop.Name), linkStyle.Render("link"), prop.LinkTarget)

	case rvfs.PropertyObject:
		fmt.Fprintf(b, "%s%s:", propertyIndent, propStyle.Render(prop.Name))
		if len(prop.Children) == 0 {
			fmt.Fprintf(b, " %s\n", dimStyle.Render("{}"))
		} else {
			fmt.Fprintf(b, " %s\n", dimStyle.Render(fmt.Sprintf("{%d}", len(prop.Children))))
			keys := make([]string, 0, len(prop.Children))
			for name := range prop.Children {
				keys = append(keys, name)
			}
			sort.Strings(keys)
			for _, name := range keys {
				child := prop.Children[name]
				showProperty(b, child, indent+2, false)
			}
		}

	case rvfs.PropertyArray:
		fmt.Fprintf(b, "%s%s:", propertyIndent, propStyle.Render(prop.Name))
		if len(prop.Elements) == 0 {
			fmt.Fprintf(b, " %s\n", dimStyle.Render("[]"))
		} else {
			fmt.Fprintf(b, " %s\n", dimStyle.Render(fmt.Sprintf("[%d]", len(prop.Elements))))
			for _, elem := range prop.Elements {
				if elem.Type == rvfs.PropertyObject && len(elem.Children) > 0 {
					fmt.Fprintf(b, "%s- ", childIndent)
					keys := make([]string, 0, len(elem.Children))
					for name := range elem.Children {
						keys = append(keys, name)
					}
					sort.Strings(keys)
					for i, name := range keys {
						child := elem.Children[name]
						if i == 0 {
							showProperty(b, child, indent+4, true)
						} else {
							showProperty(b, child, indent+4, false)
						}
					}
				} else {
					fmt.Fprintf(b, "%s- ", childIndent)
					switch elem.Type {
					case rvfs.PropertySimple:
						b.WriteString(formatTypedValue(elem.Value))
						b.WriteString("\n")
					case rvfs.PropertyObject:
						b.WriteString(dimStyle.Render("{}"))
						b.WriteString("\n")
					case rvfs.PropertyLink:
						fmt.Fprintf(b, "%s → %s\n", linkStyle.Render("link"), elem.LinkTarget)
					}
				}
			}
		}
	}
}

// showResource writes a resource in formatted style to a builder
func showResource(b *strings.Builder, vfs rvfs.VFS, path string) error {
	resource, err := vfs.Get(path)
	if err != nil {
		return err
	}

	b.WriteString("\n")
	b.WriteString(boldStyle.Render(path))
	b.WriteString("\n")
	if resource.ODataType != "" {
		fmt.Fprintf(b, "Type: %s\n", resource.ODataType)
	}

	if len(resource.Properties) > 0 {
		b.WriteString("\nProperties:\n")
		propNames := make([]string, 0, len(resource.Properties))
		for name := range resource.Properties {
			propNames = append(propNames, name)
		}
		sort.Strings(propNames)
		for _, name := range propNames {
			prop := resource.Properties[name]
			showProperty(b, prop, 2, false)
		}
	}

	if len(resource.Children) > 0 {
		b.WriteString("\nChildren:\n")
		childNames := make([]string, 0, len(resource.Children))
		for name := range resource.Children {
			childNames = append(childNames, name)
		}
		sort.Strings(childNames)
		for _, name := range childNames {
			child := resource.Children[name]
			if child.Type == rvfs.ChildLink {
				fmt.Fprintf(b, "  %s → %s\n", childStyle.Render(name+"/"), child.Target)
			} else {
				fmt.Fprintf(b, "  %s → %s\n", linkStyle.Render(name+"@"), child.Target)
			}
		}
	}

	return nil
}

func formatResourceAge(target *rvfs.Target) string {
	var fetchedAt time.Time
	switch target.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		if target.Resource != nil {
			fetchedAt = target.Resource.FetchedAt
		}
	case rvfs.TargetProperty:
		if target.Resource != nil {
			fetchedAt = target.Resource.FetchedAt
		}
	}
	if fetchedAt.IsZero() {
		return ""
	}
	return dimStyle.Render(formatAge(fetchedAt))
}

// formatHelp returns the help text
func formatHelp() string {
	cmd := func(s string) string { return linkStyle.Render(s) }
	arg := func(s string) string { return warnStyle.Render(s) }
	dim := func(s string) string { return dimStyle.Render(s) }

	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(boldStyle.Render("Navigation"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s %-12s %s    %s %-12s %s\n", cmd("cd"), arg("<path>"), "Navigate to resource/property", cmd("open"), arg("<path>"), "Follow link to target resource")
	fmt.Fprintf(&b, "  %s %-12s %s    %s %-12s %s\n", cmd("pwd"), "", "Print working directory", cmd("ls"), arg("[path]"), "List entries")
	fmt.Fprintf(&b, "  %s %-12s %s\n", cmd("ll"), arg("[path]"), "Show formatted content (YAML-style)")

	b.WriteString("\n")
	b.WriteString(boldStyle.Render("Viewing & Search"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s %-12s %s    %s %-12s %s\n", cmd("dump"), arg("[path]"), "Show raw JSON", cmd("tree"), arg("[depth]"), "Tree view (default: 2)")
	fmt.Fprintf(&b, "  %s %-12s %s\n", cmd("find"), arg("<pattern>"), "Search properties recursively")

	b.WriteString("\n")
	b.WriteString(boldStyle.Render("Fetching"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s %-12s %s\n", cmd("scrape"), "", "Crawl all reachable resources from cwd")
	fmt.Fprintf(&b, "  %s %-12s %s\n", cmd("refresh"), arg("[path]"), "Re-fetch a resource (invalidate + fetch)")

	b.WriteString("\n")
	b.WriteString(boldStyle.Render("Other"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s %-12s %s    %s %-12s %s\n", cmd("!"), "", "Enter action mode (POST)", cmd("cache"), arg("[cmd]"), "Cache ops (clear, list)")
	fmt.Fprintf(&b, "  %s %-12s %s    %s %s\n", cmd("clear"), "", "Clear screen", cmd("help"), dim("exit/quit"))

	b.WriteString("\n")
	b.WriteString(boldStyle.Render("Paths"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s  %s  %s  %s             %s\n",
		arg("/"), dim("separator"),
		arg("[n]"), dim("array index"),
		dim("Systems/1/Status/Health  BootOrder[0]"))
	fmt.Fprintf(&b, "  %s %s  %s  %s             %s\n",
		arg(".."), dim("parent"),
		arg("~"), dim("root (/redfish/v1)"),
		dim("open .  returns to containing resource"))

	b.WriteString("\n")
	b.WriteString(boldStyle.Render("Keys"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s  %s    %s  %s\n",
		dim("Tab"), "complete (ghost text)",
		dim("Up/Down"), "history")
	fmt.Fprintf(&b, "  %s  %s    %s  %s\n",
		dim("Ctrl+C"), "clear / exit action mode",
		dim("Ctrl+D"), "quit")

	b.WriteString("\n")
	b.WriteString(boldStyle.Render("Display"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s  %s  %s  %s  %s\n",
		childStyle.Render("dir/"), dim("child"),
		linkStyle.Render("link@"), dim("symlink"),
		propStyle.Render("prop"))
	fmt.Fprintf(&b, "  %s  %s  %s  %s\n",
		objectStyle.Render("obj/"), dim("object"),
		objectStyle.Render("arr[]"), dim("array"))
	b.WriteString("\n")

	return b.String()
}

// formatActionHelp returns action mode help text
func formatActionHelp() string {
	cmd := func(s string) string { return linkStyle.Render(s) }
	arg := func(s string) string { return warnStyle.Render(s) }

	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(boldStyle.Render("Action Mode"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s %-16s %s\n", cmd("ls"), "", "List available actions")
	fmt.Fprintf(&b, "  %s %-16s %s\n", cmd("ll"), arg("<action>"), "Show action details and parameters")
	fmt.Fprintf(&b, "  %s %-16s %s\n", cmd("<action>"), arg("[k=v ...]"), "Invoke action (with confirmation)")
	fmt.Fprintf(&b, "  %s %-16s %s\n", cmd("!"), "", "Exit action mode")
	fmt.Fprintf(&b, "  %s %-16s %s\n", cmd("help"), "", "Show this help")
	b.WriteString("\n")
	b.WriteString(boldStyle.Render("Example"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s\n", warnStyle.Render("Reset ResetType=GracefulShutdown"))
	b.WriteString("\n")

	return b.String()
}
