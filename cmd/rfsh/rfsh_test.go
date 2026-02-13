package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"bluefish/rvfs"
)

// captureOutput captures stdout during function execution
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// mockVFS creates a minimal VFS for testing
type mockVFS struct {
	resources map[string]*rvfs.Resource
}

func (m *mockVFS) Get(path string) (*rvfs.Resource, error) {
	if res, ok := m.resources[path]; ok {
		return res, nil
	}
	return nil, fmt.Errorf("not found: %s", path)
}

func (m *mockVFS) ListAll(path string) ([]*rvfs.Entry, error) {
	return nil, nil
}

func (m *mockVFS) ResolveTarget(basePath, targetPath string) (*rvfs.Target, error) {
	return nil, nil
}

func (m *mockVFS) GetKnownPaths() []string {
	return nil
}

func (m *mockVFS) Clear() {}

func (m *mockVFS) Sync() error {
	return nil
}

func (m *mockVFS) Parent(path string) string {
	return "/redfish/v1"
}

func TestShowProperty_SimpleValue(t *testing.T) {
	nav := &Navigator{cwd: "/redfish/v1"}

	prop := &rvfs.Property{
		Name:  "Health",
		Type:  rvfs.PropertySimple,
		Value: "OK",
	}

	output := captureOutput(func() {
		nav.showProperty(prop, 0, false)
	})

	expected := "Health: OK\n"
	if output != expected {
		t.Errorf("Expected %q, got %q", expected, output)
	}
}

func TestShowProperty_Link(t *testing.T) {
	nav := &Navigator{cwd: "/redfish/v1"}

	prop := &rvfs.Property{
		Name:       "Target",
		Type:       rvfs.PropertyLink,
		LinkTarget: "/redfish/v1/Systems/1",
	}

	output := captureOutput(func() {
		nav.showProperty(prop, 0, false)
	})

	if !strings.Contains(output, "link →") {
		t.Errorf("Expected link indicator, got %q", output)
	}
	if !strings.Contains(output, "/redfish/v1/Systems/1") {
		t.Errorf("Expected link target, got %q", output)
	}
}

func TestShowProperty_EmptyObject(t *testing.T) {
	nav := &Navigator{cwd: "/redfish/v1"}

	prop := &rvfs.Property{
		Name:     "EmptyObj",
		Type:     rvfs.PropertyObject,
		Children: map[string]*rvfs.Property{},
	}

	output := captureOutput(func() {
		nav.showProperty(prop, 0, false)
	})

	expected := "EmptyObj: {}\n"
	if output != expected {
		t.Errorf("Expected %q, got %q", expected, output)
	}
}

func TestShowProperty_EmptyArray(t *testing.T) {
	nav := &Navigator{cwd: "/redfish/v1"}

	prop := &rvfs.Property{
		Name:     "EmptyArr",
		Type:     rvfs.PropertyArray,
		Elements: []*rvfs.Property{},
	}

	output := captureOutput(func() {
		nav.showProperty(prop, 0, false)
	})

	expected := "EmptyArr: []\n"
	if output != expected {
		t.Errorf("Expected %q, got %q", expected, output)
	}
}

func TestShowProperty_SimpleObject(t *testing.T) {
	nav := &Navigator{cwd: "/redfish/v1"}

	prop := &rvfs.Property{
		Name: "Status",
		Type: rvfs.PropertyObject,
		Children: map[string]*rvfs.Property{
			"Health": {
				Name:  "Health",
				Type:  rvfs.PropertySimple,
				Value: "OK",
			},
			"State": {
				Name:  "State",
				Type:  rvfs.PropertySimple,
				Value: "Enabled",
			},
		},
	}

	output := captureOutput(func() {
		nav.showProperty(prop, 0, false)
	})

	// Should start with property name
	if !strings.HasPrefix(output, "Status:") {
		t.Errorf("Expected property name 'Status:', got %q", output)
	}

	// Should contain both fields indented
	if !strings.Contains(output, "Health:") {
		t.Errorf("Expected Health field, got %q", output)
	}
	if !strings.Contains(output, "State:") {
		t.Errorf("Expected State field, got %q", output)
	}
	if !strings.Contains(output, "OK") {
		t.Errorf("Expected OK value, got %q", output)
	}
	if !strings.Contains(output, "Enabled") {
		t.Errorf("Expected Enabled value, got %q", output)
	}
}

