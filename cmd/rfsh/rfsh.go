package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"bluefish/rvfs"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Colors for output
var (
	colorCyan     = color.New(color.FgCyan)
	colorGreen    = color.New(color.FgGreen)
	colorPurple   = color.New(color.FgMagenta)
	colorYellow   = color.New(color.FgYellow)
	colorBold     = color.New(color.Bold)
	colorBoldBlue = color.New(color.FgBlue, color.Bold)
	colorBoldRed  = color.New(color.FgRed, color.Bold)
	colorRed      = color.New(color.FgRed)
)

// Config holds connection configuration
type Config struct {
	Endpoint string `yaml:"endpoint"`
	User     string `yaml:"user"`
	Pass     string `yaml:"pass"`
	Insecure bool   `yaml:"insecure"`
}

// loadConfig reads configuration from a YAML file
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("config missing required field: endpoint")
	}
	if cfg.User == "" {
		return nil, fmt.Errorf("config missing required field: user")
	}
	if cfg.Pass == "" {
		return nil, fmt.Errorf("config missing required field: pass")
	}

	return &cfg, nil
}

// Navigator manages shell state
type Navigator struct {
	vfs        rvfs.VFS
	cwd        string
	actionMode bool
}

// NewNavigator creates a navigator
func NewNavigator(vfs rvfs.VFS) *Navigator {
	return &Navigator{
		vfs: vfs,
		cwd: "/redfish/v1",
	}
}

// cd changes directory
func (n *Navigator) cd(target string) error {
	if target == "" || target == "~" {
		n.cwd = "/redfish/v1"
		resolved, _ := n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
		entries := n.listResolved(resolved)
		fmt.Printf("%s  (%s)\n", n.cwd, getEntriesSummary(entries))
		return nil
	}

	if target == "." {
		resolved, _ := n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
		entries := n.listResolved(resolved)
		fmt.Printf("%s  (%s)\n", n.cwd, getEntriesSummary(entries))
		return nil
	}

	if target == ".." {
		n.cwd = n.vfs.Parent(n.cwd)
		resolved, _ := n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
		entries := n.listResolved(resolved)
		fmt.Printf("%s  (%s)\n", n.cwd, getEntriesSummary(entries))
		return nil
	}

	resolvedTarget, err := n.vfs.ResolveTarget(n.cwd, target)
	if err != nil {
		return err
	}

	switch resolvedTarget.Type {
	case rvfs.TargetResource:
		n.cwd = resolvedTarget.ResourcePath

	case rvfs.TargetLink:
		n.cwd = resolvedTarget.ResourcePath

	case rvfs.TargetProperty:
		switch resolvedTarget.Property.Type {
		case rvfs.PropertyObject, rvfs.PropertyArray:
			// Navigate into property — compose the full path
			if strings.HasPrefix(target, "/") {
				n.cwd = strings.TrimRight(target, "/")
			} else {
				n.cwd = n.cwd + "/" + target
			}
		default:
			return fmt.Errorf("cannot cd to value: %s", target)
		}
	}

	entries := n.listResolved(resolvedTarget)
	fmt.Printf("%s  (%s)\n", n.cwd, getEntriesSummary(entries))
	return nil
}

