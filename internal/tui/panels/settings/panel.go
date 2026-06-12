// Package settings implements the right-panel Settings tab (tab 4).
//
// Mirrors src/tui/panels/settings-tab.ts (MVP subset: slippage tolerance +
// network selection; the LazySwapPass row is gated off by FEATURES.buyPass and
// the API-routing row by FEATURES.swap.apiMode, both disabled here).
//
// The panel never mutates global state directly: committing the slippage editor
// emits SlippageChangedMsg and selecting Network emits NetworkChangeMsg, leaving
// the parent screen to apply the change (and re-dial RPC services on a switch).
package settings

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/theme"
)

// SlippageChangedMsg is emitted when the user commits a new slippage value.
type SlippageChangedMsg struct{ Value float64 }

// NetworkChangeMsg is emitted when the user selects the Network row; ChainKey
// is the next chain in the cycle. The parent applies the switch.
type NetworkChangeMsg struct{ ChainKey string }

// ShowWalletMsg is emitted when the user selects the Wallet row. The parent
// owns the current wallet, so it presents the address + QR overlay.
type ShowWalletMsg struct{}

// row indexes the selectable options.
type row int

const (
	rowSlippage row = iota
	rowNetwork
	rowWallet
	rowCount
)

// Model owns the settings-tab state.
type Model struct {
	slippage  float64
	chainKey  string
	chainName string

	cursor  row
	editing bool
	input   textinput.Model

	focused       bool
	width, height int
}

// New builds the panel seeded with the active slippage + chain.
func New(slippage float64, chainKey, chainName string) Model {
	ti := textinput.New()
	ti.Placeholder = "e.g. 0.5"
	ti.Prompt = ""
	ti.CharLimit = 5
	ti.Width = 8

	return Model{
		slippage:  slippage,
		chainKey:  chainKey,
		chainName: chainName,
		input:     ti,
	}
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// SetSize stores the outer dimensions.
func (m *Model) SetSize(w, h int) { m.width, m.height = w, h }

// SetFocused toggles the focused border color.
func (m *Model) SetFocused(b bool) { m.focused = b }

// SetSlippage updates the displayed slippage (after the parent applies it).
func (m *Model) SetSlippage(v float64) { m.slippage = v }

// SetNetwork updates the displayed chain (after the parent applies a switch).
func (m *Model) SetNetwork(key, name string) {
	m.chainKey, m.chainName = key, name
}

// Capturing reports whether the slippage editor is currently active.
func (m Model) Capturing() bool { return m.editing }

// Update advances the panel state and emits intent messages to the parent.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.editing {
		switch k.Type {
		case tea.KeyEnter:
			m.editing = false
			m.input.Blur()
			v, err := strconv.ParseFloat(strings.TrimSpace(m.input.Value()), 64)
			if err != nil || v < 0 || v > 100 {
				return m, nil // reject invalid; keep the prior value
			}
			m.slippage = v
			return m, func() tea.Msg { return SlippageChangedMsg{Value: v} }
		case tea.KeyEsc:
			m.editing = false
			m.input.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch k.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < rowCount-1 {
			m.cursor++
		}
	case "enter":
		switch m.cursor {
		case rowSlippage:
			m.editing = true
			m.input.SetValue(strconv.FormatFloat(m.slippage, 'f', -1, 64))
			m.input.CursorEnd()
			m.input.Focus()
			return m, textinput.Blink
		case rowNetwork:
			next := chain.NextKey(m.chainKey)
			return m, func() tea.Msg { return NetworkChangeMsg{ChainKey: next} }
		case rowWallet:
			return m, func() tea.Msg { return ShowWalletMsg{} }
		}
	}
	return m, nil
}

// View renders the bordered settings list.
func (m Model) View() string {
	rows := []string{
		m.renderRow(rowSlippage, "Slippage tolerance",
			strconv.FormatFloat(m.slippage, 'f', -1, 64)+"%  —  Enter to edit"),
		m.renderRow(rowNetwork, "Network",
			m.chainName+"  —  Enter to switch"),
		m.renderRow(rowWallet, "Wallet",
			"Address & QR  —  Enter to view"),
	}
	if m.editing {
		editor := theme.Border(true).Padding(0, 1).Render(m.input.View())
		rows = append(rows, "", editor)
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, rows...)
	w, h := m.width-2, m.height-2
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	titled := lipgloss.JoinVertical(lipgloss.Left,
		theme.Text().Bold(true).Padding(0, 1).Render("Settings"),
		inner,
	)
	return theme.Border(m.focused).Width(w).Height(h).Render(titled)
}

func (m Model) renderRow(r row, name, desc string) string {
	nameStyle := theme.Dim()
	descStyle := theme.Dim()
	prefix := "  "
	if r == m.cursor {
		nameStyle = theme.Text().Bold(true).Background(theme.YellowSel)
		descStyle = theme.Dim().Background(theme.YellowSel)
		prefix = "> "
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		nameStyle.Render(prefix+name),
		descStyle.Render("  "+desc),
	)
}
