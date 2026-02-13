package rvfs

import (
	"testing"
)

// Test data
var serviceRoot = []byte(`{
	"@odata.id": "/redfish/v1",
	"@odata.type": "#ServiceRoot.v1_0_0.ServiceRoot",
	"Id": "RootService",
	"Name": "Root Service",
	"RedfishVersion": "1.6.0",
	"Systems": {
		"@odata.id": "/redfish/v1/Systems"
	},
	"Chassis": {
		"@odata.id": "/redfish/v1/Chassis"
	}
}`)

var systemsCollection = []byte(`{
	"@odata.id": "/redfish/v1/Systems",
	"@odata.type": "#ComputerSystemCollection.ComputerSystemCollection",
	"Name": "Computer System Collection",
	"Members": [
		{"@odata.id": "/redfish/v1/Systems/1"}
	],
	"Members@odata.count": 1
}`)

var system1 = []byte(`{
	"@odata.id": "/redfish/v1/Systems/1",
	"@odata.type": "#ComputerSystem.v1_0_0.ComputerSystem",
	"Id": "1",
	"Name": "System 1",
	"Status": {
		"State": "Enabled",
		"Health": "OK"
	},
	"Boot": {
		"BootOrder": ["Pxe", "Hdd", "Usb"]
	},
	"Links": {
		"Chassis": [
			{"@odata.id": "/redfish/v1/Chassis/1"}
		]
	}
}`)

// TestParser_Basic tests basic parsing functionality
func TestParser_Basic(t *testing.T) {
	parser := NewParser()

	t.Run("parse service root", func(t *testing.T) {
		resource, err := parser.Parse("/redfish/v1", serviceRoot)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		if resource.ODataID != "/redfish/v1" {
			t.Errorf("ODataID = %q, want %q", resource.ODataID, "/redfish/v1")
		}

		if len(resource.Children) != 2 {
			t.Errorf("Children count = %d, want 2", len(resource.Children))
		}

		if _, ok := resource.Children["Systems"]; !ok {
			t.Error("Missing Systems child")
		}

		if len(resource.Properties) != 3 { // Id, Name, RedfishVersion
			t.Errorf("Properties count = %d, want 3", len(resource.Properties))
		}
	})

	t.Run("parse collection with Members", func(t *testing.T) {
		resource, err := parser.Parse("/redfish/v1/Systems", systemsCollection)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		if len(resource.Children) != 1 {
			t.Errorf("Children count = %d, want 1", len(resource.Children))
		}

		if child, ok := resource.Children["1"]; !ok {
			t.Error("Missing child '1'")
		} else if child.Target != "/redfish/v1/Systems/1" {
			t.Errorf("Child target = %q, want %q", child.Target, "/redfish/v1/Systems/1")
		}
	})

	t.Run("parse nested properties", func(t *testing.T) {
		resource, err := parser.Parse("/redfish/v1/Systems/1", system1)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		status := resource.Properties["Status"]
		if status == nil {
			t.Fatal("Missing Status property")
		}
		if status.Type != PropertyObject {
			t.Errorf("Status type = %v, want PropertyObject", status.Type)
		}

		health := status.Children["Health"]
		if health == nil {
			t.Fatal("Missing Status.Health")
		}
		if health.Type != PropertySimple {
			t.Errorf("Health type = %v, want PropertySimple", health.Type)
		}
		if health.Value != "OK" {
			t.Errorf("Health value = %v, want OK", health.Value)
		}
	})

	t.Run("parse array properties", func(t *testing.T) {
		resource, err := parser.Parse("/redfish/v1/Systems/1", system1)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		boot := resource.Properties["Boot"]
		if boot == nil {
			t.Fatal("Missing Boot property")
		}

		bootOrder := boot.Children["BootOrder"]
		if bootOrder == nil {
			t.Fatal("Missing Boot.BootOrder")
		}
		if bootOrder.Type != PropertyArray {
			t.Errorf("BootOrder type = %v, want PropertyArray", bootOrder.Type)
		}
		if len(bootOrder.Elements) != 3 {
			t.Errorf("BootOrder elements = %d, want 3", len(bootOrder.Elements))
		}
		if bootOrder.Elements[0].Value != "Pxe" {
			t.Errorf("BootOrder[0] = %v, want Pxe", bootOrder.Elements[0].Value)
		}
	})

	t.Run("parse property links", func(t *testing.T) {
		resource, err := parser.Parse("/redfish/v1/Systems/1", system1)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		links := resource.Properties["Links"]
		if links == nil {
			t.Fatal("Missing Links property")
		}

		chassis := links.Children["Chassis"]
		if chassis == nil {
			t.Fatal("Missing Links.Chassis")
		}
		if chassis.Type != PropertyArray {
			t.Errorf("Chassis type = %v, want PropertyArray", chassis.Type)
		}

		chassis0 := chassis.Elements[0]
		if chassis0.Type != PropertyLink {
			t.Errorf("Chassis[0] type = %v, want PropertyLink", chassis0.Type)
		}
		if chassis0.LinkTarget != "/redfish/v1/Chassis/1" {
			t.Errorf("Chassis[0] target = %q, want %q", chassis0.LinkTarget, "/redfish/v1/Chassis/1")
		}
	})
}