// open follows links to their canonical destinations (always canonicalizes PropertyLinks)
func (n *Navigator) open(target string) error {
	if target == "" {
		return fmt.Errorf("open requires a target path")
	}

	// Resolve the target
	resolvedTarget, err := n.vfs.ResolveTarget(n.cwd, target)
	if err != nil {
		// Special case: "open ." from a property path
		if target == "." {
			resolvedTarget, err = n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	switch resolvedTarget.Type {
	case rvfs.TargetResource:
		n.cwd = resolvedTarget.ResourcePath
		entries, _ := n.vfs.ListAll(n.cwd)
		fmt.Printf("%s  (%s)\n", n.cwd, getEntriesSummary(entries))

	case rvfs.TargetLink:
		n.cwd = resolvedTarget.ResourcePath
		entries, _ := n.vfs.ListAll(n.cwd)
		fmt.Printf("%s  (%s)\n", n.cwd, getEntriesSummary(entries))

	case rvfs.TargetProperty:
		prop := resolvedTarget.Property
		if prop.Type == rvfs.PropertyLink {
			// Follow the link
			n.cwd = prop.LinkTarget
			entries, _ := n.vfs.ListAll(n.cwd)
			fmt.Printf("%s  (%s)\n", n.cwd, getEntriesSummary(entries))
		} else if target == "." {
			// "open ." from a property path — navigate to containing resource
			n.cwd = resolvedTarget.Resource.Path
			entries, _ := n.vfs.ListAll(n.cwd)
			fmt.Printf("%s  (%s)\n", n.cwd, getEntriesSummary(entries))
		} else {
			return fmt.Errorf("cannot open property %s (not a link; use 'cd' to navigate into objects)", target)
		}
	}

	return nil
}

// ls lists all entries (children + properties)
func (n *Navigator) ls(target string) error {
	if target == "." {
		target = ""
	}

	// Resolve the path
	var resolved *rvfs.Target
	var err error
	if target == "" {
		resolved, err = n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
	} else {
		resolved, err = n.vfs.ResolveTarget(n.cwd, target)
	}
	if err != nil {
		return err
	}

	entries := n.listResolved(resolved)
	n.printShortListingAll(entries)
	return nil
}

// entriesFromProperty creates Entry list from a property's children/elements
func entriesFromProperty(prop *rvfs.Property) []*rvfs.Entry {
	var entries []*rvfs.Entry

	switch prop.Type {
	case rvfs.PropertyObject:
		for name, child := range prop.Children {
			entries = append(entries, &rvfs.Entry{
				Name: name,
				Path: child.LinkTarget,
				Type: entryTypeForProperty(child),
			})
		}
	case rvfs.PropertyArray:
		for _, elem := range prop.Elements {
			entries = append(entries, &rvfs.Entry{
				Name: elem.Name,
				Type: entryTypeForProperty(elem),
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// entryTypeForProperty maps property types to entry types
func entryTypeForProperty(prop *rvfs.Property) rvfs.EntryType {
	switch prop.Type {
	case rvfs.PropertyObject:
		return rvfs.EntryComplex
	case rvfs.PropertyArray:
		return rvfs.EntryArray
	case rvfs.PropertyLink:
		return rvfs.EntrySymlink
	default:
		return rvfs.EntryProperty
	}
}

// listResolved returns entries for any resolved target
func (n *Navigator) listResolved(target *rvfs.Target) []*rvfs.Entry {
	switch target.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		entries, _ := n.vfs.ListAll(target.ResourcePath)
		return entries
	case rvfs.TargetProperty:
		return entriesFromProperty(target.Property)
	}
	return nil
}

// dump displays raw JSON
func (n *Navigator) dump(target string) error {
	// Resolve the path
	var resolved *rvfs.Target
	var err error
	if target == "" {
		resolved, err = n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
	} else {
		resolved, err = n.vfs.ResolveTarget(n.cwd, target)
	}
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	switch resolved.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		json.Indent(&buf, resolved.Resource.RawJSON, "", "  ")
	case rvfs.TargetProperty:
		json.Indent(&buf, resolved.Property.RawJSON, "", "  ")
	}
	fmt.Println(buf.String())
	return nil
}

// ll displays formatted content using parsed structure
func (n *Navigator) ll(target string) error {
	if target == "." {
		target = ""
	}

	// Resolve the path
	var resolved *rvfs.Target
	var err error
	if target == "" {
		resolved, err = n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
	} else {
		resolved, err = n.vfs.ResolveTarget(n.cwd, target)
	}
	if err != nil {
		return err
	}

	switch resolved.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		return n.showResource(resolved.ResourcePath)
	case rvfs.TargetProperty:
		n.showProperty(resolved.Property, 0, false)
	}
	return nil
}

// showResource displays a resource in formatted style
func (n *Navigator) showResource(path string) error {
	resource, err := n.vfs.Get(path)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(colorBold.Sprint(path))
	if resource.ODataType != "" {
		fmt.Printf("Type: %s\n", resource.ODataType)
	}

	// Show properties (sorted for deterministic output)
	if len(resource.Properties) > 0 {
		fmt.Println("\nProperties:")

		// Sort property names
		propNames := make([]string, 0, len(resource.Properties))
		for name := range resource.Properties {
			propNames = append(propNames, name)
		}
		sort.Strings(propNames)

		for _, name := range propNames {
			prop := resource.Properties[name]
			n.showProperty(prop, 2, false)
		}
	}

	// Show children (sorted for deterministic output)
	if len(resource.Children) > 0 {
		fmt.Println("\nChildren:")

		// Sort child names
		childNames := make([]string, 0, len(resource.Children))
		for name := range resource.Children {
			childNames = append(childNames, name)
		}
		sort.Strings(childNames)

		for _, name := range childNames {
			child := resource.Children[name]
			if child.Type == rvfs.ChildLink {
				fmt.Printf("  %s → %s\n", colorBoldBlue.Sprintf("%s/", name), child.Target)
			} else {
				fmt.Printf("  %s → %s\n", colorCyan.Sprintf("%s@", name), child.Target)
			}
		}
	}

	return nil
}

// showProperty displays a property in formatted style with indentation (YAML-style)
// indent is the indentation level for this property itself
// isArrayElement indicates this property is the first field of an array element object (suppress indent)
func (n *Navigator) showProperty(prop *rvfs.Property, indent int, isArrayElement bool) {
	var propertyIndent string
	if isArrayElement {
		propertyIndent = "" // No indent for first field of array element (inline with dash)
	} else {
		propertyIndent = strings.Repeat(" ", indent)
	}
	childIndent := strings.Repeat(" ", indent+2)

	switch prop.Type {
	case rvfs.PropertySimple:
		// Print property name and simple value inline
		fmt.Printf("%s%s: %s\n", propertyIndent, colorGreen.Sprint(prop.Name), formatSimpleValue(prop.Value))

	case rvfs.PropertyLink:
		// Print property name and link target
		fmt.Printf("%s%s: %s → %s\n", propertyIndent, colorGreen.Sprint(prop.Name), colorCyan.Sprint("link"), prop.LinkTarget)

	case rvfs.PropertyObject:
		// Print property name
		fmt.Printf("%s%s:", propertyIndent, colorGreen.Sprint(prop.Name))

		// Object - show nested fields with indentation (YAML-style)
		if len(prop.Children) == 0 {
			// Empty object
			fmt.Println(" {}")
		} else {
			// Print leading newline
			fmt.Println()

			// Sort keys for deterministic output
			keys := make([]string, 0, len(prop.Children))
			for name := range prop.Children {
				keys = append(keys, name)
			}
			sort.Strings(keys)

			// Print fields
			for _, name := range keys {
				child := prop.Children[name]
				n.showProperty(child, indent+2, false)
			}
		}

	case rvfs.PropertyArray:
		// Print property name
		fmt.Printf("%s%s:", propertyIndent, colorGreen.Sprint(prop.Name))

		// Array - show elements with YAML-style list markers
		if len(prop.Elements) == 0 {
			// Empty array
			fmt.Println(" []")
		} else {
			fmt.Println()
			// Print each element with dash marker
			for _, elem := range prop.Elements {
				// For array elements, we need special handling for objects
				if elem.Type == rvfs.PropertyObject && len(elem.Children) > 0 {
					// Print dash at child indent level
					fmt.Printf("%s- ", childIndent)

					// Print first field inline with dash, rest indented
					keys := make([]string, 0, len(elem.Children))
					for name := range elem.Children {
						keys = append(keys, name)
					}
					sort.Strings(keys)

					for i, name := range keys {
						child := elem.Children[name]
						if i == 0 {
							// First field inline with dash (at childIndent level, but suppress indent)
							n.showProperty(child, indent+4, true)
						} else {
							// Subsequent fields indented to align with first field
							n.showProperty(child, indent+4, false)
						}
					}
				} else {
					// Simple element or empty object - show inline
					fmt.Printf("%s- ", childIndent)
					switch elem.Type {
					case rvfs.PropertySimple:
						fmt.Println(formatSimpleValue(elem.Value))
					case rvfs.PropertyObject:
						fmt.Println("{}")
					case rvfs.PropertyLink:
						fmt.Printf("%s → %s\n", colorCyan.Sprint("link"), elem.LinkTarget)
					}
				}
			}
		}
	}
}

// formatSimpleValue formats a simple property value
func formatSimpleValue(value any) string {
	switch v := value.(type) {
	case string:
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
	default:
		return fmt.Sprintf("%v", v)
	}
}

// tree displays tree view
func (n *Navigator) tree(depth int) error {
	resolved, err := n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
	if err != nil {
		return err
	}

	var entries []*rvfs.Entry
	switch resolved.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		entries, _ = n.vfs.ListAll(resolved.ResourcePath)
	case rvfs.TargetProperty:
		entries = entriesFromProperty(resolved.Property)
	}

	output := n.buildTreeFromEntries(n.cwd, entries, "", depth, 0)
	if output == "" {
		fmt.Println("(empty)")
	} else {
		fmt.Println(output)
	}
	return nil
}

func (n *Navigator) buildTreeFromEntries(basePath string, entries []*rvfs.Entry, prefix string, maxDepth, currentDepth int) string {
	if currentDepth >= maxDepth {
		return ""
	}

	var lines []string
	for i, entry := range entries {
		isLast := i == len(entries)-1

		connector := "├── "
		if isLast {
			connector = "└── "
		}

		line := prefix + connector + formatEntry(entry)
		lines = append(lines, line)

		// Recurse for directories
		if entry.IsDir() && currentDepth+1 < maxDepth {
			extension := "│   "
			if isLast {
				extension = "    "
			}

			// Resolve child to get its entries
			childPath := entry.Path
			if childPath == "" {
				childPath = basePath + "/" + entry.Name
			}

			resolved, err := n.vfs.ResolveTarget(rvfs.RedfishRoot, childPath)
			if err != nil {
				continue
			}

			var childEntries []*rvfs.Entry
			switch resolved.Type {
			case rvfs.TargetResource, rvfs.TargetLink:
				childEntries, _ = n.vfs.ListAll(resolved.ResourcePath)
			case rvfs.TargetProperty:
				childEntries = entriesFromProperty(resolved.Property)
			}

			subtree := n.buildTreeFromEntries(childPath, childEntries, prefix+extension, maxDepth, currentDepth+1)
			if subtree != "" {
				lines = append(lines, subtree)
			}
		}
	}

	return strings.Join(lines, "\n")
}

// find searches for properties recursively
func (n *Navigator) find(pattern string) error {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %v", err)
	}

	resolved, err := n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
	if err != nil {
		return err
	}

	var results []string

	switch resolved.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		n.findInResource(resolved.ResourcePath, "", re, &results, 0)
	case rvfs.TargetProperty:
		findInProperty(resolved.Property, "", re, &results)
	}

	if len(results) == 0 {
		fmt.Printf("No matches found for '%s'\n", pattern)
	} else {
		for _, result := range results {
			fmt.Println(result)
		}
	}

	return nil
}

func (n *Navigator) findInResource(resourcePath, prefix string, re *regexp.Regexp, results *[]string, depth int) {
	if depth > 5 {
		return
	}

	resource, err := n.vfs.Get(resourcePath)
	if err != nil {
		return
	}

	// Search all properties in this resource
	for _, prop := range resource.Properties {
		findInProperty(prop, prefix, re, results)
	}

	// Recurse into child resources
	for _, child := range resource.Children {
		childPrefix := child.Name
		if prefix != "" {
			childPrefix = prefix + "/" + child.Name
		}
		n.findInResource(child.Target, childPrefix, re, results, depth+1)
	}
}

func findInProperty(prop *rvfs.Property, prefix string, re *regexp.Regexp, results *[]string) {
	fullPath := prop.Name
	if prefix != "" {
		fullPath = prefix + "/" + prop.Name
	}

	if re.MatchString(prop.Name) {
		*results = append(*results,
			fmt.Sprintf("%s = %s",
				colorYellow.Sprint(fullPath),
				formatPropertyValue(prop)))
	}

	// Recurse into children
	switch prop.Type {
	case rvfs.PropertyObject:
		for _, child := range prop.Children {
			findInProperty(child, fullPath, re, results)
		}
	case rvfs.PropertyArray:
		for _, elem := range prop.Elements {
			findInProperty(elem, fullPath, re, results)
		}
	}
}

// ActionInfo describes a Redfish action on a resource
type ActionInfo struct {
	Name      string              // Full name (e.g. #ComputerSystem.Reset)
	ShortName string              // Stripped name (e.g. Reset)
	Target    string              // POST URI
	InfoURI   string              // @Redfish.ActionInfo URI (may be empty)
	Allowable map[string][]string // Parameter name → AllowableValues
}

// discoverActions finds all actions on the resource at nav.cwd
func discoverActions(nav *Navigator) ([]ActionInfo, error) {
	resolved, err := nav.vfs.ResolveTarget(rvfs.RedfishRoot, nav.cwd)
	if err != nil {
		return nil, err
	}

	var resource *rvfs.Resource
	switch resolved.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		resource = resolved.Resource
		if resource == nil {
			resource, err = nav.vfs.Get(resolved.ResourcePath)
			if err != nil {
				return nil, err
			}
		}
	case rvfs.TargetProperty:
		resource = resolved.Resource
	}
	if resource == nil {
		return nil, nil
	}

	actionsProp, ok := resource.Properties["Actions"]
	if !ok || actionsProp.Type != rvfs.PropertyObject {
		return nil, nil
	}

	var actions []ActionInfo
	for key, child := range actionsProp.Children {
		if key == "Oem" {
			continue
		}

		info := ActionInfo{
			Name:      key,
			Allowable: make(map[string][]string),
		}

		// Extract short name: strip #Type. prefix
		if idx := strings.LastIndex(key, "."); idx != -1 && strings.HasPrefix(key, "#") {
			info.ShortName = key[idx+1:]
		} else {
			info.ShortName = key
		}

		if child.Type != rvfs.PropertyObject {
			continue
		}

		// Extract target, ActionInfo URI, and AllowableValues from children
		for childKey, childProp := range child.Children {
			if childKey == "target" && childProp.Type == rvfs.PropertyLink {
				info.Target = childProp.LinkTarget
			} else if childKey == "@Redfish.ActionInfo" && childProp.Type == rvfs.PropertyLink {
				info.InfoURI = childProp.LinkTarget
			} else if strings.HasSuffix(childKey, "@Redfish.AllowableValues") && childProp.Type == rvfs.PropertyArray {
				paramName := strings.TrimSuffix(childKey, "@Redfish.AllowableValues")
				var values []string
				for _, elem := range childProp.Elements {
					if elem.Type == rvfs.PropertySimple {
						if s, ok := elem.Value.(string); ok {
							values = append(values, s)
						}
					}
				}
				info.Allowable[paramName] = values
			}
		}

		if info.Target != "" {
			actions = append(actions, info)
		}
	}

	sort.Slice(actions, func(i, j int) bool {
		return actions[i].ShortName < actions[j].ShortName
	})
	return actions, nil
}

