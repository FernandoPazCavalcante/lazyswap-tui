package theme

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SelectedRow is the shared style for the highlighted row in any list/menu:
// a bold yellow "> " marker + text on the YellowSel background. This is the
// single source of truth for selection highlighting (Settings, Wallets,
// Tokens, Swap BTC all use it) so every selected option looks identical.
func SelectedRow() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(Yellow).Background(YellowSel).Bold(true)
}

// NormalRow is the style for an unselected row (dim, no background).
func NormalRow() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(YellowDim)
}

// ListDelegate is a single-line bubbles/list delegate that renders the selected
// item with SelectedRow ("> Title") and others with NormalRow ("  Title").
type ListDelegate struct{}

func (ListDelegate) Height() int                         { return 1 }
func (ListDelegate) Spacing() int                        { return 0 }
func (ListDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (ListDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	title := item.FilterValue()
	if di, ok := item.(list.DefaultItem); ok {
		title = di.Title()
	}
	if index == m.Index() {
		fmt.Fprint(w, SelectedRow().Render("> "+title))
		return
	}
	fmt.Fprint(w, NormalRow().Render("  "+title))
}
