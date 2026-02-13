package rvfs

import (
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
)

const RedfishRoot = "/redfish/v1"

// VFS provides a virtual filesystem view of Redfish resources
type VFS interface {
	// Core operations
	Get(path string) (*Resource, error)
	ResolveTarget(basePath, targetPath string) (*Target, error)

	// Directory-like operations
	ListAll(path string) ([]*Entry, error)
	ListProperties(path string) ([]*Property, error)

	// Path utilities
	Join(base, target string) string
	Parent(path string) string

	// Cache management
	GetKnownPaths() []string
	Clear()
	Sync() error
}

// cache interface for dependency injection
type cache interface {
	Get(path string) (*Resource, error)
	GetKnownPaths() []string
	Clear()
	Save() error
}

// vfs implements VFS interface
type vfs struct {
	cache cache
}

// NewVFS creates a new VFS instance
func NewVFS(endpoint, username, password string, insecure bool) (VFS, error) {
	client, err := NewClient(endpoint, username, password, insecure)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(endpoint)
	cacheFile := fmt.Sprintf(".rfsh_cache_%s.json", u.Hostname())

	parser := NewParser()
	cache := NewResourceCache(client, parser, cacheFile)

	return &vfs{cache: cache}, nil
}

// Get retrieves a resource
func (v *vfs) Get(path string) (*Resource, error) {
	// If path contains : or [, it's a composite path - resolve it
	if strings.Contains(path, ":") || strings.Contains(path, "[") {
		target, err := v.ResolveTarget(RedfishRoot, path)
		if err != nil {
			return nil, err
		}
		return v.cache.Get(target.ResourcePath)
	}
	return v.cache.Get(path)
}

// ResolveTarget resolves a target path from a base path
// Handles:
// - Absolute paths: /redfish/v1/Systems/1
// - Resource navigation: Systems/1
// - Property access: Status:Health
// - Array indexing: Members[0]
// - Composite paths: Systems/1/Status:Health
func (v *vfs) ResolveTarget(basePath, targetPath string) (*Target, error) {
	// If basePath is composite (contains : or [), resolve it first
	var baseResourcePath string
	if strings.Contains(basePath, ":") || strings.Contains(basePath, "[") {
		baseTarget, err := v.ResolveTarget(RedfishRoot, basePath)
		if err != nil {
			return nil, err
		}
		baseResourcePath = baseTarget.ResourcePath
	} else {
		baseResourcePath = basePath
	}

	// Empty target = current resource
	if targetPath == "" {
		res, err := v.cache.Get(baseResourcePath)
		if err != nil {
			return nil, err
		}
		return &Target{
			Type:         TargetResource,
			Resource:     res,
			ResourcePath: baseResourcePath,
		}, nil
	}

	// Absolute path - resolve from root
	if strings.HasPrefix(targetPath, "/") {
		return v.resolveAbsolute(targetPath)
	}

	// Relative path - resolve from base resource
	return v.resolveRelative(baseResourcePath, targetPath)
}

// resolveAbsolute resolves an absolute path like /redfish/v1/Systems/1/Status:Health
func (v *vfs) resolveAbsolute(path string) (*Target, error) {
	// Strip /redfish/v1 prefix
	if !strings.HasPrefix(path, RedfishRoot) {
		return nil, fmt.Errorf("invalid absolute path: %s", path)
	}

	if path == RedfishRoot {
		res, err := v.cache.Get(RedfishRoot)
		if err != nil {
			return nil, err
		}
		return &Target{
			Type:         TargetResource,
			Resource:     res,
			ResourcePath: RedfishRoot,
		}, nil
	}

	relativePath := strings.TrimPrefix(path, RedfishRoot+"/")
	return v.resolveRelative(RedfishRoot, relativePath)
}

// resolveRelative resolves a path relative to a base resource
func (v *vfs) resolveRelative(basePath, targetPath string) (*Target, error) {
	// Split by / for resource navigation
	segments := strings.Split(targetPath, "/")

	currentPath := basePath
	var currentResource *Resource
	var err error

	for i, seg := range segments {
		// Check if segment contains : (property access)
		if strings.Contains(seg, ":") {
			// Everything from here is property navigation
			propertyPath := strings.Join(segments[i:], "/")
			return v.resolveProperty(currentPath, propertyPath)
		}

		// Resource navigation - fetch resource if needed
		if currentResource == nil {
			currentResource, err = v.cache.Get(currentPath)
			if err != nil {
				return nil, err
			}
		}

		// Check if segment is a child resource
		if child, ok := currentResource.Children[seg]; ok {
			currentPath = child.Target
			currentResource = nil // Need to fetch next
			continue
		}

		// Check if it's a property (no : but might have [n])
		if prop, err := v.navigatePropertySegment(currentResource.Properties, seg); err == nil {
			// Found a property - everything from here is property space
			if i < len(segments)-1 {
				// More segments after - invalid (can't navigate past a property without :)
				return nil, fmt.Errorf("cannot navigate into property %s without :", seg)
			}

			if prop.Type == PropertyLink {
				return &Target{
					Type:         TargetLink,
					Resource:     currentResource,
					Property:     prop,
					ResourcePath: prop.LinkTarget,
				}, nil
			}

			return &Target{
				Type:     TargetProperty,
				Resource: currentResource,
				Property: prop,
			}, nil
		}

		// Not found
		return nil, &NotFoundError{Path: seg}
	}

	// Landed on a resource
	if currentResource == nil {
		currentResource, err = v.cache.Get(currentPath)
		if err != nil {
			return nil, err
		}
	}

	return &Target{
		Type:         TargetResource,
		Resource:     currentResource,
		ResourcePath: currentPath,
	}, nil
}

