package main

import (
	"strings"
	"testing"

	"bluefish/rvfs"
)

// mockVFSForCompletion provides a minimal VFS for completion testing
type mockVFSForCompletion struct {
	resource *rvfs.Resource
}

func (m *mockVFSForCompletion) Get(path string) (*rvfs.Resource, error) {
	return m.resource, nil
}

func (m *mockVFSForCompletion) ListAll(path string) ([]*rvfs.Entry, error) {
	// Return top-level entries
	entries := []*rvfs.Entry{
		{Name: "Name", Type: rvfs.EntryProperty},
		{Name: "Status", Type: rvfs.EntryComplex},
		{Name: "Boot", Type: rvfs.EntryComplex},
		{Name: "Storage", Type: rvfs.EntryLink, Path: "/redfish/v1/Systems/1/Storage"},
	}
	return entries, nil
}

func (m *mockVFSForCompletion) ResolveTarget(basePath, targetPath string) (*rvfs.Target, error) {
	// Handle Boot property resolution
	if targetPath == "Boot" {
		bootProp := m.resource.Properties["Boot"]
		return &rvfs.Target{
			Type:         rvfs.TargetProperty,
			Resource:     m.resource,
			Property:     bootProp,
			ResourcePath: basePath,
		}, nil
	}

	// Handle Boot:BootOrder resolution
	if targetPath == "Boot:BootOrder" {
		bootProp := m.resource.Properties["Boot"]
		bootOrderProp := bootProp.Children["BootOrder"]
		return &rvfs.Target{
			Type:         rvfs.TargetProperty,
			Resource:     m.resource,
			Property:     bootOrderProp,
			ResourcePath: basePath,
		}, nil
	}

	return nil, &rvfs.NotFoundError{Path: targetPath}
}

func (m *mockVFSForCompletion) GetKnownPaths() []string {
	return []string{"/redfish/v1/Systems/1"}
}

func (m *mockVFSForCompletion) Clear() {}

func (m *mockVFSForCompletion) Sync() error {
	return nil
}

func (m *mockVFSForCompletion) Parent(path string) string {
	return "/redfish/v1"
}

func (m *mockVFSForCompletion) Stat(path string) (*rvfs.Entry, error) {
	return nil, nil
}

func (m *mockVFSForCompletion) Exists(path string) bool {
	return false
}

func (m *mockVFSForCompletion) ListChildren(path string) ([]*rvfs.Child, error) {
	return nil, nil
}

func (m *mockVFSForCompletion) ListProperties(path string) ([]*rvfs.Property, error) {
	return nil, nil
}

func (m *mockVFSForCompletion) GetProperty(resourcePath, propertyPath string) (*rvfs.Property, error) {
	return nil, nil
}

func (m *mockVFSForCompletion) ListProperty(resourcePath, propertyPath string) ([]*rvfs.Entry, error) {
	return nil, nil
}

func (m *mockVFSForCompletion) Resolve(currentPath, target string) (string, error) {
	return "", nil
}

func (m *mockVFSForCompletion) Join(base, target string) string {
	return ""
}

func (m *mockVFSForCompletion) Invalidate(path string) {}

func (m *mockVFSForCompletion) IsOffline() bool {
	return false
}

func (m *mockVFSForCompletion) SetOffline(offline bool) {}

func createTestResource() *rvfs.Resource {
	return &rvfs.Resource{
		Path: "/redfish/v1/Systems/1",
		Properties: map[string]*rvfs.Property{
			"Name": {
				Name:  "Name",
				Type:  rvfs.PropertySimple,
				Value: "System",
			},
			"Status": {
				Name: "Status",
				Type: rvfs.PropertyObject,
				Children: map[string]*rvfs.Property{
					"Health": {Name: "Health", Type: rvfs.PropertySimple, Value: "OK"},
					"State":  {Name: "State", Type: rvfs.PropertySimple, Value: "Enabled"},
				},
			},
			"Boot": {
				Name: "Boot",
				Type: rvfs.PropertyObject,
				Children: map[string]*rvfs.Property{
					"BootOrder": {
						Name: "BootOrder",
						Type: rvfs.PropertyArray,
						Elements: []*rvfs.Property{
							{Name: "[0]", Type: rvfs.PropertySimple, Value: "Pxe"},
							{Name: "[1]", Type: rvfs.PropertySimple, Value: "Hdd"},
						},
					},
					"BootSourceOverrideTarget": {
						Name:       "BootSourceOverrideTarget",
						Type:       rvfs.PropertyLink,
						LinkTarget: "/redfish/v1/Systems/1/BootOptions/Pxe",
					},
				},
			},
		},
		Children: map[string]*rvfs.Child{
			"Storage": {
				Name:   "Storage",
				Type:   rvfs.ChildLink,
				Target: "/redfish/v1/Systems/1/Storage",
				Parent: "/redfish/v1/Systems/1",
			},
		},
	}
}

