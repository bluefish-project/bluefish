package main

import (
	"strings"
)

// BreadcrumbModel renders a path as styled segments
type BreadcrumbModel struct {
	path     string
	maxWidth int
}

func NewBreadcrumbModel() BreadcrumbModel {
	return BreadcrumbModel{path: "/redfish/v1"}
}

func (b *BreadcrumbModel) SetPath(path string) {
	b.path = path
}

func (b *BreadcrumbModel) SetWidth(width int) {
	b.maxWidth = width
}

func (b *BreadcrumbModel) View() string {
	if b.path == "" {
		return ""
	}

	segments := strings.Split(strings.TrimPrefix(b.path, "/"), "/")
	sep := breadcrumbSepStyle.Render(" > ")

	// Build from right, truncate from left if needed
	var parts []string
	for i, seg := range segments {
		if i == len(segments)-1 {
			parts = append(parts, breadcrumbLastStyle.Render(seg))
		} else {
			parts = append(parts, breadcrumbStyle.Render(seg))
		}
	}

	result := strings.Join(parts, sep)

	// If too wide, truncate from the left
	if b.maxWidth > 0 {
		plain := strings.Join(segments, " > ")
		for len(plain) > b.maxWidth && len(parts) > 2 {
			parts = parts[1:]
			parts[0] = breadcrumbSepStyle.Render("..") + sep + parts[0]
			segments = segments[1:]
			plain = ".. > " + strings.Join(segments, " > ")
		}
		result = strings.Join(parts, sep)
	}

	return result
}