// mockCache implements a simple in-memory cache for testing
type mockCache struct {
	resources map[string]*Resource
	parser    *Parser
}

func newMockCache() *mockCache {
	return &mockCache{
		resources: make(map[string]*Resource),
		parser:    NewParser(),
	}
}

func (m *mockCache) loadJSON(path string, data []byte) error {
	resource, err := m.parser.Parse(path, data)
	if err != nil {
		return err
	}
	m.resources[path] = resource
	return nil
}

func (m *mockCache) Get(path string) (*Resource, error) {
	if res, ok := m.resources[path]; ok {
		return res, nil
	}
	return nil, &NotFoundError{Path: path}
}

func (m *mockCache) GetKnownPaths() []string {
	paths := make([]string, 0, len(m.resources))
	for p := range m.resources {
		paths = append(paths, p)
	}
	return paths
}

func (m *mockCache) Clear() {
	m.resources = make(map[string]*Resource)
}

func (m *mockCache) Save() error {
	return nil
}

// TestVFS_PathResolution tests path resolution
func TestVFS_PathResolution(t *testing.T) {
	cache := newMockCache()
	cache.loadJSON("/redfish/v1", serviceRoot)
	cache.loadJSON("/redfish/v1/Systems", systemsCollection)
	cache.loadJSON("/redfish/v1/Systems/1", system1)

	vfs := &vfs{cache: cache}

	t.Run("absolute resource path", func(t *testing.T) {
		target, err := vfs.ResolveTarget("/redfish/v1", "/redfish/v1/Systems/1")
		if err != nil {
			t.Fatalf("ResolveTarget failed: %v", err)
		}

		if target.Type != TargetResource {
			t.Errorf("Type = %v, want TargetResource", target.Type)
		}
		if target.ResourcePath != "/redfish/v1/Systems/1" {
			t.Errorf("ResourcePath = %q, want %q", target.ResourcePath, "/redfish/v1/Systems/1")
		}
	})

	t.Run("relative resource path", func(t *testing.T) {
		target, err := vfs.ResolveTarget("/redfish/v1", "Systems/1")
		if err != nil {
			t.Fatalf("ResolveTarget failed: %v", err)
		}

		if target.Type != TargetResource {
			t.Errorf("Type = %v, want TargetResource", target.Type)
		}
		if target.ResourcePath != "/redfish/v1/Systems/1" {
			t.Errorf("ResourcePath = %q, want %q", target.ResourcePath, "/redfish/v1/Systems/1")
		}
	})

	t.Run("simple property access", func(t *testing.T) {
		target, err := vfs.ResolveTarget("/redfish/v1/Systems/1", "Status/Health")
		if err != nil {
			t.Fatalf("ResolveTarget failed: %v", err)
		}

		if target.Type != TargetProperty {
			t.Errorf("Type = %v, want TargetProperty", target.Type)
		}
		if target.Property.Type != PropertySimple {
			t.Errorf("Property type = %v, want PropertySimple", target.Property.Type)
		}
		if target.Property.Value != "OK" {
			t.Errorf("Property value = %v, want OK", target.Property.Value)
		}
	})

	t.Run("array indexing", func(t *testing.T) {
		target, err := vfs.ResolveTarget("/redfish/v1/Systems/1", "Boot/BootOrder[0]")
		if err != nil {
			t.Fatalf("ResolveTarget failed: %v", err)
		}

		if target.Type != TargetProperty {
			t.Errorf("Type = %v, want TargetProperty", target.Type)
		}
		if target.Property.Value != "Pxe" {
			t.Errorf("Property value = %v, want Pxe", target.Property.Value)
		}
	})

	t.Run("composite path - resource then property", func(t *testing.T) {
		target, err := vfs.ResolveTarget("/redfish/v1", "Systems/1/Status/Health")
		if err != nil {
			t.Fatalf("ResolveTarget failed: %v", err)
		}

		if target.Type != TargetProperty {
			t.Errorf("Type = %v, want TargetProperty", target.Type)
		}
		if target.Property.Value != "OK" {
			t.Errorf("Property value = %v, want OK", target.Property.Value)
		}
	})

	t.Run("property link", func(t *testing.T) {
		target, err := vfs.ResolveTarget("/redfish/v1/Systems/1", "Links/Chassis[0]")
		if err != nil {
			t.Fatalf("ResolveTarget failed: %v", err)
		}

		if target.Type != TargetLink {
			t.Errorf("Type = %v, want TargetLink", target.Type)
		}
		if target.ResourcePath != "/redfish/v1/Chassis/1" {
			t.Errorf("ResourcePath = %q, want %q", target.ResourcePath, "/redfish/v1/Chassis/1")
		}
	})

	t.Run("nested property access", func(t *testing.T) {
		target, err := vfs.ResolveTarget("/redfish/v1/Systems/1", "Status/State")
		if err != nil {
			t.Fatalf("ResolveTarget failed: %v", err)
		}

		if target.Property.Value != "Enabled" {
			t.Errorf("Property value = %v, want Enabled", target.Property.Value)
		}
	})

	t.Run("empty path returns current resource", func(t *testing.T) {
		target, err := vfs.ResolveTarget("/redfish/v1/Systems/1", "")
		if err != nil {
			t.Fatalf("ResolveTarget failed: %v", err)
		}

		if target.Type != TargetResource {
			t.Errorf("Type = %v, want TargetResource", target.Type)
		}
		if target.ResourcePath != "/redfish/v1/Systems/1" {
			t.Errorf("ResourcePath = %q, want %q", target.ResourcePath, "/redfish/v1/Systems/1")
		}
	})

	t.Run("collection member by name", func(t *testing.T) {
		target, err := vfs.ResolveTarget("/redfish/v1/Systems", "1")
		if err != nil {
			t.Fatalf("ResolveTarget failed: %v", err)
		}

		if target.Type != TargetResource {
			t.Errorf("Type = %v, want TargetResource", target.Type)
		}
		if target.ResourcePath != "/redfish/v1/Systems/1" {
			t.Errorf("ResourcePath = %q, want %q", target.ResourcePath, "/redfish/v1/Systems/1")
		}
	})
}

