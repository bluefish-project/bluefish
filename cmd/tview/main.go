package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gopkg.in/yaml.v3"

	"bluefish/rvfs"
)

// NodeRef holds a reference to what this tree node represents
type NodeRef struct {
	Path       string // Full composite path
	Resource   *rvfs.Resource
	Property   *rvfs.Property
	IsLink     bool
	LinkTarget string
	Parent     *tview.TreeNode // Direct pointer to parent node
}

type App struct {
	app      *tview.Application
	vfs      rvfs.VFS

	tree     *tview.TreeView
	details  *tview.TextView
	status   *tview.TextView

	fullRoot *tview.TreeNode   // The original full tree root (never changes)
	rootStack []*tview.TreeNode // Stack of previous roots for back navigation

	nodeMap  map[string]*tview.TreeNode // Path -> TreeNode for jumping

	// Current state
	basePath string   // Current tree root path (for display)
}

func NewApp(vfs rvfs.VFS) *App {
	a := &App{
		app:       tview.NewApplication(),
		vfs:       vfs,
		nodeMap:   make(map[string]*tview.TreeNode),
		basePath:  "/redfish/v1",
		rootStack: []*tview.TreeNode{},
	}

	a.buildUI()
	return a
}

func (a *App) buildUI() {
	// Status bar at top
	a.status = tview.NewTextView().
		SetDynamicColors(true).
		SetText("[yellow::b]RFUI[-:-:-] | ↑↓:navigate | Enter:expand/jump | Space:toggle | /:filter | q:quit")

	// Tree view on left
	a.tree = tview.NewTreeView()
	a.tree.SetBorder(true).
		SetTitle("Resources & Properties").
		SetTitleAlign(tview.AlignLeft)

	// Details panel on right - CREATE BEFORE buildTree
	a.details = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetScrollable(true)
	a.details.SetBorder(true).
		SetTitle("Details").
		SetTitleAlign(tview.AlignLeft)

	// Build initial tree
	a.buildTree()

	// Handle node selection
	a.tree.SetChangedFunc(func(node *tview.TreeNode) {
		a.updateDetails(node)
	})

	// Handle node activation (Enter key) - rebase on links
	a.tree.SetSelectedFunc(func(node *tview.TreeNode) {
		ref := node.GetReference()
		if ref == nil {
			return
		}

		nodeRef, ok := ref.(*NodeRef)
		if !ok {
			return
		}

		// If it's a link, rebase tree on its target
		if nodeRef.IsLink {
			a.rebaseTree(node)
		}
	})

	// Custom key bindings for h/j/k/l navigation
	a.tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		current := a.tree.GetCurrentNode()
		if current == nil {
			return event
		}

		switch event.Rune() {
		case 'h':
			// Collapse if expanded and has children, otherwise go to parent and collapse
			if current.IsExpanded() && len(current.GetChildren()) > 0 {
				current.SetExpanded(false)
			} else {
				// Use parent pointer from NodeRef
				if ref := current.GetReference(); ref != nil {
					if nodeRef, ok := ref.(*NodeRef); ok && nodeRef.Parent != nil {
						nodeRef.Parent.SetExpanded(false)
						a.tree.SetCurrentNode(nodeRef.Parent)
					}
				}
			}
			return nil
		case 'l':
			// Expand if has children
			if len(current.GetChildren()) > 0 {
				current.SetExpanded(true)
			}
			return nil
		case 's':
			// Rebase tree on current node (subtree)
			a.rebaseTree(current)
			return nil
		case 'b':
			// Go back to previous tree base
			a.popTree()
			return nil
		case 'u':
			// Go up to parent
			a.goUp()
			return nil
		case 'J':
			// Scroll details panel down
			row, col := a.details.GetScrollOffset()
			a.details.ScrollTo(row+1, col)
			return nil
		case 'K':
			// Scroll details panel up
			row, col := a.details.GetScrollOffset()
			if row > 0 {
				a.details.ScrollTo(row-1, col)
			}
			return nil
		}

		// Handle backspace as alternative to 'b'
		if event.Key() == tcell.KeyBackspace || event.Key() == tcell.KeyBackspace2 {
			a.popTree()
			return nil
		}

		// Handle Home key or ~ to go to root
		if event.Key() == tcell.KeyHome || event.Rune() == '~' {
			a.goHome()
			return nil
		}

		return event
	})

	// Layout: tree on left (40%), details on right (60%)
	grid := tview.NewGrid().
		SetRows(1, 0, 1).
		SetColumns(0, 0, 0). // 3 columns for 40/60 split
		AddItem(a.status, 0, 0, 1, 3, 0, 0, false).
		AddItem(a.tree, 1, 0, 1, 1, 0, 0, true).
		AddItem(a.details, 1, 1, 1, 2, 0, 0, false).
		AddItem(a.makeHelpBar(), 2, 0, 1, 3, 0, 0, false)

	// Global key bindings
	grid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' {
			a.app.Stop()
			return nil
		}
		return event
	})

	a.app.SetRoot(grid, true).SetFocus(a.tree)
}