// matchAction finds an action by short name or full name (case-insensitive)
func matchAction(actions []ActionInfo, name string) *ActionInfo {
	lower := strings.ToLower(name)
	for i := range actions {
		if strings.ToLower(actions[i].ShortName) == lower {
			return &actions[i]
		}
	}
	for i := range actions {
		if strings.ToLower(actions[i].Name) == lower {
			return &actions[i]
		}
	}
	return nil
}

// Display formatting

func (n *Navigator) printShortListingAll(entries []*rvfs.Entry) {
	if len(entries) == 0 {
		fmt.Println("(empty)")
		return
	}

	items := make([]string, len(entries))
	for i, entry := range entries {
		items[i] = formatEntry(entry)
	}

	fmt.Println(formatColumns(items))
}

func formatEntry(entry *rvfs.Entry) string {
	switch entry.Type {
	case rvfs.EntryLink:
		return colorBoldBlue.Sprintf("%s/", entry.Name)
	case rvfs.EntrySymlink:
		return colorCyan.Sprintf("%s@", entry.Name)
	case rvfs.EntryComplex:
		return colorPurple.Sprintf("%s/", entry.Name)
	case rvfs.EntryArray:
		return colorPurple.Sprintf("%s[]", entry.Name)
	case rvfs.EntryProperty:
		return colorGreen.Sprint(entry.Name)
	default:
		return entry.Name
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

func main() {
	// Parse arguments: config file only
	if len(os.Args) != 2 {
		fmt.Println("Usage: rfsh CONFIG_FILE")
		fmt.Println("Example: rfsh config.yaml")
		os.Exit(1)
	}

	configPath := os.Args[1]

	// Check if it's a YAML file
	if !strings.HasSuffix(configPath, ".yaml") && !strings.HasSuffix(configPath, ".yml") {
		fmt.Println("Usage: rfsh CONFIG_FILE")
		fmt.Println("Example: rfsh config.yaml")
		os.Exit(1)
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	endpoint := cfg.Endpoint
	username := cfg.User
	password := cfg.Pass
	insecure := cfg.Insecure

	// Create VFS
	fmt.Printf("Connecting to %s...\n", endpoint)
	vfs, err := rvfs.NewVFS(endpoint, username, password, insecure)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	defer vfs.Sync()

	// Create navigator
	nav := NewNavigator(vfs)

	// Show initial status
	entries, _ := vfs.ListAll(nav.cwd)
	summary := getEntriesSummary(entries)
	fmt.Printf("%s  (%s)\n", nav.cwd, summary)
	fmt.Println("Type 'help' for commands")

	// Setup readline with completion preprocessing
	completer := NewCompleter(nav)
	listener := NewCompletionListener(nav)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:            getPrompt(nav),
		HistoryFile:       os.ExpandEnv("$HOME/.rfsh_history"),
		AutoComplete:      completer,
		Listener:          listener,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
		HistoryLimit:      1000,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	defer rl.Close()

	// REPL loop
	for {
		rl.SetPrompt(getPrompt(nav))

		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt && nav.actionMode {
				nav.actionMode = false
				fmt.Println("Exited action mode")
				continue
			}
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Enter action mode
		if line == "!" && !nav.actionMode {
			actions, err := discoverActions(nav)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			if len(actions) == 0 {
				fmt.Println("No actions on current resource")
				continue
			}
			nav.actionMode = true
			printActionList(actions)
			continue
		}

		// Parse command
		parts := strings.Fields(line)
		cmd := parts[0]
		args := parts[1:]

		if nav.actionMode {
			if cmd == "!" {
				nav.actionMode = false
				fmt.Println("Exited action mode")
				continue
			}
			if cmd == "exit" || cmd == "quit" || cmd == "q" {
				break
			}
			if err := executeActionCommand(nav, cmd, args); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
			continue
		}

		// Execute command
		if err := executeCommand(nav, cmd, args); err != nil {
			fmt.Printf("Error: %v\n", err)
		}

		if cmd == "exit" || cmd == "quit" || cmd == "q" {
			break
		}
	}
}

func getPrompt(nav *Navigator) string {
	if nav.actionMode {
		return colorBoldRed.Sprint("action> ")
	}
	return fmt.Sprintf("%s> ", colorBoldBlue.Sprint(nav.cwd))
}

func executeCommand(nav *Navigator, cmd string, args []string) error {
	switch cmd {
	case "cd":
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return nav.cd(target)

	case "open":
		if len(args) == 0 {
			return fmt.Errorf("usage: open <path>")
		}
		return nav.open(args[0])

	case "ls":
		target := ""
		if len(args) > 0 {
			target = strings.Join(args, " ")
		}
		return nav.ls(target)

	case "ll":
		target := ""
		if len(args) > 0 {
			target = strings.Join(args, " ")
		}
		return nav.ll(target)

	case "pwd":
		fmt.Println(nav.cwd)

	case "dump":
		target := ""
		if len(args) > 0 {
			target = strings.Join(args, " ")
		}
		return nav.dump(target)

	case "tree":
		depth := 2
		if len(args) > 0 {
			if d, err := strconv.Atoi(args[0]); err == nil {
				depth = d
			}
		}
		return nav.tree(depth)

	case "find":
		if len(args) == 0 {
			return fmt.Errorf("usage: find <pattern>")
		}
		return nav.find(args[0])

	case "cache":
		if len(args) == 0 {
			paths := nav.vfs.GetKnownPaths()
			fmt.Printf("Cache: %d resources\n", len(paths))
		} else if args[0] == "clear" {
			nav.vfs.Clear()
			fmt.Println("Cache cleared")
		} else if args[0] == "list" {
			paths := nav.vfs.GetKnownPaths()
			sort.Strings(paths)
			for _, path := range paths {
				fmt.Println(path)
			}
		}

	case "clear":
		fmt.Print("\033[H\033[2J")

	case "help", "?":
		printHelp()

	case "exit", "quit", "q":
		// Handled in main loop
		return nil

	default:
		return fmt.Errorf("unknown command: %s (type 'help' for commands)", cmd)
	}

	return nil
}

// printActionList displays available actions
func printActionList(actions []ActionInfo) {
	fmt.Println()
	colorBoldRed.Println("Actions")
	for _, a := range actions {
		line := fmt.Sprintf("  %s", colorYellow.Sprint(a.ShortName))
		if len(a.Allowable) > 0 {
			var params []string
			for param, vals := range a.Allowable {
				params = append(params, fmt.Sprintf("%s=[%s]", param, strings.Join(vals, "|")))
			}
			sort.Strings(params)
			line += fmt.Sprintf("  %s", strings.Join(params, " "))
		}
		fmt.Println(line)
	}
	fmt.Println()
}

// executeActionCommand handles commands in action mode
func executeActionCommand(nav *Navigator, cmd string, args []string) error {
	switch cmd {
	case "ls":
		actions, err := discoverActions(nav)
		if err != nil {
			return err
		}
		if len(args) > 0 {
			action := matchAction(actions, args[0])
			if action == nil {
				return fmt.Errorf("unknown action: %s", args[0])
			}
			printActionList([]ActionInfo{*action})
		} else {
			printActionList(actions)
		}
		return nil

	case "ll":
		actions, err := discoverActions(nav)
		if err != nil {
			return err
		}
		if len(args) > 0 {
			action := matchAction(actions, args[0])
			if action == nil {
				return fmt.Errorf("unknown action: %s", args[0])
			}
			showActionDetail(nav, action)
		} else {
			for i := range actions {
				showActionDetail(nav, &actions[i])
			}
		}
		return nil

	case "help", "?":
		printActionHelp()
		return nil

	default:
		// Try to match as action invocation
		actions, err := discoverActions(nav)
		if err != nil {
			return err
		}
		action := matchAction(actions, cmd)
		if action == nil {
			return fmt.Errorf("unknown action: %s (type 'help' for commands)", cmd)
		}
		return invokeAction(nav, action, args)
	}
}

// showActionDetail shows detailed info for one action
func showActionDetail(nav *Navigator, action *ActionInfo) {
	fmt.Println()
	colorBoldRed.Println(action.Name)
	fmt.Printf("  Target: %s\n", action.Target)

	if action.InfoURI != "" {
		fmt.Printf("  ActionInfo: %s\n", action.InfoURI)

		// Try to fetch and display the ActionInfo resource
		resource, err := nav.vfs.Get(action.InfoURI)
		if err == nil {
			paramsProp, ok := resource.Properties["Parameters"]
			if ok && paramsProp.Type == rvfs.PropertyArray {
				fmt.Println("\n  Parameters:")
				for _, elem := range paramsProp.Elements {
					if elem.Type != rvfs.PropertyObject {
						continue
					}
					name := ""
					dataType := ""
					required := false
					var allowable []string

					if n, ok := elem.Children["Name"]; ok && n.Type == rvfs.PropertySimple {
						name = fmt.Sprintf("%v", n.Value)
					}
					if dt, ok := elem.Children["DataType"]; ok && dt.Type == rvfs.PropertySimple {
						dataType = fmt.Sprintf("%v", dt.Value)
					}
					if r, ok := elem.Children["Required"]; ok && r.Type == rvfs.PropertySimple {
						if b, ok := r.Value.(bool); ok {
							required = b
						}
					}
					if av, ok := elem.Children["AllowableValues"]; ok && av.Type == rvfs.PropertyArray {
						for _, v := range av.Elements {
							if v.Type == rvfs.PropertySimple {
								allowable = append(allowable, fmt.Sprintf("%v", v.Value))
							}
						}
					}

					reqStr := ""
					if required {
						reqStr = colorRed.Sprint(" (required)")
					}
					fmt.Printf("    %s%s  %s", colorYellow.Sprint(name), reqStr, dataType)
					if len(allowable) > 0 {
						fmt.Printf("  [%s]", strings.Join(allowable, "|"))
					}
					fmt.Println()
				}
			}
		}
	}

	if len(action.Allowable) > 0 {
		if action.InfoURI == "" {
			fmt.Println("\n  Parameters:")
		} else {
			fmt.Println("\n  Allowable values (from annotations):")
		}
		for param, vals := range action.Allowable {
			fmt.Printf("    %s: [%s]\n", colorYellow.Sprint(param), strings.Join(vals, "|"))
		}
	}

	fmt.Println()
}

// invokeAction executes a Redfish action with confirmation
func invokeAction(nav *Navigator, action *ActionInfo, args []string) error {
	// Parse key=value arguments
	body := make(map[string]any)
	for _, arg := range args {
		idx := strings.Index(arg, "=")
		if idx == -1 {
			return fmt.Errorf("invalid argument %q (expected key=value)", arg)
		}
		key := arg[:idx]
		val := arg[idx+1:]

		// Validate against AllowableValues if present
		if allowed, ok := action.Allowable[key]; ok {
			found := false
			for _, a := range allowed {
				if a == val {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("invalid value %q for %s (allowed: %s)", val, key, strings.Join(allowed, ", "))
			}
		}

		// Attempt numeric conversion
		if n, err := strconv.ParseFloat(val, 64); err == nil {
			if n == float64(int64(n)) {
				body[key] = int64(n)
			} else {
				body[key] = n
			}
		} else if val == "true" {
			body[key] = true
		} else if val == "false" {
			body[key] = false
		} else {
			body[key] = val
		}
	}

	jsonBody, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}

	// Show confirmation
	fmt.Printf("\n%s %s\n", colorBoldRed.Sprint("POST"), action.Target)
	if len(body) > 0 {
		fmt.Println(string(jsonBody))
	}
	fmt.Print("\nConfirm? [y/N] ")

	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "y" && confirm != "Y" {
		fmt.Println("Cancelled")
		return nil
	}

	// Execute
	data, status, err := nav.vfs.Post(action.Target, jsonBody)
	if err != nil {
		return err
	}

	fmt.Printf("\nHTTP %d\n", status)
	if len(data) > 0 {
		var buf bytes.Buffer
		if json.Indent(&buf, data, "", "  ") == nil {
			fmt.Println(buf.String())
		} else {
			fmt.Println(string(data))
		}
	}
	return nil
}

// printActionHelp shows action mode help
func printActionHelp() {
	bold := color.New(color.Bold).SprintFunc()
	cmd := colorCyan.SprintFunc()
	arg := colorYellow.SprintFunc()

	fmt.Println()
	fmt.Println(bold("Action Mode"))
	fmt.Printf("  %s %-16s %s\n", cmd("ls"), "", "List available actions")
	fmt.Printf("  %s %-16s %s\n", cmd("ll"), arg("<action>"), "Show action details and parameters")
	fmt.Printf("  %s %-16s %s\n", cmd("<action>"), arg("[k=v ...]"), "Invoke action (with confirmation)")
	fmt.Printf("  %s %-16s %s\n", cmd("!"), "", "Exit action mode")
	fmt.Printf("  %s %-16s %s\n", cmd("help"), "", "Show this help")
	fmt.Println()
	fmt.Println(bold("Example"))
	fmt.Printf("  %s\n", colorYellow.Sprint("Reset ResetType=GracefulShutdown"))
	fmt.Println()
}

func printHelp() {
	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()
	cmd := colorCyan.SprintFunc()
	arg := colorYellow.SprintFunc()

	fmt.Println()
	fmt.Println(bold("Navigation"))
	fmt.Printf("  %s %-12s %s    %s %-12s %s\n", cmd("cd"), arg("<path>"), "Navigate to resource/property", cmd("open"), arg("<path>"), "Follow link to target resource")
	fmt.Printf("  %s %-12s %s    %s %-12s %s\n", cmd("pwd"), "", "Print working directory", cmd("ls"), arg("[path]"), "List entries")
	fmt.Printf("  %s %-12s %s\n", cmd("ll"), arg("[path]"), "Show formatted content (YAML-style)")

	fmt.Println()
	fmt.Println(bold("Viewing & Search"))
	fmt.Printf("  %s %-12s %s    %s %-12s %s\n", cmd("dump"), arg("[path]"), "Show raw JSON", cmd("tree"), arg("[depth]"), "Tree view (default: 2)")
	fmt.Printf("  %s %-12s %s\n", cmd("find"), arg("<pattern>"), "Search properties recursively")

	fmt.Println()
	fmt.Println(bold("Other"))
	fmt.Printf("  %s %-12s %s    %s %-12s %s\n", cmd("!"), "", "Enter action mode (POST)", cmd("cache"), arg("[cmd]"), "Cache ops (clear, list)")
	fmt.Printf("  %s %-12s %s    %s %s\n", cmd("clear"), "", "Clear screen", cmd("help"), dim("exit/quit"))

	fmt.Println()
	fmt.Println(bold("Paths"))
	fmt.Printf("  %s  %s  %s  %s             %s\n",
		arg("/"), dim("separator"),
		arg("[n]"), dim("array index"),
		dim("Systems/1/Status/Health  BootOrder[0]"))
	fmt.Printf("  %s %s  %s  %s             %s\n",
		arg(".."), dim("parent"),
		arg("~"), dim("root (/redfish/v1)"),
		dim("open .  returns to containing resource"))

	fmt.Println()
	fmt.Println(bold("Keys"))
	fmt.Printf("  %s  %s    %s  %s\n",
		dim("Tab"), "complete",
		dim("Ctrl+R"), "history search")
	fmt.Printf("  %s  %s    %s  %s\n",
		dim("↑/↓"), "history",
		dim("Ctrl+L"), "clear screen")

	fmt.Println()
	fmt.Println(bold("Display"))
	fmt.Printf("  %s  %s  %s  %s  %s\n",
		colorBoldBlue.Sprint("dir/"), dim("child"),
		colorCyan.Sprint("link@"), dim("symlink"),
		colorGreen.Sprint("prop"))
	fmt.Printf("  %s  %s  %s  %s\n",
		colorPurple.Sprint("obj/"), dim("object"),
		colorPurple.Sprint("arr[]"), dim("array"))
	fmt.Println()
}

// formatColumns formats items in columns like ls
func formatColumns(items []string) string {
	if len(items) == 0 {
		return ""
	}

	// Get terminal width
	width := 100 // default
	if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil {
			width = w
		}
	}

	// Calculate column width (accounting for ANSI codes)
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

	// Format in columns
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

// stripAnsi removes ANSI escape codes from text
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
