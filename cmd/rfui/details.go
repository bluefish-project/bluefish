package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"bluefish/rvfs"
)

// DetailsModel manages the details panel with a scrollable viewport
type DetailsModel struct {
	viewport viewport.Model
	content  string
	ready    bool
}

func NewDetailsModel() DetailsModel {
	return DetailsModel{}
}

func (d *DetailsModel) SetSize(width, height int) {
	if !d.ready {
		d.viewport = viewport.New(width, height)
		d.viewport.SetContent(d.content)
		d.ready = true
	} else {
		d.viewport.Width = width
		d.viewport.Height = height
	}
}

func (d *DetailsModel) Update(msg tea.Msg) {
	d.viewport, _ = d.viewport.Update(msg)
}

func (d *DetailsModel) ScrollDown() {
	d.viewport.ScrollDown(1)
}

func (d *DetailsModel) ScrollUp() {
	d.viewport.ScrollUp(1)
}

// SetItem updates the details panel to show info about a tree item
func (d *DetailsModel) SetItem(item *TreeItem) {
	if item == nil {
		d.content = ""
		if d.ready {
			d.viewport.SetContent("")
			d.viewport.GotoTop()
		}
		return
	}

	var b strings.Builder

	// Path
	b.WriteString(detailLabelStyle.Render("Path: "))
	b.WriteString(detailValueStyle.Render(item.Path))
	b.WriteString("\n\n")

	switch item.Kind {
	case KindResource:
		d.renderResource(&b, item)
	case KindChild:
		d.renderChild(&b, item)
	case KindSimple:
		d.renderSimple(&b, item)
	case KindObject:
		d.renderObject(&b, item)
	case KindArray:
		d.renderArray(&b, item)
	case KindLink:
		d.renderLink(&b, item)
	}

	d.content = b.String()
	if d.ready {
		d.viewport.SetContent(d.content)
		d.viewport.GotoTop()
	}
}

func (d *DetailsModel) renderResource(b *strings.Builder, item *TreeItem) {
	b.WriteString(detailLabelStyle.Render("Type: "))
	b.WriteString("Resource\n")

	if item.Resource == nil {
		return
	}

	if item.Resource.ODataType != "" {
		b.WriteString(detailLabelStyle.Render("@odata.type: "))
		b.WriteString(detailValueStyle.Render(item.Resource.ODataType))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if len(item.Resource.Children) > 0 {
		b.WriteString(detailLabelStyle.Render(fmt.Sprintf("Children: %d\n", len(item.Resource.Children))))
		childNames := sortedKeys(item.Resource.Children)
		for _, name := range childNames {
			child := item.Resource.Children[name]
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				childStyle.Render(name),
				linkStyle.Render("→"),
				detailValueStyle.Render(child.Target)))
		}
		b.WriteString("\n")
	}

	if len(item.Resource.Properties) > 0 {
		b.WriteString(detailLabelStyle.Render("Properties:\n"))
		propNames := make([]string, 0, len(item.Resource.Properties))
		for n := range item.Resource.Properties {
			propNames = append(propNames, n)
		}
		sort.Strings(propNames)
		for _, name := range propNames {
			prop := item.Resource.Properties[name]
			d.renderPropertyRecursive(b, name, prop, 1)
		}
	}
}

func (d *DetailsModel) renderChild(b *strings.Builder, item *TreeItem) {
	b.WriteString(detailLabelStyle.Render("Type: "))
	b.WriteString("Child Resource\n")

	if item.Child != nil {
		b.WriteString(detailLabelStyle.Render("Target: "))
		b.WriteString(childStyle.Render(item.Child.Target))
		b.WriteString("\n")
		if item.Child.IsExternal() {
			b.WriteString(detailLabelStyle.Render("External: "))
			b.WriteString("yes (symlink)\n")
		}
	}

	if item.Resource != nil {
		b.WriteString("\n")
		d.renderResourceProperties(b, item.Resource)
	}
}

