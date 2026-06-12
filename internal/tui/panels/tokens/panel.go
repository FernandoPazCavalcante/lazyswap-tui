// Package tokens implements the token-balance list shown on the right side.
//
// Mirrors src/tui/panels/tokens-tab.ts. State machine: idle → loading →
// loaded | error. Parent feeds balances via SetBalances; refresh requests
// are signaled by the parent (typically pressing 'r' or selecting a wallet).
package tokens

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/balance"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/theme"
)

// State enumerates the visible UI states.
type State int

const (
	StateIdle State = iota
	StateLoading
	StateLoaded
	StateError
)

// Item wraps a TokenBalance for bubbles/list.
type Item struct{ B balance.TokenBalance }

func (i Item) FilterValue() string { return i.B.Symbol }
func (i Item) Title() string {
	usd := i.B.USDValue
	if usd == "" {
		usd = "—"
	}
	return fmt.Sprintf("%-8s %14s   %s", i.B.Symbol, i.B.Balance, usd)
}
func (i Item) Description() string { return i.B.Address }

// Model owns the tokens panel state.
type Model struct {
	list     list.Model
	state    State
	errMsg   string
	focused  bool
	width    int
	height   int
	subtitle string // e.g. "0xabc…1234 · BSC"
}

func New() Model {
	l := list.New(nil, theme.ListDelegate{}, 60, 10)
	l.Title = "Tokens"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Padding(0, 1)

	return Model{list: l, state: StateIdle}
}

func (m Model) Init() tea.Cmd { return nil }

// SetSize accepts outer dimensions (border + content). The rendered box will
// occupy exactly w × h cells.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	inner := h - 4 // border (2) + title (1) + subtitle (1)
	if inner < 1 {
		inner = 1
	}
	innerW := w - 2
	if innerW < 1 {
		innerW = 1
	}
	m.list.SetSize(innerW, inner)
}

func (m *Model) SetFocused(b bool) { m.focused = b }

func (m *Model) SetSubtitle(s string) { m.subtitle = s }

// SetLoading flips the panel to loading state and clears any prior content.
func (m *Model) SetLoading() {
	m.state = StateLoading
	m.errMsg = ""
	m.list.SetItems(nil)
}

// SetBalances replaces the list contents and switches to loaded state.
func (m *Model) SetBalances(balances []balance.TokenBalance) {
	items := make([]list.Item, len(balances))
	for i, b := range balances {
		items[i] = Item{B: b}
	}
	m.list.SetItems(items)
	m.state = StateLoaded
	m.errMsg = ""
}

// SetError switches to error state with the given message.
func (m *Model) SetError(err string) {
	m.state = StateError
	m.errMsg = err
	m.list.SetItems(nil)
}

// Selected returns the highlighted token, or nil if the list is empty.
func (m Model) Selected() *balance.TokenBalance {
	it, ok := m.list.SelectedItem().(Item)
	if !ok {
		return nil
	}
	return &it.B
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	var body string
	switch m.state {
	case StateLoading:
		body = theme.Dim().Render("  Loading balances...")
	case StateError:
		body = theme.Error().Render("  " + m.errMsg)
	case StateIdle:
		body = theme.Dim().Render("  Select a wallet to view balances.")
	case StateLoaded:
		if len(m.list.Items()) == 0 {
			body = theme.Dim().Render("  No tokens.")
		} else {
			body = m.list.View()
		}
	}

	subtitle := ""
	if m.subtitle != "" {
		subtitle = theme.Dim().Render("  " + m.subtitle)
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, subtitle, body)
	w, h := m.width-2, m.height-2
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return theme.Border(m.focused).Width(w).Height(h).Render(inner)
}