func TestShowProperty_ArrayOfSimpleValues(t *testing.T) {
	nav := &Navigator{cwd: "/redfish/v1"}

	prop := &rvfs.Property{
		Name: "BootOrder",
		Type: rvfs.PropertyArray,
		Elements: []*rvfs.Property{
			{
				Name:  "[0]",
				Type:  rvfs.PropertySimple,
				Value: "Pxe",
			},
			{
				Name:  "[1]",
				Type:  rvfs.PropertySimple,
				Value: "Hdd",
			},
		},
	}

	output := captureOutput(func() {
		nav.showProperty(prop, 0, false)
	})

	// Should start with property name
	if !strings.HasPrefix(output, "BootOrder:") {
		t.Errorf("Expected property name 'BootOrder:', got %q", output)
	}

	// Should have dashes for each element
	dashCount := strings.Count(output, "- ")
	if dashCount != 2 {
		t.Errorf("Expected 2 dashes for array elements, got %d in %q", dashCount, output)
	}

	// Should contain values inline with dashes
	if !strings.Contains(output, "- Pxe") {
		t.Errorf("Expected inline value after dash, got %q", output)
	}
	if !strings.Contains(output, "- Hdd") {
		t.Errorf("Expected inline value after dash, got %q", output)
	}
}

func TestShowProperty_ArrayOfObjects(t *testing.T) {
	nav := &Navigator{cwd: "/redfish/v1"}

	prop := &rvfs.Property{
		Name: "Capabilities",
		Type: rvfs.PropertyArray,
		Elements: []*rvfs.Property{
			{
				Name: "[0]",
				Type: rvfs.PropertyObject,
				Children: map[string]*rvfs.Property{
					"CapabilitiesObject": {
						Name:       "CapabilitiesObject",
						Type:       rvfs.PropertyLink,
						LinkTarget: "/redfish/v1/Systems/Capabilities",
					},
					"UseCase": {
						Name:  "UseCase",
						Type:  rvfs.PropertySimple,
						Value: "ComputerSystemComposition",
					},
				},
			},
		},
	}

	output := captureOutput(func() {
		nav.showProperty(prop, 0, false)
	})

	// Critical test: First field should be inline with dash
	// Should NOT have "- \n  CapabilitiesObject:"
	// Should HAVE "- CapabilitiesObject:"
	if strings.Contains(output, "- \n") {
		t.Errorf("Found dash followed by newline (incorrect YAML formatting): %q", output)
	}

	// Should have first field inline with dash
	if !strings.Contains(output, "- CapabilitiesObject:") {
		t.Errorf("Expected first field inline with dash, got %q", output)
	}

	// Second field should be indented at same level as first
	lines := strings.Split(output, "\n")
	var capabilitiesLine, useCaseLine string
	for _, line := range lines {
		if strings.Contains(line, "CapabilitiesObject:") {
			capabilitiesLine = line
		}
		if strings.Contains(line, "UseCase:") {
			useCaseLine = line
		}
	}

	if capabilitiesLine == "" || useCaseLine == "" {
		t.Fatalf("Missing expected fields in output: %q", output)
	}

	// Both fields should start at same column
	// Find the position where field name starts (after leading spaces/dash)
	capabilitiesFieldStart := strings.Index(capabilitiesLine, "CapabilitiesObject")
	useCaseFieldStart := strings.Index(useCaseLine, "UseCase")

	if capabilitiesFieldStart != useCaseFieldStart {
		t.Errorf("Field alignment mismatch: CapabilitiesObject at column %d, UseCase at column %d\nCapabilities line: %q\nUseCase line: %q\nFull output:\n%s",
			capabilitiesFieldStart, useCaseFieldStart, capabilitiesLine, useCaseLine, output)
	}

	// Verify the format matches YAML: "- FieldName:" not "- \n  FieldName:"
	if strings.Contains(capabilitiesLine, "- CapabilitiesObject:") {
		// Good - first field is inline with dash
	} else {
		t.Errorf("First field should be inline with dash, got: %q", capabilitiesLine)
	}
}