// resolveProperty navigates a property path like Status:Health or Boot:BootOrder[0]
func (v *vfs) resolveProperty(resourcePath, propertyPath string) (*Target, error) {
	resource, err := v.cache.Get(resourcePath)
	if err != nil {
		return nil, err
	}

	// Split by : for nested property access
	segments := strings.Split(propertyPath, ":")
	currentProps := resource.Properties
	var currentProp *Property

	for i, seg := range segments {
		prop, err := v.navigatePropertySegment(currentProps, seg)
		if err != nil {
			return nil, err
		}
		currentProp = prop

		// If this is a link and we have more segments, follow it
		if currentProp.Type == PropertyLink && i < len(segments)-1 {
			resource, err = v.cache.Get(currentProp.LinkTarget)
			if err != nil {
				return nil, err
			}
			currentProps = resource.Properties
			continue
		}

		// Prepare for next segment
		if i < len(segments)-1 {
			if currentProp.Type != PropertyObject {
				return nil, fmt.Errorf("cannot navigate into property of type %d", currentProp.Type)
			}
			currentProps = currentProp.Children
		}
	}

	if currentProp.Type == PropertyLink {
		return &Target{
			Type:         TargetLink,
			Resource:     resource,
			Property:     currentProp,
			ResourcePath: currentProp.LinkTarget,
		}, nil
	}

	return &Target{
		Type:     TargetProperty,
		Resource: resource,
		Property: currentProp,
	}, nil
}

// navigatePropertySegment handles a single property segment with optional array indexing
func (v *vfs) navigatePropertySegment(properties map[string]*Property, segment string) (*Property, error) {
	// Check for array indexing: PropertyName[n]
	if idx := strings.Index(segment, "["); idx != -1 {
		if !strings.HasSuffix(segment, "]") {
			return nil, &NotFoundError{Path: segment}
		}

		propName := segment[:idx]
		indexStr := segment[idx+1 : len(segment)-1]

		prop, ok := properties[propName]
		if !ok {
			return nil, &NotFoundError{Path: propName}
		}

		if prop.Type != PropertyArray {
			return nil, fmt.Errorf("%s is not an array", propName)
		}

		index := 0
		fmt.Sscanf(indexStr, "%d", &index)

		if index >= len(prop.Elements) {
			return nil, fmt.Errorf("index %d out of bounds", index)
		}

		return prop.Elements[index], nil
	}

	// Simple property lookup
	prop, ok := properties[segment]
	if !ok {
		return nil, &NotFoundError{Path: segment}
	}

	return prop, nil
}

// ListAll returns all entries (children and properties) at a path
func (v *vfs) ListAll(path string) ([]*Entry, error) {
	var resource *Resource
	var err error

	// If path contains : or [, it's a property path - resolve it
	if strings.Contains(path, ":") || strings.Contains(path, "[") {
		target, err := v.ResolveTarget(RedfishRoot, path)
		if err != nil {
			return nil, err
		}
		resource, err = v.cache.Get(target.ResourcePath)
		if err != nil {
			return nil, err
		}
	} else {
		// Simple resource path - fetch directly
		resource, err = v.cache.Get(path)
		if err != nil {
			return nil, err
		}
	}

	entries := make([]*Entry, 0, len(resource.Children)+len(resource.Properties))

	// Add children
	for _, child := range resource.Children {
		entryType := EntryLink
		if child.Type == ChildSymlink {
			entryType = EntrySymlink
		}
		entries = append(entries, &Entry{
			Name:     child.Name,
			Path:     child.Target,
			Type:     entryType,
			Modified: resource.FetchedAt,
		})
	}

	// Add properties
	for _, prop := range resource.Properties {
		entryType := EntryProperty
		if prop.Type != PropertySimple {
			entryType = EntryComplex
		}
		entries = append(entries, &Entry{
			Name:     prop.Name,
			Path:     path + "." + prop.Name,
			Type:     entryType,
			Size:     int64(len(prop.RawJSON)),
			Modified: resource.FetchedAt,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// ListProperties returns properties at a path
func (v *vfs) ListProperties(path string) ([]*Property, error) {
	var resource *Resource
	var err error

	// If path contains : or [, it's a property path - resolve it
	if strings.Contains(path, ":") || strings.Contains(path, "[") {
		target, err := v.ResolveTarget(RedfishRoot, path)
		if err != nil {
			return nil, err
		}
		resource, err = v.cache.Get(target.ResourcePath)
		if err != nil {
			return nil, err
		}
	} else {
		// Simple resource path - fetch directly
		resource, err = v.cache.Get(path)
		if err != nil {
			return nil, err
		}
	}

	properties := make([]*Property, 0, len(resource.Properties))
	for _, prop := range resource.Properties {
		properties = append(properties, prop)
	}

	sort.Slice(properties, func(i, j int) bool {
		return properties[i].Name < properties[j].Name
	})

	return properties, nil
}

// Join joins path segments
func (v *vfs) Join(base, target string) string {
	return normalizePath(path.Join(base, target))
}

// Parent returns the parent path
func (v *vfs) Parent(p string) string {
	p = normalizePath(p)
	if p == RedfishRoot || p == "/" {
		return p
	}
	return path.Dir(p)
}

// GetKnownPaths returns all cached paths
func (v *vfs) GetKnownPaths() []string {
	return v.cache.GetKnownPaths()
}

// Clear removes all cached resources
func (v *vfs) Clear() {
	v.cache.Clear()
}

// Sync saves cache to disk
func (v *vfs) Sync() error {
	return v.cache.Save()
}