// TestVFS_ListOperations tests list operations
func TestVFS_ListOperations(t *testing.T) {
	cache := newMockCache()
	cache.loadJSON("/redfish/v1/Systems/1", system1)

	vfs := &vfs{cache: cache}

	t.Run("ListAll", func(t *testing.T) {
		entries, err := vfs.ListAll("/redfish/v1/Systems/1")
		if err != nil {
			t.Fatalf("ListAll failed: %v", err)
		}

		if len(entries) == 0 {
			t.Error("Expected entries, got none")
		}

		// Check for specific entries
		found := make(map[string]bool)
		for _, e := range entries {
			found[e.Name] = true
		}

		if !found["Status"] {
			t.Error("Missing Status property")
		}
		if !found["Boot"] {
			t.Error("Missing Boot property")
		}
	})

	t.Run("ListProperties", func(t *testing.T) {
		props, err := vfs.ListProperties("/redfish/v1/Systems/1")
		if err != nil {
			t.Fatalf("ListProperties failed: %v", err)
		}

		if len(props) == 0 {
			t.Error("Expected properties, got none")
		}

		found := false
		for _, p := range props {
			if p.Name == "Status" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Status property not found")
		}
	})
}

// TestVFS_PathUtilities tests path utility functions
func TestVFS_PathUtilities(t *testing.T) {
	cache := newMockCache()
	vfs := &vfs{cache: cache}

	t.Run("Join", func(t *testing.T) {
		tests := []struct {
			base, target, want string
		}{
			{"/redfish/v1", "Systems", "/redfish/v1/Systems"},
			{"/redfish/v1/Systems", "1", "/redfish/v1/Systems/1"},
			{"/redfish/v1/", "Systems/", "/redfish/v1/Systems"},
		}

		for _, tt := range tests {
			got := vfs.Join(tt.base, tt.target)
			if got != tt.want {
				t.Errorf("Join(%q, %q) = %q, want %q", tt.base, tt.target, got, tt.want)
			}
		}
	})

	t.Run("Parent", func(t *testing.T) {
		tests := []struct {
			path, want string
		}{
			{"/redfish/v1/Systems/1", "/redfish/v1/Systems"},
			{"/redfish/v1/Systems", "/redfish/v1"},
			{"/redfish/v1", "/redfish/v1"},
		}

		for _, tt := range tests {
			got := vfs.Parent(tt.path)
			if got != tt.want {
				t.Errorf("Parent(%q) = %q, want %q", tt.path, got, tt.want)
			}
		}
	})
}