func (a *App) makeHelpBar() *tview.TextView {
	return tview.NewTextView().
		SetDynamicColors(true).
		SetText("[gray]h:collapse | j/k:nav | l:expand | s:subtree | u:up | b/⌫:back | ~:home | Enter:follow | J/K:scroll | q:quit[-]")
}

func (a *App) buildTree() {
	// Clear nodeMap for fresh build
	a.nodeMap = make(map[string]*tview.TreeNode)

	// Get basename for root node
	baseName := a.basePath
	if baseName == "/redfish/v1" {
		baseName = "Root"
	} else {
		// Use last segment of path
		parts := strings.Split(strings.TrimSuffix(a.basePath, "/"), "/")
		baseName = parts[len(parts)-1]
	}

	// Create root node
	root := tview.NewTreeNode(baseName).
		SetColor(tcell.ColorYellow).
		SetSelectable(true)

	// Load root resource - trust RVFS to handle the path
	resource, err := a.vfs.Get(a.basePath)
	if err != nil {
		errMsg := fmt.Sprintf("Error loading %s: %v", a.basePath, err)
		root.SetText(errMsg).SetColor(tcell.ColorRed)
		a.tree.SetRoot(root).SetCurrentNode(root)
		a.status.SetText(fmt.Sprintf("[red]%s[-]", errMsg))
		return
	}

	// Set reference (root has no parent)
	root.SetReference(&NodeRef{
		Path:     a.basePath,
		Resource: resource,
		Parent:   nil, // Root has no parent
	})
	a.nodeMap[a.basePath] = root

	// Build children
	a.addResourceChildren(root, resource, a.basePath)

	// Save the full root on first build
	if a.fullRoot == nil {
		a.fullRoot = root
	}

	a.tree.SetRoot(root).SetCurrentNode(root)
	a.updateDetails(root)

	// Update status to show current subtree
	if a.basePath == "/redfish/v1" {
		a.status.SetText("[yellow::b]RFUI[-:-:-] | Tree: [cyan]Full[-]")
	} else {
		a.status.SetText(fmt.Sprintf("[yellow::b]RFUI[-:-:-] | Subtree: [cyan]%s[-]", a.basePath))
	}
}

func (a *App) addResourceChildren(parent *tview.TreeNode, resource *rvfs.Resource, basePath string) {
	// Add child resources
	childNames := make([]string, 0, len(resource.Children))
	for name := range resource.Children {
		childNames = append(childNames, name)
	}
	sort.Strings(childNames)

	for _, name := range childNames {
		child := resource.Children[name]
		childPath := child.Target // Use @odata.id directly

		node := tview.NewTreeNode(name).
			SetColor(tcell.ColorLightBlue).
			SetSelectable(true).
			SetReference(&NodeRef{
				Path:     childPath,
				Resource: nil, // Lazy load
				Parent:   parent, // Store parent reference
			})

		parent.AddChild(node)
		a.nodeMap[childPath] = node

		// Mark as having children (will load on expand)
		node.SetExpanded(false)
	}

	// Add properties
	propNames := make([]string, 0, len(resource.Properties))
	for name := range resource.Properties {
		// Skip @odata metadata
		if strings.HasPrefix(name, "@odata") {
			continue
		}
		propNames = append(propNames, name)
	}
	sort.Strings(propNames)

	for _, name := range propNames {
		prop := resource.Properties[name]
		propPath := basePath + "/" + name

		node := a.createPropertyNode(name, prop, propPath, parent)
		parent.AddChild(node)
		a.nodeMap[propPath] = node
	}
}

