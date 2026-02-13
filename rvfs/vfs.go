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
	Post(path string, body []byte) ([]byte, int, error)
	ResolveTarget(basePath, targetPath string) (*Target, error)

	// Directory-like operations
	ListAll(path string) ([]*Entry, error)
	ListProperties(path string) ([]*Property, error)

	// Path utilities
	Join(base, target string) string
	Parent(path string) string

	// Cache management
	GetKnownPaths() []string
	Invalidate(path string)
	Clear()
	Sync() error
}

// cache interface for dependency injection
type cache interface {
	Get(path string) (*Resource, error)
	Post(path string, body []byte) ([]byte, int, error)
	GetKnownPaths() []string
	Invalidate(path string)
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
	cacheFile := fmt.Sprintf(".bfsh_cache_%s.json", u.Hostname())

	parser := NewParser()
	cache := NewResourceCache(client, parser, cacheFile)

	return &vfs{cache: cache}, nil
}

// Get retrieves a resource by its canonical path
func (v *vfs) Get(path string) (*Resource, error) {
	return v.cache.Get(path)
}

// Post sends a POST request (no caching for writes)
func (v *vfs) Post(path string, body []byte) ([]byte, int, error) {
	return v.cache.Post(path, body)
}

// ResolveTarget resolves a target path from a base path.
// All paths use / as the separator. Handles:
// - Absolute paths: /redfish/v1/Systems/1/Status/Health
// - Relative paths: Status/Health (joined with basePath)
// - Array indexing: BootOrder[0]
func (v *vfs) ResolveTarget(basePath, targetPath string) (*Target, error) {
	// Empty target = resolve basePath itself
	if targetPath == "" {
		return v.resolveAbsolute(basePath)
	}

	// Absolute path - resolve from root
	if strings.HasPrefix(targetPath, "/") {
		return v.resolveAbsolute(targetPath)
	}

	// Relative path - join with base and resolve from root
	return v.resolveAbsolute(basePath + "/" + targetPath)
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

// resolveRelative resolves a path relative to a base resource.
// All navigation uses / as the separator. For each segment:
//   - In resource mode: check Children first, then Properties
//   - In property mode: check property children
//   - PropertyLink + more segments: follow link, back to resource mode
//   - PropertyObject + more segments: descend into children
//   - [n] within a segment handles array indexing
func (v *vfs) resolveRelative(basePath, targetPath string) (*Target, error) {
	// Filter empty segments (from trailing or double slashes)
	allSegments := strings.Split(targetPath, "/")
	segments := allSegments[:0]
	for _, s := range allSegments {
		if s != "" {
			segments = append(segments, s)
		}
	}

	currentPath := basePath
	var currentResource *Resource
	var currentProps map[string]*Property // nil = resource mode, non-nil = property mode
	var err error

	for i, seg := range segments {
		// In resource mode, try children first
		if currentProps == nil {
			if currentResource == nil {
				currentResource, err = v.cache.Get(currentPath)
				if err != nil {
					return nil, err
				}
			}

			if child, ok := currentResource.Children[seg]; ok {
				currentPath = child.Target
				currentResource = nil
				continue
			}

			// Not a child — fall through to property lookup
			currentProps = currentResource.Properties
		}

		// Property lookup (works in both resource and property mode)
		prop, err := v.navigatePropertySegment(currentProps, seg)
		if err != nil {
			return nil, err
		}

		// Last segment — return result
		if i == len(segments)-1 {
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

		// More segments — continue navigation
		switch prop.Type {
		case PropertyLink:
			// Follow link, back to resource mode
			currentPath = prop.LinkTarget
			currentResource = nil
			currentProps = nil
		case PropertyObject:
			currentProps = prop.Children
		default:
			return nil, fmt.Errorf("cannot navigate into %s: not an object or link", seg)
		}
	}

	// Ended on a resource
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

// ListAll returns all entries (children and properties) at a resource path
func (v *vfs) ListAll(path string) ([]*Entry, error) {
	resource, err := v.cache.Get(path)
	if err != nil {
		return nil, err
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
		entryType := entryTypeForProperty(prop)
		entries = append(entries, &Entry{
			Name:     prop.Name,
			Path:     path + "/" + prop.Name,
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

// ListProperties returns properties at a resource path
func (v *vfs) ListProperties(path string) ([]*Property, error) {
	resource, err := v.cache.Get(path)
	if err != nil {
		return nil, err
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

// entryTypeForProperty returns the appropriate EntryType for a property
func entryTypeForProperty(prop *Property) EntryType {
	switch prop.Type {
	case PropertyObject:
		return EntryComplex
	case PropertyArray:
		return EntryArray
	case PropertyLink:
		return EntrySymlink
	default:
		return EntryProperty
	}
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

// Invalidate removes a single resource from cache, forcing re-fetch on next Get
func (v *vfs) Invalidate(path string) {
	v.cache.Invalidate(path)
}

// Clear removes all cached resources
func (v *vfs) Clear() {
	v.cache.Clear()
}

// Sync saves cache to disk
func (v *vfs) Sync() error {
	return v.cache.Save()
}
