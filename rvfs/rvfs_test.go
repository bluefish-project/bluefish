package rvfs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	},
	"Actions": {
		"#ComputerSystem.Reset": {
			"target": "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset",
			"@Redfish.ActionInfo": "/redfish/v1/Systems/1/ResetActionInfo",
			"ResetType@Redfish.AllowableValues": ["On", "ForceOff", "GracefulShutdown"]
		}
	},
	"BiosVersion": "2.1.0",
	"GraphicalConsole": {
		"ConnectTypesSupported": ["KVMIP"],
		"MaxConcurrentSessions": 4,
		"ServiceEnabled": true
	},
	"Assembly": {
		"@odata.id": "/redfish/v1/Systems/1/Assembly"
	},
	"LocationIndicatorActive": false,
	"FirmwareInventoryUri": "/redfish/v1/UpdateService/FirmwareInventory/BMC",
	"ImageURI": "https://example.com/bios.img"
}`)

// TestClient_Post tests the POST method
func TestClient_Post(t *testing.T) {
	var receivedBody []byte
	var receivedToken string
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == "POST" {
			w.Header().Set("X-Auth-Token", "test-token-123")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{}`))
			return
		}
		if r.Method == "POST" {
			receivedBody, _ = io.ReadAll(r.Body)
			receivedToken = r.Header.Get("X-Auth-Token")
			receivedContentType = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "done"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "admin", "pass", true)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"ResetType": "ForceOff"})
	data, status, err := client.Post("/redfish/v1/Systems/1/Actions/ComputerSystem.Reset", body)
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}

	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
	if receivedToken != "test-token-123" {
		t.Errorf("token = %q, want %q", receivedToken, "test-token-123")
	}
	if receivedContentType != "application/json" {
		t.Errorf("content-type = %q, want %q", receivedContentType, "application/json")
	}
	if string(receivedBody) != string(body) {
		t.Errorf("body = %q, want %q", string(receivedBody), string(body))
	}
	if len(data) == 0 {
		t.Error("expected response body, got empty")
	}
}

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

// TestParser_URIStringDetection tests that URI string properties are detected as PropertyLinks
func TestParser_URIStringDetection(t *testing.T) {
	parser := NewParser()
	resource, err := parser.Parse("/redfish/v1/Systems/1", system1)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	t.Run("FirmwareInventoryUri is PropertyLink", func(t *testing.T) {
		prop := resource.Properties["FirmwareInventoryUri"]
		if prop == nil {
			t.Fatal("Missing FirmwareInventoryUri property")
		}
		if prop.Type != PropertyLink {
			t.Errorf("Type = %v, want PropertyLink", prop.Type)
		}
		if prop.LinkTarget != "/redfish/v1/UpdateService/FirmwareInventory/BMC" {
			t.Errorf("LinkTarget = %q, want %q", prop.LinkTarget, "/redfish/v1/UpdateService/FirmwareInventory/BMC")
		}
	})

	t.Run("ImageURI with external URL stays PropertySimple", func(t *testing.T) {
		prop := resource.Properties["ImageURI"]
		if prop == nil {
			t.Fatal("Missing ImageURI property")
		}
		// External URLs (https://) are not Redfish paths, should remain simple
		if prop.Type != PropertySimple {
			t.Errorf("Type = %v, want PropertySimple (external URL)", prop.Type)
		}
	})

	t.Run("Actions target is PropertyLink", func(t *testing.T) {
		actions := resource.Properties["Actions"]
		if actions == nil {
			t.Fatal("Missing Actions property")
		}
		reset := actions.Children["#ComputerSystem.Reset"]
		if reset == nil {
			t.Fatal("Missing #ComputerSystem.Reset action")
		}

		target := reset.Children["target"]
		if target == nil {
			t.Fatal("Missing target property in action")
		}
		if target.Type != PropertyLink {
			t.Errorf("target Type = %v, want PropertyLink", target.Type)
		}
		if target.LinkTarget != "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset" {
			t.Errorf("target LinkTarget = %q", target.LinkTarget)
		}
	})

	t.Run("@Redfish.ActionInfo is PropertyLink", func(t *testing.T) {
		actions := resource.Properties["Actions"]
		reset := actions.Children["#ComputerSystem.Reset"]

		actionInfo := reset.Children["@Redfish.ActionInfo"]
		if actionInfo == nil {
			t.Fatal("Missing @Redfish.ActionInfo property")
		}
		if actionInfo.Type != PropertyLink {
			t.Errorf("@Redfish.ActionInfo Type = %v, want PropertyLink", actionInfo.Type)
		}
		if actionInfo.LinkTarget != "/redfish/v1/Systems/1/ResetActionInfo" {
			t.Errorf("@Redfish.ActionInfo LinkTarget = %q", actionInfo.LinkTarget)
		}
	})

	t.Run("regular strings stay PropertySimple", func(t *testing.T) {
		prop := resource.Properties["BiosVersion"]
		if prop == nil {
			t.Fatal("Missing BiosVersion property")
		}
		if prop.Type != PropertySimple {
			t.Errorf("BiosVersion Type = %v, want PropertySimple", prop.Type)
		}
		if prop.Value != "2.1.0" {
			t.Errorf("BiosVersion Value = %v, want 2.1.0", prop.Value)
		}
	})

	t.Run("AllowableValues annotation stays PropertyArray", func(t *testing.T) {
		actions := resource.Properties["Actions"]
		reset := actions.Children["#ComputerSystem.Reset"]

		allowable := reset.Children["ResetType@Redfish.AllowableValues"]
		if allowable == nil {
			t.Fatal("Missing ResetType@Redfish.AllowableValues")
		}
		if allowable.Type != PropertyArray {
			t.Errorf("AllowableValues Type = %v, want PropertyArray", allowable.Type)
		}
		if len(allowable.Elements) != 3 {
			t.Errorf("AllowableValues elements = %d, want 3", len(allowable.Elements))
		}
	})

	t.Run("Assembly is still a Child (link-only object)", func(t *testing.T) {
		// Assembly is {"@odata.id": "..."} — should be a Child, not a Property
		if _, ok := resource.Children["Assembly"]; !ok {
			t.Error("Assembly should be a Child (link-only object), not a Property")
		}
		if _, ok := resource.Properties["Assembly"]; ok {
			t.Error("Assembly should NOT be in Properties")
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

func (m *mockCache) Post(path string, body []byte) ([]byte, int, error) {
	return nil, 0, fmt.Errorf("post not supported in mock")
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
