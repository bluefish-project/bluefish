package main

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bluefish-project/bluefish/rvfs"
)

// TreeItemKind classifies what a tree item represents
type TreeItemKind int

const (
	KindResource TreeItemKind = iota
	KindChild
	KindSimple
	KindObject
	KindArray
	KindLink
)

// TreeItem is one row in the flat visible list
type TreeItem struct {
	Path        string
	Name        string
	Depth       int
	Kind        TreeItemKind
	Property    *rvfs.Property
	Child       *rvfs.Child
	Resource    *rvfs.Resource
	Value       string // Formatted plain value for simple props
	LinkTarget  string
	ChildCount  int
	HasChildren bool
	IsExpanded  bool
}

// treeNode is the backing data for the full tree (not just visible items)
type treeNode struct {
	Item     TreeItem
	Children []*treeNode
	Loaded   bool // Whether children have been fetched
}

// TreeModel manages the tree panel
type TreeModel struct {
	root    *treeNode
	visible []TreeItem
	cursor  int
	offset  int // Scroll offset
	height  int // Visible rows
	width   int

	// Node lookup for async load results
	nodeMap map[string]*treeNode
}

func NewTreeModel() TreeModel {
	return TreeModel{
		nodeMap: make(map[string]*treeNode),
	}
}

// Init builds the tree from a resource
func (t *TreeModel) Init(resource *rvfs.Resource, basePath string) {
	t.nodeMap = make(map[string]*treeNode)
	t.root = t.buildResourceNode(resource, basePath, 0)
	t.root.Item.IsExpanded = true
	t.root.Loaded = true
	t.rebuildVisible()
	t.cursor = 0
	t.offset = 0
}

func (t *TreeModel) buildResourceNode(resource *rvfs.Resource, path string, depth int) *treeNode {
	name := path
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) > 0 {
		name = parts[len(parts)-1]
	}
	if path == rvfs.RedfishRoot {
		name = "Root"
	}

	node := &treeNode{
		Item: TreeItem{
			Path:     path,
			Name:     name,
			Depth:    depth,
			Kind:     KindResource,
			Resource: resource,
		},
		Loaded: true,
	}
	t.nodeMap[path] = node

	// Add children (sorted)
	childNames := make([]string, 0, len(resource.Children))
	for n := range resource.Children {
		childNames = append(childNames, n)
	}
	sort.Strings(childNames)

	for _, cn := range childNames {
		child := resource.Children[cn]
		childNode := &treeNode{
			Item: TreeItem{
				Path:        child.Target,
				Name:        cn,
				Depth:       depth + 1,
				Kind:        KindChild,
				Child:       child,
				HasChildren: true, // Assume children have sub-items
			},
		}
		t.nodeMap[child.Target] = childNode
		node.Children = append(node.Children, childNode)
	}

	// Add properties (sorted, skip @odata)
	propNames := make([]string, 0, len(resource.Properties))
	for n := range resource.Properties {
		if !strings.HasPrefix(n, "@odata") {
			propNames = append(propNames, n)
		}
	}
	sort.Strings(propNames)

	for _, pn := range propNames {
		prop := resource.Properties[pn]
		propPath := path + "/" + pn
		propNode := t.buildPropertyNode(prop, propPath, depth+1)
		node.Children = append(node.Children, propNode)
	}

	node.Item.HasChildren = len(node.Children) > 0
	return node
}

func (t *TreeModel) buildPropertyNode(prop *rvfs.Property, path string, depth int) *treeNode {
	item := TreeItem{
		Path:     path,
		Name:     prop.Name,
		Depth:    depth,
		Property: prop,
	}

	var children []*treeNode

	switch prop.Type {
	case rvfs.PropertySimple:
		item.Kind = KindSimple
		item.Value = formatPlainValue(prop.Value)

	case rvfs.PropertyObject:
		item.Kind = KindObject
		item.HasChildren = len(prop.Children) > 0
		item.ChildCount = len(prop.Children)

		objNames := make([]string, 0, len(prop.Children))
		for n := range prop.Children {
			objNames = append(objNames, n)
		}
		sort.Strings(objNames)

		for _, cn := range objNames {
			childProp := prop.Children[cn]
			childPath := path + "/" + cn
			children = append(children, t.buildPropertyNode(childProp, childPath, depth+1))
		}

	case rvfs.PropertyArray:
		item.Kind = KindArray
		item.HasChildren = len(prop.Elements) > 0
		item.ChildCount = len(prop.Elements)

		for i, elem := range prop.Elements {
			elemPath := fmt.Sprintf("%s[%d]", path, i)
			elemNode := t.buildPropertyNode(elem, elemPath, depth+1)
			elemNode.Item.Name = fmt.Sprintf("[%d]", i)
			children = append(children, elemNode)
		}

	case rvfs.PropertyLink:
		item.Kind = KindLink
		item.LinkTarget = prop.LinkTarget
	}

	node := &treeNode{
		Item:     item,
		Children: children,
		Loaded:   true,
	}
	t.nodeMap[path] = node
	return node
}

