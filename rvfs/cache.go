package rvfs

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"sync"
)

// ResourceCache manages resources with transparent fetch-on-miss
type ResourceCache struct {
	client  *Client
	parser  *Parser
	store   map[string]*Resource
	file    string
	offline bool
	mu      sync.RWMutex
}

// cacheEntry represents a serialized resource for persistence
type cacheEntry struct {
	Path      string `json:"path"`
	ODataID   string `json:"odataId"`
	ODataType string `json:"odataType"`
	FetchedAt string `json:"fetchedAt"`
	Data      string `json:"data"` // Base64 encoded raw JSON
}

// NewResourceCache creates a cache with auto-fetch capability
func NewResourceCache(client *Client, parser *Parser, cacheFile string) *ResourceCache {
	cache := &ResourceCache{
		client: client,
		parser: parser,
		store:  make(map[string]*Resource),
		file:   cacheFile,
	}

	// Try to load existing cache
	cache.Load()

	return cache
}

// NewOfflineCache creates a cache from disk only (offline mode)
func NewOfflineCache(cacheFile string) (*ResourceCache, error) {
	cache := &ResourceCache{
		parser:  NewParser(),
		store:   make(map[string]*Resource),
		file:    cacheFile,
		offline: true,
	}

	if err := cache.Load(); err != nil {
		return nil, err
	}

	return cache, nil
}

// Get retrieves a resource, fetching if necessary
func (c *ResourceCache) Get(path string) (*Resource, error) {
	path = normalizePath(path)

	// Check cache
	c.mu.RLock()
	if resource, ok := c.store[path]; ok {
		c.mu.RUnlock()
		return resource, nil
	}
	c.mu.RUnlock()

	// Not cached - check if offline
	if c.offline {
		return nil, &NotCachedError{Path: path}
	}

	// Fetch from server
	data, err := c.client.Fetch(path)
	if err != nil {
		return nil, err
	}

	// Parse into resource
	resource, err := c.parser.Parse(path, data)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.mu.Lock()
	c.store[path] = resource
	c.mu.Unlock()

	return resource, nil
}

// Put stores a resource in cache
func (c *ResourceCache) Put(resource *Resource) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[resource.Path] = resource
}

// GetKnownPaths returns all cached paths
func (c *ResourceCache) GetKnownPaths() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	paths := make([]string, 0, len(c.store))
	for path := range c.store {
		paths = append(paths, path)
	}
	return paths
}

// Invalidate removes a resource from cache
func (c *ResourceCache) Invalidate(path string) {
	path = normalizePath(path)

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.store, path)
}

// Clear removes all cached resources
func (c *ResourceCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.store = make(map[string]*Resource)
}

// Size returns the number of cached resources
func (c *ResourceCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.store)
}

// Save persists cache to disk
func (c *ResourceCache) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.file == "" {
		return nil
	}

	// Convert to cache entries
	entries := make(map[string]cacheEntry)
	for path, resource := range c.store {
		entries[path] = cacheEntry{
			Path:      resource.Path,
			ODataID:   resource.ODataID,
			ODataType: resource.ODataType,
			FetchedAt: resource.FetchedAt.Format("2006-01-02T15:04:05Z07:00"),
			Data:      base64.StdEncoding.EncodeToString(resource.RawJSON),
		}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.file, data, 0644)
}

// Load restores cache from disk
func (c *ResourceCache) Load() error {
	if c.file == "" {
		return nil
	}

	data, err := os.ReadFile(c.file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No cache file yet
		}
		return err
	}

	var entries map[string]cacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	// Convert to resources
	parser := c.parser
	if parser == nil {
		parser = NewParser()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range entries {
		// Decode raw JSON
		rawJSON, err := base64.StdEncoding.DecodeString(entry.Data)
		if err != nil {
			continue // Skip corrupted entries
		}

		// Re-parse to get full structure
		resource, err := parser.Parse(entry.Path, rawJSON)
		if err != nil {
			continue // Skip unparseable entries
		}

		c.store[entry.Path] = resource
	}

	return nil
}

// IsOffline returns true if cache is in offline mode
func (c *ResourceCache) IsOffline() bool {
	return c.offline
}

// SetOffline sets offline mode
func (c *ResourceCache) SetOffline(offline bool) {
	c.offline = offline
}
