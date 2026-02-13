package main

import "github.com/bluefish-project/bluefish/rvfs"

// ResourceLoadedMsg is sent when an async resource fetch completes
type ResourceLoadedMsg struct {
	Path     string
	Resource *rvfs.Resource
	Err      error
}

// ActionsDiscoveredMsg is sent when action discovery completes
type ActionsDiscoveredMsg struct {
	Path    string
	Actions []ActionInfo
	Err     error
}

// ActionResultMsg is sent when a POST action completes
type ActionResultMsg struct {
	StatusCode int
	Body       string
	Err        error
}

// NodeSelectedMsg is sent when the cursor moves to a new tree item
type NodeSelectedMsg struct {
	Item TreeItem
}