func TestCompleter_PropertyCompletion(t *testing.T) {
	resource := createTestResource()
	vfs := &mockVFSForCompletion{resource: resource}
	nav := &Navigator{
		vfs: vfs,
		cwd: "/redfish/v1/Systems/1",
	}
	completer := NewCompleter(nav)

	tests := []struct {
		name           string
		partial        string
		expectedPrefix string
		wantMatch      []string // At least these should be in results
	}{
		{
			name:           "complete after property colon",
			partial:        "Boot:",
			expectedPrefix: "",
			wantMatch:      []string{"BootOrder", "BootSourceOverrideTarget"},
		},
		{
			name:           "complete partial property name after colon",
			partial:        "Boot:Boot",
			expectedPrefix: "Boot",
			wantMatch:      []string{"BootOrder", "BootSourceOverrideTarget"},
		},
		{
			name:           "complete at top level",
			partial:        "",
			expectedPrefix: "",
			wantMatch:      []string{"Name", "Status", "Boot", "Storage"},
		},
		{
			name:           "complete partial top level",
			partial:        "Bo",
			expectedPrefix: "Bo",
			wantMatch:      []string{"Boot"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completions, prefixLen := completer.completePath(tt.partial)

			if prefixLen != len(tt.expectedPrefix) {
				t.Errorf("Expected prefix length %d, got %d", len(tt.expectedPrefix), prefixLen)
			}

			// Convert rune slices back to strings for easier testing
			results := make([]string, len(completions))
			for i, c := range completions {
				results[i] = tt.expectedPrefix + string(c)
			}

			// Check that all expected matches are present
			for _, want := range tt.wantMatch {
				found := false
				for _, got := range results {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected completion %q not found in results: %v", want, results)
				}
			}
		})
	}
}

func TestCompleter_IncompleteBracket(t *testing.T) {
	resource := createTestResource()
	vfs := &mockVFSForCompletion{resource: resource}
	nav := &Navigator{
		vfs: vfs,
		cwd: "/redfish/v1/Systems/1",
	}
	completer := NewCompleter(nav)

	// This should not panic when user types "Boot:BootOrder[" during tab completion
	// It will fail to resolve, but shouldn't crash
	completions, _ := completer.completePath("Boot:BootOrder[")

	// We expect empty or error, but NOT a panic
	// The key test is that we got here without crashing
	_ = completions
}

func TestCompleter_ArrayIndexCompletion(t *testing.T) {
	resource := createTestResource()
	vfs := &mockVFSForCompletion{resource: resource}
	nav := &Navigator{
		vfs: vfs,
		cwd: "/redfish/v1/Systems/1",
	}
	completer := NewCompleter(nav)

	// When completing after "[", should return just the index numbers, not "[0]"
	// This prevents "BootOrder[<tab>" from becoming "BootOrder[[0]"
	completions, prefixLen := completer.completePath("Boot:BootOrder[")

	if prefixLen != 0 {
		t.Errorf("Expected prefix length 0, got %d", prefixLen)
	}

	// Convert rune slices back to strings
	results := make([]string, len(completions))
	for i, c := range completions {
		results[i] = string(c)
	}

	// Should have completions for array indices
	if len(results) == 0 {
		t.Fatal("Expected array index completions, got none")
	}

	// Verify completions are just numbers, not "[0]", "[1]"
	for _, result := range results {
		if strings.HasPrefix(result, "[") {
			t.Errorf("Array index completion should not include brackets, got: %q", result)
		}
		// Should be numeric
		if result != "0" && result != "1" {
			t.Errorf("Expected numeric index, got: %q", result)
		}
	}

	// Verify we got both indices
	found0 := false
	found1 := false
	for _, result := range results {
		if result == "0" {
			found0 = true
		}
		if result == "1" {
			found1 = true
		}
	}

	if !found0 || !found1 {
		t.Errorf("Expected completions for indices 0 and 1, got: %v", results)
	}
}

func TestCompleter_InvalidSeparatorCombinations(t *testing.T) {
	resource := createTestResource()
	vfs := &mockVFSForCompletion{resource: resource}
	nav := &Navigator{
		vfs: vfs,
		cwd: "/redfish/v1/Systems/1",
	}
	completer := NewCompleter(nav)

	tests := []struct {
		name        string
		partial     string
		shouldEmpty bool
		reason      string
	}{
		{
			name:        "colon after array property",
			partial:     "Boot:BootOrder:",
			shouldEmpty: true,
			reason:      "Cannot use : separator on array - must use [",
		},
		{
			name:        "bracket after object property",
			partial:     "Boot[",
			shouldEmpty: true,
			reason:      "Cannot use [ separator on object - must use :",
		},
		{
			name:        "bracket after nested object",
			partial:     "Boot:BootSourceOverrideTarget[",
			shouldEmpty: true,
			reason:      "Cannot use [ separator on link property",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completions, _ := completer.completePath(tt.partial)

			if tt.shouldEmpty && len(completions) != 0 {
				t.Errorf("%s: expected no completions (invalid syntax), got %d", tt.reason, len(completions))
			}
		})
	}
}