// rebuildVisible walks the tree and builds the flat visible slice
func (t *TreeModel) rebuildVisible() {
	t.visible = nil
	if t.root == nil {
		return
	}
	t.walkNode(t.root)
}

func (t *TreeModel) walkNode(node *treeNode) {
	t.visible = append(t.visible, node.Item)
	if node.Item.IsExpanded {
		for _, child := range node.Children {
			t.walkNode(child)
		}
	}
}

// Current returns the currently selected item, or nil
func (t *TreeModel) Current() *TreeItem {
	if t.cursor >= 0 && t.cursor < len(t.visible) {
		return &t.visible[t.cursor]
	}
	return nil
}

// findNode finds a treeNode by path
func (t *TreeModel) findNode(path string) *treeNode {
	return t.nodeMap[path]
}

// HandleResourceLoaded integrates an async-fetched resource into the tree
func (t *TreeModel) HandleResourceLoaded(path string, resource *rvfs.Resource) {
	node := t.findNode(path)
	if node == nil {
		return
	}

	node.Loaded = true
	node.Item.Resource = resource
	node.Item.Kind = KindResource
	node.Children = nil

	// Build child nodes
	childNames := make([]string, 0, len(resource.Children))
	for n := range resource.Children {
		childNames = append(childNames, n)
	}
	sort.Strings(childNames)

	for _, cn := range childNames {
		child := resource.Children[cn]
		childNode := &treeNode{
			Item: TreeItem{
				Path:        child.Target,
				Name:        cn,
				Depth:       node.Item.Depth + 1,
				Kind:        KindChild,
				Child:       child,
				HasChildren: true,
			},
		}
		t.nodeMap[child.Target] = childNode
		node.Children = append(node.Children, childNode)
	}

	propNames := make([]string, 0, len(resource.Properties))
	for n := range resource.Properties {
		if !strings.HasPrefix(n, "@odata") {
			propNames = append(propNames, n)
		}
	}
	sort.Strings(propNames)

	for _, pn := range propNames {
		prop := resource.Properties[pn]
		propPath := path + "/" + pn
		propNode := t.buildPropertyNode(prop, propPath, node.Item.Depth+1)
		node.Children = append(node.Children, propNode)
	}

	node.Item.HasChildren = len(node.Children) > 0
	t.rebuildVisible()
}

// MoveUp moves cursor up
func (t *TreeModel) MoveUp() *TreeItem {
	if t.cursor > 0 {
		t.cursor--
		t.ensureVisible()
	}
	return t.Current()
}

// MoveDown moves cursor down
func (t *TreeModel) MoveDown() *TreeItem {
	if t.cursor < len(t.visible)-1 {
		t.cursor++
		t.ensureVisible()
	}
	return t.Current()
}

// Expand expands the current item
func (t *TreeModel) Expand() tea.Cmd {
	item := t.Current()
	if item == nil || !item.HasChildren {
		return nil
	}

	node := t.findNode(item.Path)
	if node == nil {
		return nil
	}

	if !node.Loaded {
		// Need to fetch this resource
		node.Item.IsExpanded = true
		item.IsExpanded = true
		t.rebuildVisible()
		path := item.Path
		return func() tea.Msg {
			// Will be handled by the root model
			return fetchResourceMsg{Path: path}
		}
	}

	node.Item.IsExpanded = true
	t.rebuildVisible()
	return nil
}

