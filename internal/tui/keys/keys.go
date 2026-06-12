// Package keys defines the global key bindings used by the TUI.
package keys

import "github.com/charmbracelet/bubbles/key"

type Set struct {
	Quit    key.Binding
	Enter   key.Binding
	Escape  key.Binding
	Tab     key.Binding
	NextTab key.Binding
	PrevTab key.Binding
	Swap    key.Binding
	Refresh key.Binding
}

// Default returns the canonical key bindings.
func Default() Set {
	return Set{
		Quit:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
		Enter:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
		Escape:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Tab:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
		NextTab: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next tab")),
		PrevTab: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev tab")),
		Swap:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "swap")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
}