func TestCompleter_ComplexSeparatorCompositions(t *testing.T) {
	// Test valid complex combinations of separators
	// This verifies our generic separator handling works for multi-level navigation

	// Create a more complex resource with nested arrays of objects
	complexResource := &rvfs.Resource{
		Path: "/redfish/v1/Systems/1",
		Properties: map[string]*rvfs.Property{
			"PCIeDevices": {
				Name: "PCIeDevices",
				Type: rvfs.PropertyArray,
				Elements: []*rvfs.Property{
					{
						Name: "[0]",
						Type: rvfs.PropertyObject,
						Children: map[string]*rvfs.Property{
							"DeviceType": {
								Name:  "DeviceType",
								Type:  rvfs.PropertySimple,
								Value: "GPU",
							},
							"FirmwareVersion": {
								Name:  "FirmwareVersion",
								Type:  rvfs.PropertySimple,
								Value: "1.2.3",
							},
						},
					},
				},
			},
		},
	}

	// Create a custom mock VFS for this test
	mockVFS := &mockVFSForComplexCompletion{resource: complexResource}

	nav := &Navigator{
		vfs: mockVFS,
		cwd: "/redfish/v1/Systems/1",
	}
	completer := NewCompleter(nav)

	// Test: After navigating to array element with [, then use : to complete properties
	// Pattern: ArrayProp[index]:
	completions, prefixLen := completer.completePath("PCIeDevices[0]:")

	if prefixLen != 0 {
		t.Errorf("Expected prefix length 0, got %d", prefixLen)
	}

	// Convert results
	results := make([]string, len(completions))
	for i, c := range completions {
		results[i] = string(c)
	}

	// Should get property completions from the object at array index 0
	if len(results) == 0 {
		t.Error("Expected property completions after navigating into array element, got none")
	}

	// Check that we get the expected properties
	expectedProps := map[string]bool{
		"DeviceType":      false,
		"FirmwareVersion": false,
	}

	for _, result := range results {
		if _, exists := expectedProps[result]; exists {
			expectedProps[result] = true
		}
	}

	for prop, found := range expectedProps {
		if !found {
			t.Errorf("Expected property %q in completions after PCIeDevices[0]:, got: %v", prop, results)
		}
	}
}

// mockVFSForComplexCompletion is a specialized mock for testing complex separator compositions
type mockVFSForComplexCompletion struct {
	resource *rvfs.Resource
}

func (m *mockVFSForComplexCompletion) Get(path string) (*rvfs.Resource, error) {
	return m.resource, nil
}

func (m *mockVFSForComplexCompletion) ListAll(path string) ([]*rvfs.Entry, error) {
	return nil, nil
}

func (m *mockVFSForComplexCompletion) ResolveTarget(basePath, targetPath string) (*rvfs.Target, error) {
	// Handle PCIeDevices[0] - should resolve to the array element (an object)
	if targetPath == "PCIeDevices[0]" {
		arrayProp := m.resource.Properties["PCIeDevices"]
		elementProp := arrayProp.Elements[0]
		return &rvfs.Target{
			Type:         rvfs.TargetProperty,
			Resource:     m.resource,
			Property:     elementProp,
			ResourcePath: basePath,
		}, nil
	}
	return nil, &rvfs.NotFoundError{Path: targetPath}
}

func (m *mockVFSForComplexCompletion) GetKnownPaths() []string               { return nil }
func (m *mockVFSForComplexCompletion) Clear()                                {}
func (m *mockVFSForComplexCompletion) Sync() error                           { return nil }
func (m *mockVFSForComplexCompletion) Parent(path string) string             { return "" }
func (m *mockVFSForComplexCompletion) Stat(path string) (*rvfs.Entry, error) { return nil, nil }
func (m *mockVFSForComplexCompletion) Exists(path string) bool               { return false }
func (m *mockVFSForComplexCompletion) ListChildren(path string) ([]*rvfs.Child, error) {
	return nil, nil
}
func (m *mockVFSForComplexCompletion) ListProperties(path string) ([]*rvfs.Property, error) {
	return nil, nil
}
func (m *mockVFSForComplexCompletion) GetProperty(rp, pp string) (*rvfs.Property, error) {
	return nil, nil
}
func (m *mockVFSForComplexCompletion) ListProperty(rp, pp string) ([]*rvfs.Entry, error) {
	return nil, nil
}
func (m *mockVFSForComplexCompletion) Resolve(cp, t string) (string, error) { return "", nil }
func (m *mockVFSForComplexCompletion) Join(b, t string) string              { return "" }
func (m *mockVFSForComplexCompletion) Invalidate(path string)               {}
func (m *mockVFSForComplexCompletion) IsOffline() bool                      { return false }
func (m *mockVFSForComplexCompletion) SetOffline(offline bool)              {}