func (a *App) createPropertyNode(name string, prop *rvfs.Property, path string, parent *tview.TreeNode) *tview.TreeNode {
	var nodeText string
	var color tcell.Color
	var expandable bool

	switch prop.Type {
	case rvfs.PropertySimple:
		nodeText = fmt.Sprintf("%s: %v", name, formatValue(prop.Value))
		color = tcell.ColorGreen
		expandable = false

	case rvfs.PropertyObject:
		nodeText = fmt.Sprintf("%s {...}", name)
		color = tcell.ColorPurple
		expandable = true

	case rvfs.PropertyArray:
		nodeText = fmt.Sprintf("%s [%d]", name, len(prop.Elements))
		color = tcell.ColorPurple
		expandable = true

	case rvfs.PropertyLink:
		nodeText = fmt.Sprintf("%s → %s", name, prop.LinkTarget)
		color = tcell.ColorYellow
		expandable = false
	}

	node := tview.NewTreeNode(nodeText).
		SetColor(color).
		SetSelectable(true).
		SetReference(&NodeRef{
			Path:       path,
			Property:   prop,
			IsLink:     prop.Type == rvfs.PropertyLink,
			LinkTarget: prop.LinkTarget,
			Parent:     parent, // Store parent reference
		})

	// Add placeholder children for expandable nodes
	if expandable {
		node.SetExpanded(false)
		// Add children immediately
		switch prop.Type {
		case rvfs.PropertyObject:
			a.addPropertyObjectChildren(node, prop, path)
		case rvfs.PropertyArray:
			a.addPropertyArrayChildren(node, prop, path)
		}
	}

	return node
}

func (a *App) addPropertyObjectChildren(parent *tview.TreeNode, prop *rvfs.Property, basePath string) {
	propNames := make([]string, 0, len(prop.Children))
	for name := range prop.Children {
		propNames = append(propNames, name)
	}
	sort.Strings(propNames)

	for _, name := range propNames {
		child := prop.Children[name]
		childPath := basePath + ":" + name

		node := a.createPropertyNode(name, child, childPath, parent)
		parent.AddChild(node)
		a.nodeMap[childPath] = node
	}
}

func (a *App) addPropertyArrayChildren(parent *tview.TreeNode, prop *rvfs.Property, basePath string) {
	for i, elem := range prop.Elements {
		elemPath := fmt.Sprintf("%s[%d]", basePath, i)
		name := fmt.Sprintf("[%d]", i)

		node := a.createPropertyNode(name, elem, elemPath, parent)
		parent.AddChild(node)
		a.nodeMap[elemPath] = node
	}
}