// Collapse collapses the current item, or moves to parent
func (t *TreeModel) Collapse() {
	item := t.Current()
	if item == nil {
		return
	}

	node := t.findNode(item.Path)
	if node == nil {
		return
	}

	if node.Item.IsExpanded && node.Item.HasChildren {
		node.Item.IsExpanded = false
		t.rebuildVisible()
		return
	}

	// Go to parent: find the item at a lesser depth above cursor
	for i := t.cursor - 1; i >= 0; i-- {
		if t.visible[i].Depth < item.Depth {
			t.cursor = i
			t.ensureVisible()
			return
		}
	}
}

// Toggle toggles expand/collapse
func (t *TreeModel) Toggle() tea.Cmd {
	item := t.Current()
	if item == nil || !item.HasChildren {
		return nil
	}

	node := t.findNode(item.Path)
	if node == nil {
		return nil
	}

	if node.Item.IsExpanded {
		t.Collapse()
		return nil
	}
	return t.Expand()
}

// ensureVisible adjusts scroll offset to keep cursor in view
func (t *TreeModel) ensureVisible() {
	if t.height <= 0 {
		return
	}
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+t.height {
		t.offset = t.cursor - t.height + 1
	}
}

// fetchResourceMsg is an internal message to trigger a VFS fetch
type fetchResourceMsg struct {
	Path string
}

// View renders the tree panel
func (t *TreeModel) View() string {
	if len(t.visible) == 0 {
		return loadingStyle.Render("  Loading...")
	}

	var b strings.Builder

	end := t.offset + t.height
	if end > len(t.visible) {
		end = len(t.visible)
	}

	for i := t.offset; i < end; i++ {
		item := t.visible[i]

		var line string
		if i == t.cursor {
			// Render plain text with reverse for clean highlight bar
			plain := t.renderItemPlain(item)
			padding := t.width - len(plain)
			if padding < 0 {
				padding = 0
			}
			line = cursorStyle.Render(plain + strings.Repeat(" ", padding))
		} else {
			line = t.renderItem(item)
		}

		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (t *TreeModel) renderItem(item TreeItem) string {
	indent := strings.Repeat("  ", item.Depth)

	var indicator string
	if item.HasChildren {
		if item.IsExpanded {
			indicator = indicatorStyle.Render("▾ ")
		} else {
			indicator = indicatorStyle.Render("▸ ")
		}
	} else {
		indicator = "  "
	}

	var text string
	switch item.Kind {
	case KindResource:
		text = childStyle.Render(item.Name)
	case KindChild:
		node := t.findNode(item.Path)
		if node != nil && !node.Loaded && item.IsExpanded {
			text = childStyle.Render(item.Name) + " " + loadingStyle.Render("loading...")
		} else {
			text = childStyle.Render(item.Name)
		}
	case KindSimple:
		text = propNameStyle.Render(item.Name) + ": " + formatHealthValue(item.Name, item.Property.Value)
	case KindObject:
		text = objectStyle.Render(item.Name) + " " + objectStyle.Render(fmt.Sprintf("{%d}", item.ChildCount))
	case KindArray:
		text = arrayStyle.Render(item.Name) + " " + arrayStyle.Render(fmt.Sprintf("[%d]", item.ChildCount))
	case KindLink:
		text = linkStyle.Render(item.Name) + " " + linkStyle.Render("→") + " " + linkStyle.Render(item.LinkTarget)
	}

	return indent + indicator + text
}

// renderItemPlain returns the item text without ANSI codes (for width measurement)
func (t *TreeModel) renderItemPlain(item TreeItem) string {
	indent := strings.Repeat("  ", item.Depth)

	var indicator string
	if item.HasChildren {
		indicator = "▾ "
		if !item.IsExpanded {
			indicator = "▸ "
		}
	} else {
		indicator = "  "
	}

	var text string
	switch item.Kind {
	case KindResource:
		text = item.Name
	case KindChild:
		text = item.Name
	case KindSimple:
		text = item.Name + ": " + item.Value
	case KindObject:
		text = item.Name + fmt.Sprintf(" {%d}", item.ChildCount)
	case KindArray:
		text = item.Name + fmt.Sprintf(" [%d]", item.ChildCount)
	case KindLink:
		text = item.Name + " → " + item.LinkTarget
	}

	return indent + indicator + text
}
