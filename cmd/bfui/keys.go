package main

import "github.com/charmbracelet/bubbles/key"

// NormalKeyMap defines key bindings for normal browsing mode
type NormalKeyMap struct {
	Up         key.Binding
	Down       key.Binding
	Collapse   key.Binding
	Expand     key.Binding
	Toggle     key.Binding
	Enter      key.Binding
	Back       key.Binding
	GoUp       key.Binding
	Home       key.Binding
	Refresh    key.Binding
	Scrape     key.Binding
	ScrollDown key.Binding
	ScrollUp   key.Binding
	Search     key.Binding
	Action     key.Binding
	Help       key.Binding
	Quit       key.Binding
}

var normalKeys = NormalKeyMap{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("k/↑", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j/↓", "down"),
	),
	Collapse: key.NewBinding(
		key.WithKeys("h", "left"),
		key.WithHelp("h/←", "collapse"),
	),
	Expand: key.NewBinding(
		key.WithKeys("l", "right"),
		key.WithHelp("l/→", "expand"),
	),
	Toggle: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "open / rebase"),
	),
	Back: key.NewBinding(
		key.WithKeys("backspace"),
		key.WithHelp("backspace", "back"),
	),
	GoUp: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "parent"),
	),
	Home: key.NewBinding(
		key.WithKeys("~"),
		key.WithHelp("~", "root"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	Scrape: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "scrape"),
	),
	ScrollDown: key.NewBinding(
		key.WithKeys("J"),
		key.WithHelp("J", "scroll details ↓"),
	),
	ScrollUp: key.NewBinding(
		key.WithKeys("K"),
		key.WithHelp("K", "scroll details ↑"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Action: key.NewBinding(
		key.WithKeys("!"),
		key.WithHelp("!", "actions"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

// SearchKeyMap defines key bindings for search overlay mode
type SearchKeyMap struct {
	Confirm  key.Binding
	Cancel   key.Binding
	NextItem key.Binding
	PrevItem key.Binding
}

var searchKeys = SearchKeyMap{
	Confirm: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "go"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),
	NextItem: key.NewBinding(
		key.WithKeys("ctrl+j", "down"),
		key.WithHelp("ctrl+j/↓", "next"),
	),
	PrevItem: key.NewBinding(
		key.WithKeys("ctrl+k", "up"),
		key.WithHelp("ctrl+k/↑", "prev"),
	),
}

// ActionKeyMap defines key bindings for action overlay mode
type ActionKeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Confirm key.Binding
	Cancel  key.Binding
	Tab     key.Binding
	Yes     key.Binding
	No      key.Binding
}

var actionKeys = ActionKeyMap{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("k/↑", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j/↓", "down"),
	),
	Confirm: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "cycle values"),
	),
	Yes: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "confirm POST"),
	),
	No: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "cancel"),
	),
}

// OverlayKeyMap defines the shared dismiss binding for help/scrape modals
type OverlayKeyMap struct {
	Cancel key.Binding
}

var overlayKeys = OverlayKeyMap{
	Cancel: key.NewBinding(
		key.WithKeys("esc", "?"),
		key.WithHelp("esc/?", "close"),
	),
}
