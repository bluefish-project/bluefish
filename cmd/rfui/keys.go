package main

import "github.com/charmbracelet/bubbles/key"

// NormalKeyMap defines key bindings for normal browsing mode
type NormalKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Collapse key.Binding
	Expand   key.Binding
	Toggle   key.Binding
	Enter    key.Binding
	Subtree  key.Binding
	Back     key.Binding
	BackAlt  key.Binding
	GoUp     key.Binding
	Home     key.Binding
	Refresh  key.Binding
	ScrollDown key.Binding
	ScrollUp   key.Binding
	Search   key.Binding
	Action   key.Binding
	Quit     key.Binding
}

var normalKeys = NormalKeyMap{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j", "down"),
	),
	Collapse: key.NewBinding(
		key.WithKeys("h", "left"),
		key.WithHelp("h", "collapse"),
	),
	Expand: key.NewBinding(
		key.WithKeys("l", "right"),
		key.WithHelp("l", "expand"),
	),
	Toggle: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "follow"),
	),
	Subtree: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "subtree"),
	),
	Back: key.NewBinding(
		key.WithKeys("b"),
		key.WithHelp("b", "back"),
	),
	BackAlt: key.NewBinding(
		key.WithKeys("backspace"),
		key.WithHelp("bs", "back"),
	),
	GoUp: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "up"),
	),
	Home: key.NewBinding(
		key.WithKeys("~"),
		key.WithHelp("~", "home"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	ScrollDown: key.NewBinding(
		key.WithKeys("J"),
		key.WithHelp("J", "scroll down"),
	),
	ScrollUp: key.NewBinding(
		key.WithKeys("K"),
		key.WithHelp("K", "scroll up"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Action: key.NewBinding(
		key.WithKeys("!"),
		key.WithHelp("!", "action"),
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
		key.WithHelp("ctrl+j", "next"),
	),
	PrevItem: key.NewBinding(
		key.WithKeys("ctrl+k", "up"),
		key.WithHelp("ctrl+k", "prev"),
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
		key.WithHelp("k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j", "down"),
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
		key.WithHelp("tab", "cycle"),
	),
	Yes: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "confirm"),
	),
	No: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "cancel"),
	),
}
