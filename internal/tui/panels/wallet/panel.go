// Package wallet implements the left-side wallet list panel.
//
// Mirrors src/tui/panels/wallet-panel.ts: a scrollable list of wallet
// addresses with keyboard navigation. Selection events surface as
// SelectionChangedMsg so the parent can refresh dependent panels.
package wallet

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/theme"
	walletpkg "github.com/FernandoPazCavalcante/lazyswap-tui/internal/wallet"
)

// SelectionChangedMsg fires when the highlighted wallet changes.
type SelectionChangedMsg struct {
	Wallet *walletpkg.Wallet
}

// Item is the list.Item wrapper for a wallet.
type Item struct {
	W walletpkg.Wallet
}

func (i Item) FilterValue() string { return i.W.Address }
func (i Item) Title() string       { return shortAddr(i.W.Address) }
func (i Item) Description() string { return i.W.ID }

// Model owns the wallet panel state.
type Model struct {
	list    list.Model
	focused bool
	width   int
	height  int
}

// New constructs an empty panel. SetWallets seeds the list.
func New() Model {
	l := list.New(nil, theme.ListDelegate{}, 28, 10)
	l.Title = "Wallets"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Padding(0, 1)

	return Model{list: l}
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// SetSize lays out the panel within the given outer dimensions (the rendered
// box, including its border, will occupy exactly width × height cells).
func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
	// Account for border (2) + title (1).
	inner := height - 3
	if inner < 1 {
		inner = 1
	}
	innerW := width - 2
	if innerW < 1 {
		innerW = 1
	}
	m.list.SetSize(innerW, inner)
}

// SetFocused toggles the focused border color.
func (m *Model) SetFocused(b bool) { m.focused = b }

// SetWallets replaces the list contents while trying to preserve selection.
func (m *Model) SetWallets(ws []walletpkg.Wallet) {
	items := make([]list.Item, len(ws))
	for i, w := range ws {
		items[i] = Item{W: w}
	}
	m.list.SetItems(items)
}

// Selected returns the currently highlighted wallet, or nil if empty.
func (m Model) Selected() *walletpkg.Wallet {
	it, ok := m.list.SelectedItem().(Item)
	if !ok {
		return nil
	}
	return &it.W
}

// Update routes key/mouse messages into the underlying list.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	prev := m.Selected()
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	cur := m.Selected()

	var more tea.Cmd
	if !sameWallet(prev, cur) {
		more = func() tea.Msg { return SelectionChangedMsg{Wallet: cur} }
	}
	return m, tea.Batch(cmd, more)
}

// View renders the bordered panel. Width/Height set the *content* area; the
// border adds 2 to each axis, so we subtract 2 here to honour the outer
// footprint stored in m.width / m.height.
func (m Model) View() string {
	w, h := m.width-2, m.height-2
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	border := theme.Border(m.focused).Width(w).Height(h)
	return border.Render(m.list.View())
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func shortAddr(addr string) string {
	if len(addr) <= 12 {
		return addr
	}
	return fmt.Sprintf("%s…%s", addr[:6], addr[len(addr)-4:])
}

func sameWallet(a, b *walletpkg.Wallet) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.ID == b.ID
	}
}