func (a *App) updateDetails(node *tview.TreeNode) {
	if node == nil {
		a.details.Clear()
		return
	}

	ref := node.GetReference()
	if ref == nil {
		a.details.SetText("[yellow]" + node.GetText() + "[-]\n\nNo details available")
		return
	}

	nodeRef, ok := ref.(*NodeRef)
	if !ok {
		a.details.SetText("[yellow]" + node.GetText() + "[-]\n\nInvalid node reference")
		return
	}

	var text = &strings.Builder{}

	// Title
	fmt.Fprintf(text, "[yellow::b]%s[-:-:-]\n\n", node.GetText())

	// Path
	fmt.Fprintf(text, "[gray]Path:[-] [cyan]%s[-]\n\n", nodeRef.Path)

	// If it's a link, show jump instruction
	if nodeRef.IsLink {
		fmt.Fprintf(text, "[yellow]→ Links to:[-] [cyan]%s[-]\n", nodeRef.LinkTarget)
		text.WriteString("[gray](Press Enter to jump)[-]\n\n")
	}

	// Resource details
	if nodeRef.Resource != nil {
		a.formatResourceDetails(text, nodeRef.Resource)
	} else if nodeRef.Property != nil {
		a.formatPropertyDetails(text, nodeRef.Property)
	} else {
		// Lazy load resource
		target, err := a.vfs.ResolveTarget("/redfish/v1", nodeRef.Path)
		if err != nil {
			fmt.Fprintf(text, "[red]Error: %v[-]\n", err)
		} else if target.Type == rvfs.TargetResource {
			nodeRef.Resource = target.Resource
			a.formatResourceDetails(text, target.Resource)

			// Load children into tree if not already loaded
			if len(node.GetChildren()) == 0 {
				a.addResourceChildren(node, target.Resource, nodeRef.Path)
			}
		}
	}

	a.details.SetText(text.String())
	a.details.ScrollToBeginning()
}

func (a *App) formatResourceDetails(text *strings.Builder, res *rvfs.Resource) {
	text.WriteString("[yellow]Type:[-] Resource\n\n")

	// Show child resources count
	if len(res.Children) > 0 {
		fmt.Fprintf(text, "[yellow]Child Resources:[-] %d\n", len(res.Children))
		for name, child := range res.Children {
			fmt.Fprintf(text, "  [cyan]%s[-] → %s\n", name, child.Target)
		}
		text.WriteString("\n")
	}

	// Show properties with full hierarchy
	if len(res.Properties) > 0 {
		text.WriteString("[yellow]Properties:[-]\n")

		// Sort property names
		propNames := make([]string, 0, len(res.Properties))
		for name := range res.Properties {
			propNames = append(propNames, name)
		}
		sort.Strings(propNames)

		for _, name := range propNames {
			prop := res.Properties[name]
			a.formatPropertyRecursive(text, name, prop, 1)
		}
	}
}

// formatPropertyRecursive recursively formats a property with indentation
func (a *App) formatPropertyRecursive(text *strings.Builder, name string, prop *rvfs.Property, indent int) {
	prefix := strings.Repeat("  ", indent)

	switch prop.Type {
	case rvfs.PropertySimple:
		fmt.Fprintf(text, "%s[gray]%s:[-] %v\n", prefix, name, formatValue(prop.Value))

	case rvfs.PropertyLink:
		fmt.Fprintf(text, "%s[gray]%s:[-] [yellow]→ %s[-]\n", prefix, name, prop.LinkTarget)

	case rvfs.PropertyObject:
		fmt.Fprintf(text, "%s[gray]%s:[-] {...}\n", prefix, name)
		// Show nested properties
		if len(prop.Children) > 0 {
			childNames := make([]string, 0, len(prop.Children))
			for childName := range prop.Children {
				childNames = append(childNames, childName)
			}
			sort.Strings(childNames)

			for _, childName := range childNames {
				a.formatPropertyRecursive(text, childName, prop.Children[childName], indent+1)
			}
		}

	case rvfs.PropertyArray:
		fmt.Fprintf(text, "%s[gray]%s:[-] [%d items]\n", prefix, name, len(prop.Elements))
		// Show array elements
		for i, elem := range prop.Elements {
			elemName := fmt.Sprintf("[%d]", i)
			a.formatPropertyRecursive(text, elemName, elem, indent+1)
		}
	}
}