func (d *DetailsModel) renderSimple(b *strings.Builder, item *TreeItem) {
	b.WriteString(detailLabelStyle.Render("Type: "))
	b.WriteString("Value\n\n")
	b.WriteString(detailLabelStyle.Render("Value: "))
	if item.Property != nil {
		b.WriteString(formatHealthValue(item.Name, item.Property.Value))
	}
	b.WriteString("\n")
}

func (d *DetailsModel) renderObject(b *strings.Builder, item *TreeItem) {
	b.WriteString(detailLabelStyle.Render("Type: "))
	b.WriteString("Object\n")
	b.WriteString(detailLabelStyle.Render(fmt.Sprintf("Fields: %d\n\n", item.ChildCount)))

	if item.Property != nil {
		childNames := make([]string, 0, len(item.Property.Children))
		for n := range item.Property.Children {
			childNames = append(childNames, n)
		}
		sort.Strings(childNames)
		for _, name := range childNames {
			child := item.Property.Children[name]
			d.renderPropertyRecursive(b, name, child, 0)
		}
	}
}

func (d *DetailsModel) renderArray(b *strings.Builder, item *TreeItem) {
	b.WriteString(detailLabelStyle.Render("Type: "))
	b.WriteString("Array\n")
	b.WriteString(detailLabelStyle.Render(fmt.Sprintf("Elements: %d\n\n", item.ChildCount)))

	if item.Property != nil {
		for i, elem := range item.Property.Elements {
			name := fmt.Sprintf("[%d]", i)
			d.renderPropertyRecursive(b, name, elem, 0)
		}
	}
}

func (d *DetailsModel) renderLink(b *strings.Builder, item *TreeItem) {
	b.WriteString(detailLabelStyle.Render("Type: "))
	b.WriteString("Link\n\n")
	b.WriteString(detailLabelStyle.Render("Target: "))
	b.WriteString(linkStyle.Render(item.LinkTarget))
	b.WriteString("\n\n")
	b.WriteString(helpDescStyle.Render("Press Enter to follow"))
	b.WriteString("\n")
}

func (d *DetailsModel) renderResourceProperties(b *strings.Builder, resource *rvfs.Resource) {
	propNames := make([]string, 0, len(resource.Properties))
	for n := range resource.Properties {
		propNames = append(propNames, n)
	}
	sort.Strings(propNames)

	b.WriteString(detailLabelStyle.Render("Properties:\n"))
	for _, name := range propNames {
		prop := resource.Properties[name]
		d.renderPropertyRecursive(b, name, prop, 1)
	}
}

func (d *DetailsModel) renderPropertyRecursive(b *strings.Builder, name string, prop *rvfs.Property, indent int) {
	prefix := strings.Repeat("  ", indent)

	switch prop.Type {
	case rvfs.PropertySimple:
		b.WriteString(fmt.Sprintf("%s%s: %s\n", prefix, propNameStyle.Render(name), formatHealthValue(name, prop.Value)))

	case rvfs.PropertyLink:
		b.WriteString(fmt.Sprintf("%s%s: %s %s\n", prefix, propNameStyle.Render(name), linkStyle.Render("→"), linkStyle.Render(prop.LinkTarget)))

	case rvfs.PropertyObject:
		b.WriteString(fmt.Sprintf("%s%s:\n", prefix, propNameStyle.Render(name)))
		childNames := make([]string, 0, len(prop.Children))
		for n := range prop.Children {
			childNames = append(childNames, n)
		}
		sort.Strings(childNames)
		for _, cn := range childNames {
			d.renderPropertyRecursive(b, cn, prop.Children[cn], indent+1)
		}

	case rvfs.PropertyArray:
		b.WriteString(fmt.Sprintf("%s%s: [%d]\n", prefix, propNameStyle.Render(name), len(prop.Elements)))
		for i, elem := range prop.Elements {
			elemName := fmt.Sprintf("[%d]", i)
			d.renderPropertyRecursive(b, elemName, elem, indent+1)
		}
	}
}

func (d *DetailsModel) View() string {
	if !d.ready {
		return ""
	}
	return d.viewport.View()
}

// sortedKeys returns sorted keys from a map[string]*rvfs.Child
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