func TestShowProperty_ArrayOfObjects_WithNestedObject(t *testing.T) {
	nav := &Navigator{cwd: "/redfish/v1"}

	prop := &rvfs.Property{
		Name: "Capabilities",
		Type: rvfs.PropertyArray,
		Elements: []*rvfs.Property{
			{
				Name: "[0]",
				Type: rvfs.PropertyObject,
				Children: map[string]*rvfs.Property{
					"Links": {
						Name: "Links",
						Type: rvfs.PropertyObject,
						Children: map[string]*rvfs.Property{
							"TargetCollection": {
								Name:       "TargetCollection",
								Type:       rvfs.PropertyLink,
								LinkTarget: "/redfish/v1/Systems",
							},
						},
					},
					"UseCase": {
						Name:  "UseCase",
						Type:  rvfs.PropertySimple,
						Value: "ComputerSystemComposition",
					},
				},
			},
		},
	}

	output := captureOutput(func() {
		nav.showProperty(prop, 0, false)
	})

	// The output should be:
	//
	//   - Links:
	//       TargetCollection: link → /redfish/v1/Systems
	//     UseCase: ComputerSystemComposition
	//
	// Key points:
	// 1. "Links:" is inline with dash (first field)
	// 2. "TargetCollection:" is indented 2 more spaces (child of Links)
	// 3. "UseCase:" aligns with "Links:" (both top-level fields)

	lines := strings.Split(output, "\n")
	var linksLine, targetLine, useCaseLine string
	for _, line := range lines {
		if strings.Contains(line, "Links:") {
			linksLine = line
		}
		if strings.Contains(line, "TargetCollection:") {
			targetLine = line
		}
		if strings.Contains(line, "UseCase:") {
			useCaseLine = line
		}
	}

	if linksLine == "" || targetLine == "" || useCaseLine == "" {
		t.Fatalf("Missing expected fields in output:\n%s", output)
	}

	// Check first field is inline with dash
	if !strings.Contains(linksLine, "- Links:") {
		t.Errorf("Expected 'Links:' inline with dash, got: %q", linksLine)
	}

	// Find column positions
	linksCol := strings.Index(linksLine, "Links:")
	targetCol := strings.Index(targetLine, "TargetCollection:")
	useCaseCol := strings.Index(useCaseLine, "UseCase:")

	// TargetCollection should be indented 2 more than Links (it's a child)
	expectedTargetCol := linksCol + 2
	if targetCol != expectedTargetCol {
		t.Errorf("TargetCollection indentation incorrect: expected column %d, got %d\nLinks line:  %q\nTarget line: %q",
			expectedTargetCol, targetCol, linksLine, targetLine)
	}

	// UseCase should align with Links (both are top-level fields)
	if useCaseCol != linksCol {
		t.Errorf("UseCase should align with Links: Links at column %d, UseCase at column %d\nLinks line:   %q\nUseCase line: %q",
			linksCol, useCaseCol, linksLine, useCaseLine)
	}
}

func TestShowProperty_NestedArrays(t *testing.T) {
	nav := &Navigator{cwd: "/redfish/v1"}

	prop := &rvfs.Property{
		Name: "Links",
		Type: rvfs.PropertyObject,
		Children: map[string]*rvfs.Property{
			"RelatedItem": {
				Name: "RelatedItem",
				Type: rvfs.PropertyArray,
				Elements: []*rvfs.Property{
					{
						Name:       "[0]",
						Type:       rvfs.PropertyLink,
						LinkTarget: "/redfish/v1/Systems/1",
					},
					{
						Name:       "[1]",
						Type:       rvfs.PropertyLink,
						LinkTarget: "/redfish/v1/Systems/2",
					},
				},
			},
		},
	}

	output := captureOutput(func() {
		nav.showProperty(prop, 0, false)
	})

	// Should have proper indentation for nested array
	if !strings.Contains(output, "RelatedItem:") {
		t.Errorf("Expected RelatedItem field, got %q", output)
	}

	// Array elements should have dashes
	dashCount := strings.Count(output, "- link →")
	if dashCount != 2 {
		t.Errorf("Expected 2 array elements with dashes, got %d in %q", dashCount, output)
	}
}