func (a *App) formatPropertyDetails(text *strings.Builder, prop *rvfs.Property) {
	switch prop.Type {
	case rvfs.PropertySimple:
		text.WriteString("[yellow]Type:[-] Simple Value\n\n")
		fmt.Fprintf(text, "[yellow]Value:[-] %v\n", formatValue(prop.Value))

	case rvfs.PropertyObject:
		text.WriteString("[yellow]Type:[-] Object\n\n")
		fmt.Fprintf(text, "[yellow]Properties:[-] %d\n\n", len(prop.Children))

		// Show nested properties
		propNames := make([]string, 0, len(prop.Children))
		for name := range prop.Children {
			propNames = append(propNames, name)
		}
		sort.Strings(propNames)

		for _, name := range propNames {
			child := prop.Children[name]
			fmt.Fprintf(text, "  [gray]%s:[-] ", name)

			switch child.Type {
			case rvfs.PropertySimple:
				fmt.Fprintf(text, "%v\n", formatValue(child.Value))
			case rvfs.PropertyObject:
				text.WriteString("{...}\n")
			case rvfs.PropertyArray:
				fmt.Fprintf(text, "[%d]\n", len(child.Elements))
			case rvfs.PropertyLink:
				fmt.Fprintf(text, "→ %s\n", child.LinkTarget)
			}
		}

	case rvfs.PropertyArray:
		text.WriteString("[yellow]Type:[-] Array\n\n")
		fmt.Fprintf(text, "[yellow]Elements:[-] %d\n", len(prop.Elements))

	case rvfs.PropertyLink:
		text.WriteString("[yellow]Type:[-] Link\n\n")
		fmt.Fprintf(text, "[yellow]Target:[-] %s\n", prop.LinkTarget)
	}
}

func (a *App) rebaseTree(node *tview.TreeNode) {
	if node == nil {
		return
	}

	ref := node.GetReference()
	if ref == nil {
		return
	}

	nodeRef, ok := ref.(*NodeRef)
	if !ok {
		return
	}

	// Check if this is a link that needs to be resolved
	if nodeRef.IsLink {
		// Set the target to the link target
		target := nodeRef.LinkTarget

		// Find the target node in our tree
		if targetNode, ok := a.nodeMap[target]; ok {
			// Node already exists - rebase to it
			currentRoot := a.tree.GetRoot()
			a.rootStack = append(a.rootStack, currentRoot)

			a.tree.SetRoot(targetNode)
			a.basePath = target
			a.status.SetText(fmt.Sprintf("[yellow::b]RFUI[-:-:-] | Subtree: [cyan]%s[-]", target))
			return
		}

		// Node not in tree yet (likely an orphan) - load and create it
		resource, err := a.vfs.Get(target)
		if err != nil {
			a.status.SetText(fmt.Sprintf("[red]Cannot load %s: %v[-]", target, err))
			return
		}

		// Get basename for node
		baseName := target
		parts := strings.Split(strings.TrimSuffix(target, "/"), "/")
		if len(parts) > 0 {
			baseName = parts[len(parts)-1]
		}

		// Create new node for this orphan resource
		orphanNode := tview.NewTreeNode(baseName).
			SetColor(tcell.ColorYellow).
			SetSelectable(true).
			SetReference(&NodeRef{
				Path:     target,
				Resource: resource,
				Parent:   a.fullRoot, // Orphans are top-level, parent is root
			})

		// Add to nodeMap
		a.nodeMap[target] = orphanNode

		// Build its children
		a.addResourceChildren(orphanNode, resource, target)

		// Rebase to the new orphan node
		currentRoot := a.tree.GetRoot()
		a.rootStack = append(a.rootStack, currentRoot)

		a.tree.SetRoot(orphanNode)
		a.basePath = target
		a.status.SetText(fmt.Sprintf("[yellow::b]RFUI[-:-:-] | Orphan: [cyan]%s[-]", target))
		return
	}

	// For non-links, rebase directly on the node
	currentRoot := a.tree.GetRoot()

	// Don't rebase if already at this node
	if currentRoot == node {
		a.status.SetText("[yellow]Already at this subtree[-]")
		return
	}

	// Push current root onto stack
	a.rootStack = append(a.rootStack, currentRoot)

	// Rebase - works for both resources AND properties!
	a.tree.SetRoot(node)
	a.basePath = nodeRef.Path

	// Update status to show what we're viewing
	nodeType := "Subtree"
	if nodeRef.Property != nil {
		switch nodeRef.Property.Type {
		case rvfs.PropertyObject:
			nodeType = "Property Object"
		case rvfs.PropertyArray:
			nodeType = "Property Array"
		case rvfs.PropertySimple:
			nodeType = "Property Value"
		}
	}
	a.status.SetText(fmt.Sprintf("[yellow::b]RFUI[-:-:-] | %s: [cyan]%s[-]", nodeType, nodeRef.Path))
}

