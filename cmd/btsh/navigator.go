package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/bluefish-project/bluefish/rvfs"
)

// Navigator manages shell state
type Navigator struct {
	vfs rvfs.VFS
	cwd string
}

// NewNavigator creates a navigator
func NewNavigator(vfs rvfs.VFS) *Navigator {
	return &Navigator{
		vfs: vfs,
		cwd: "/redfish/v1",
	}
}

// normalizePath ensures path has no trailing slash
func normalizePath(p string) string {
	return strings.TrimRight(p, "/")
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
func listResolved(vfs rvfs.VFS, target *rvfs.Target) []*rvfs.Entry {
	switch target.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		entries, _ := vfs.ListAll(target.ResourcePath)
		return entries
	case rvfs.TargetProperty:
		return entriesFromProperty(target.Property)
	}
	return nil
}

// cd changes directory and returns a status message
func (n *Navigator) cd(target string) (string, error) {
	if target == "" {
		target = "~"
	}

	if target == "~" {
		target = rvfs.RedfishRoot
	} else if strings.HasPrefix(target, "~/") {
		target = rvfs.RedfishRoot + "/" + target[2:]
	}

	resolvedTarget, err := n.vfs.ResolveTarget(n.cwd, target)
	if err != nil {
		return "", err
	}

	switch resolvedTarget.Type {
	case rvfs.TargetResource:
		n.cwd = resolvedTarget.ResourcePath
	case rvfs.TargetLink:
		n.cwd = resolvedTarget.ResourcePath
	case rvfs.TargetProperty:
		switch resolvedTarget.Property.Type {
		case rvfs.PropertyObject, rvfs.PropertyArray:
			if strings.HasPrefix(target, "/") {
				n.cwd = normalizePath(target)
			} else {
				n.cwd = normalizePath(path.Join(n.cwd, target))
			}
		default:
			return "", fmt.Errorf("cannot cd to value: %s", target)
		}
	}

	entries := listResolved(n.vfs, resolvedTarget)
	return fmt.Sprintf("%s  (%s)", n.cwd, getEntriesSummary(entries)), nil
}

// open follows links to their canonical destinations
func (n *Navigator) open(target string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("open requires a target path")
	}

	resolvedTarget, err := n.vfs.ResolveTarget(n.cwd, target)
	if err != nil {
		if target == "." {
			resolvedTarget, err = n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	switch resolvedTarget.Type {
	case rvfs.TargetResource:
		n.cwd = resolvedTarget.ResourcePath
		entries, _ := n.vfs.ListAll(n.cwd)
		return fmt.Sprintf("%s  (%s)", n.cwd, getEntriesSummary(entries)), nil

	case rvfs.TargetLink:
		n.cwd = resolvedTarget.ResourcePath
		entries, _ := n.vfs.ListAll(n.cwd)
		return fmt.Sprintf("%s  (%s)", n.cwd, getEntriesSummary(entries)), nil

	case rvfs.TargetProperty:
		prop := resolvedTarget.Property
		if prop.Type == rvfs.PropertyLink {
			n.cwd = prop.LinkTarget
			entries, _ := n.vfs.ListAll(n.cwd)
			return fmt.Sprintf("%s  (%s)", n.cwd, getEntriesSummary(entries)), nil
		} else if target == "." {
			n.cwd = resolvedTarget.Resource.Path
			entries, _ := n.vfs.ListAll(n.cwd)
			return fmt.Sprintf("%s  (%s)", n.cwd, getEntriesSummary(entries)), nil
		}
		return "", fmt.Errorf("cannot open property %s (not a link; use 'cd' to navigate into objects)", target)
	}

	return "", nil
}

// ls lists all entries
func (n *Navigator) ls(target string) (string, error) {
	if target == "." {
		target = ""
	}

	var resolved *rvfs.Target
	var err error
	if target == "" {
		resolved, err = n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
	} else {
		resolved, err = n.vfs.ResolveTarget(n.cwd, target)
	}
	if err != nil {
		return "", err
	}

	entries := listResolved(n.vfs, resolved)
	var b strings.Builder
	if len(entries) == 0 {
		b.WriteString("(empty)")
	} else {
		items := make([]string, len(entries))
		for i, entry := range entries {
			items[i] = formatEntry(entry)
		}
		b.WriteString(formatColumns(items))
	}

	age := formatResourceAge(resolved)
	if age != "" {
		b.WriteString("\n")
		b.WriteString(age)
	}
	return b.String(), nil
}

// ll displays formatted content
func (n *Navigator) ll(target string) (string, error) {
	if target == "." {
		target = ""
	}

	var resolved *rvfs.Target
	var err error
	if target == "" {
		resolved, err = n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
	} else {
		resolved, err = n.vfs.ResolveTarget(n.cwd, target)
	}
	if err != nil {
		return "", err
	}

	var b strings.Builder
	switch resolved.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		if err := showResource(&b, n.vfs, resolved.ResourcePath); err != nil {
			return "", err
		}
		age := formatResourceAge(resolved)
		if age != "" {
			b.WriteString(age)
		}
	case rvfs.TargetProperty:
		showProperty(&b, resolved.Property, 0, false)
	}
	return b.String(), nil
}

