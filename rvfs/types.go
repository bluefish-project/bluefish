package rvfs

import (
	"fmt"
	"time"
)

// EntryType represents the type of VFS entry
type EntryType int

const (
	EntryResource EntryType = iota // Directory (Redfish resource)
	EntryProperty                  // File (simple property)
	EntryComplex                   // Directory (object property - navigable with /)
	EntryArray                     // Directory (array property - navigable with [n])
	EntryLink                      // Directory (child resource link)
	EntrySymlink                   // Symlink (external resource reference)
)

// Entry represents any item in the VFS
type Entry struct {
	Name     string
	Path     string
	Type     EntryType
	Size     int64
	Modified time.Time
}

// IsDir returns true if entry is navigable
func (e Entry) IsDir() bool {
	return e.Type == EntryResource || e.Type == EntryLink || e.Type == EntryComplex || e.Type == EntryArray || e.Type == EntrySymlink
}

// Resource represents a Redfish resource at a specific path
type Resource struct {
	Path       string
	ODataID    string
	ODataType  string
	RawJSON    []byte
	Properties map[string]*Property
	Children   map[string]*Child
	FetchedAt  time.Time
}

// GetProperty retrieves a property by name
func (r *Resource) GetProperty(name string) (*Property, error) {
	if prop, ok := r.Properties[name]; ok {
		return prop, nil
	}
	return nil, &NotFoundError{Path: r.Path + "/" + name}
}

// GetChild retrieves a child by name
func (r *Resource) GetChild(name string) (*Child, error) {
	if child, ok := r.Children[name]; ok {
		return child, nil
	}
	return nil, &NotFoundError{Path: r.Path + "/" + name}
}

// PropertyType represents the type of a property
type PropertyType int

const (
	PropertySimple PropertyType = iota // string, number, bool, null
	PropertyObject                     // JSON object
	PropertyArray                      // JSON array
	PropertyLink                       // Navigation reference ({"@odata.id": "..."})
)

// Property represents a data field in a resource (recursive tree structure)
type Property struct {
	Name string
	Type PropertyType

	// For PropertySimple
	Value any // Go value (string, float64, bool, nil)

	// For PropertyLink
	LinkTarget string // The @odata.id URL

	// For PropertyObject
	Children map[string]*Property // Nested fields

	// For PropertyArray
	Elements []*Property // Array items

	// Always present
	RawJSON []byte // Original JSON for this property
}

// ChildType represents the type of child resource
type ChildType int

const (
	ChildLink    ChildType = iota // Child resource (target under parent)
	ChildSymlink                  // External reference (target outside parent)
)

// Child represents a navigable child resource
type Child struct {
	Name   string
	Type   ChildType
	Target string // @odata.id path
	Parent string // Parent resource path
}

// IsExternal returns true if this links outside parent tree
func (c *Child) IsExternal() bool {
	return c.Type == ChildSymlink
}

// TargetType represents what a path resolves to
type TargetType int

const (
	TargetResource TargetType = iota // A Redfish resource (has @odata.id)
	TargetProperty                   // A non-link property (simple, object, array)
	TargetLink                       // A PropertyLink (navigable property)
)

// Target represents the result of path resolution
type Target struct {
	Type         TargetType // What type of target this is
	Resource     *Resource  // The resource we're in
	Property     *Property  // If Property or Link type
	ResourcePath string     // For navigation (Resources and Links)
}

// Error types

// NotFoundError indicates a path doesn't exist
type NotFoundError struct {
	Path string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("not found: %s", e.Path)
}

// NotCachedError indicates a resource is not cached (offline mode)
type NotCachedError struct {
	Path string
}

func (e *NotCachedError) Error() string {
	return fmt.Sprintf("not cached (offline mode): %s", e.Path)
}

// NetworkError indicates a network communication failure
type NetworkError struct {
	Path string
	Err  error
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("network error: %s: %v", e.Path, e.Err)
}

// HTTPError indicates an HTTP error response
type HTTPError struct {
	Path       string
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Path)
}

// ParseError indicates a JSON parsing error
type ParseError struct {
	Path string
	Err  error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error: %s: %v", e.Path, e.Err)
}