func (a *App) popTree() {
	if len(a.rootStack) == 0 {
		a.status.SetText("[yellow]At root of navigation stack[-]")
		return
	}

	// Pop from stack
	previousRoot := a.rootStack[len(a.rootStack)-1]
	a.rootStack = a.rootStack[:len(a.rootStack)-1]

	// Set tree to previous root
	a.tree.SetRoot(previousRoot)

	// Update basePath
	if ref := previousRoot.GetReference(); ref != nil {
		if nodeRef, ok := ref.(*NodeRef); ok {
			a.basePath = nodeRef.Path
			if a.basePath == "/redfish/v1" {
				a.status.SetText("[yellow::b]RFUI[-:-:-] | Tree: [cyan]Full[-]")
			} else {
				a.status.SetText(fmt.Sprintf("[yellow::b]RFUI[-:-:-] | Subtree: [cyan]%s[-]", a.basePath))
			}
		}
	}
}

func (a *App) goUp() {
	currentRoot := a.tree.GetRoot()
	if currentRoot == nil {
		return
	}

	// Get the parent of current root
	ref := currentRoot.GetReference()
	if ref == nil {
		return
	}

	nodeRef, ok := ref.(*NodeRef)
	if !ok {
		return
	}

	// Check if we have a parent
	if nodeRef.Parent == nil {
		a.status.SetText("[yellow]Already at top level[-]")
		return
	}

	// Push current root onto stack for back navigation
	a.rootStack = append(a.rootStack, currentRoot)

	// Rebase on parent
	a.tree.SetRoot(nodeRef.Parent)

	// Update basePath
	if parentRef := nodeRef.Parent.GetReference(); parentRef != nil {
		if parentNodeRef, ok := parentRef.(*NodeRef); ok {
			a.basePath = parentNodeRef.Path
			a.status.SetText(fmt.Sprintf("[yellow::b]RFUI[-:-:-] | Parent: [cyan]%s[-]", a.basePath))
		}
	}
}

func (a *App) goHome() {
	if a.fullRoot == nil {
		a.status.SetText("[red]No root tree available[-]")
		return
	}

	currentRoot := a.tree.GetRoot()
	if currentRoot == a.fullRoot {
		a.status.SetText("[yellow]Already at root[-]")
		return
	}

	// Clear stack since we're going home
	a.rootStack = nil

	// Jump to original root
	a.tree.SetRoot(a.fullRoot)
	a.basePath = "/redfish/v1"
	a.status.SetText("[yellow::b]RFUI[-:-:-] | Tree: [cyan]Full[-]")
}

func formatValue(v any) string {
	if v == nil {
		return "[gray]null[-]"
	}

	switch val := v.(type) {
	case string:
		return fmt.Sprintf("[green]%q[-]", val)
	case bool:
		if val {
			return "[green]true[-]"
		}
		return "[red]false[-]"
	case float64:
		return fmt.Sprintf("[blue]%v[-]", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func (a *App) Run() error {
	return a.app.Run()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: rfui CONFIG_FILE")
		os.Exit(1)
	}

	// Load config
	type Config struct {
		Endpoint string `yaml:"endpoint"`
		User     string `yaml:"user"`
		Pass     string `yaml:"pass"`
		Insecure bool   `yaml:"insecure"`
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Printf("Error reading config: %v\n", err)
		os.Exit(1)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Printf("Error parsing config: %v\n", err)
		os.Exit(1)
	}

	// Create VFS
	vfs, err := rvfs.NewVFS(cfg.Endpoint, cfg.User, cfg.Pass, cfg.Insecure)
	if err != nil {
		fmt.Printf("Error creating VFS: %v\n", err)
		os.Exit(1)
	}
	defer vfs.Sync()

	// Run TUI
	app := NewApp(vfs)
	if err := app.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