// dump displays raw JSON
func (n *Navigator) dump(target string) (string, error) {
	var resolved *rvfs.Target
	var err error
	if target == "" {
		resolved, err = n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
	} else {
		resolved, err = n.vfs.ResolveTarget(n.cwd, target)
	}
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	switch resolved.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		json.Indent(&buf, resolved.Resource.RawJSON, "", "  ")
	case rvfs.TargetProperty:
		json.Indent(&buf, resolved.Property.RawJSON, "", "  ")
	}
	return buf.String(), nil
}

// tree displays tree view
func (n *Navigator) tree(depth int) (string, error) {
	resolved, err := n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
	if err != nil {
		return "", err
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
		return "(empty)", nil
	}
	return output, nil
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

		if entry.IsDir() && currentDepth+1 < maxDepth {
			extension := "│   "
			if isLast {
				extension = "    "
			}

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
func (n *Navigator) find(pattern string) (string, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %v", err)
	}

	resolved, err := n.vfs.ResolveTarget(rvfs.RedfishRoot, n.cwd)
	if err != nil {
		return "", err
	}

	var results []string

	switch resolved.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		n.findInResource(resolved.ResourcePath, "", re, &results, 0)
	case rvfs.TargetProperty:
		findInProperty(resolved.Property, "", re, &results)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No matches found for '%s'", pattern), nil
	}
	return strings.Join(results, "\n"), nil
}

func (n *Navigator) findInResource(resourcePath, prefix string, re *regexp.Regexp, results *[]string, depth int) {
	if depth > 5 {
		return
	}

	resource, err := n.vfs.Get(resourcePath)
	if err != nil {
		return
	}

	for _, prop := range resource.Properties {
		findInProperty(prop, prefix, re, results)
	}

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
				warnStyle.Render(fullPath),
				formatPropertyValue(prop)))
	}

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

// refresh invalidates and re-fetches a resource
func (n *Navigator) refresh(target string) (string, error) {
	var p string
	if target == "" {
		p = n.cwd
	} else {
		resolved, err := n.vfs.ResolveTarget(n.cwd, target)
		if err != nil {
			return "", err
		}
		switch resolved.Type {
		case rvfs.TargetResource, rvfs.TargetLink:
			p = resolved.ResourcePath
		default:
			return "", fmt.Errorf("can only refresh resources, not properties")
		}
	}

	n.vfs.Invalidate(p)

	res, err := n.vfs.Get(p)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	if err := showResource(&b, n.vfs, p); err != nil {
		return "", err
	}
	b.WriteString(dimStyle.Render(formatAge(res.FetchedAt)))
	return b.String(), nil
}

// cache handles cache commands
func (n *Navigator) cache(args []string) (string, error) {
	if len(args) == 0 {
		paths := n.vfs.GetKnownPaths()
		return fmt.Sprintf("Cache: %d resources", len(paths)), nil
	}

	switch args[0] {
	case "clear":
		n.vfs.Clear()
		return "Cache cleared", nil
	case "list":
		paths := n.vfs.GetKnownPaths()
		sort.Strings(paths)
		return strings.Join(paths, "\n"), nil
	default:
		return "", fmt.Errorf("unknown cache command: %s (try: clear, list)", args[0])
	}
}
