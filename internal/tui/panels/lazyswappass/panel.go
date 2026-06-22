// Package lazyswappass implements the right-panel "Lazyswap Pass" tab (tab 6).
//
// It shows the selected wallet's pass status and a single "Buy Lazyswap Pass"
// action. The panel holds no business logic: pressing Enter when a pass can be
// bought emits BuyRequestMsg and the parent screen runs the on-chain mint, then
// feeds results back via SetStatus / SetBuying / SetError.
package lazyswappass

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/FernandoPazCavalcante/lazyswap/internal/pass"
	"github.com/FernandoPazCavalcante/lazyswap/internal/tui/theme"
)

// BuyRequestMsg is emitted when the user triggers a purchase. The parent owns
// the wallet key + RPC, so it performs the mint.
type BuyRequestMsg struct{}

// Model owns the pass-tab state. All inputs are pushed by the parent.
type Model struct {
	available bool   // chain has a deployed pass
	native    string // native gas-token symbol (e.g. tBNB) for the price label
	loaded    bool   // a status has been fetched at least once
	status    pass.Status
	buying    bool
	errMsg    string

	focused       bool
	width, height int
}

// New builds an empty pass panel (parent populates it on activation).
func New() Model { return Model{} }

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// SetSize stores the outer dimensions.
func (m *Model) SetSize(w, h int) { m.width, m.height = w, h }

// SetFocused toggles the focused border + button highlight.
func (m *Model) SetFocused(b bool) { m.focused = b }

// SetAvailable records whether the active chain has a pass contract, plus the
// native symbol used in the price label. Availability only changes on a chain
// switch (or at startup), so any prior status is now stale — reset it and let
// the parent re-fetch.
func (m *Model) SetAvailable(available bool, native string) {
	m.available = available
	m.native = native
	m.loaded = false
	m.status = pass.Status{}
	m.errMsg = ""
	m.buying = false
}

// SetStatus records a freshly fetched pass status (clears buying + error).
func (m *Model) SetStatus(s pass.Status) {
	m.status = s
	m.loaded = true
	m.buying = false
	m.errMsg = ""
}

// SetBuying marks a mint as in-flight (clears any prior error).
func (m *Model) SetBuying(b bool) {
	m.buying = b
	if b {
		m.errMsg = ""
	}
}

// SetError records a failed mint/refresh and stops the buying spinner.
func (m *Model) SetError(s string) {
	m.errMsg = s
	m.buying = false
}

// canBuy reports whether a purchase action is currently valid. Requires a
// loaded status so a keyboard Enter before the first fetch can't mint blind
// (consistent with View, which only shows the button once loaded).
func (m Model) canBuy() bool {
	return m.available && m.loaded && !m.buying && !m.status.HasValidPass
}

// Update emits BuyRequestMsg when the user presses Enter and a buy is valid.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if k.String() == "enter" && m.canBuy() {
		m.buying = true
		m.errMsg = ""
		return m, func() tea.Msg { return BuyRequestMsg{} }
	}
	return m, nil
}

// View renders the bordered pass panel.
func (m Model) View() string {
	var body []string

	switch {
	case !m.available:
		body = append(body,
			theme.Dim().Render("Not available on this network."),
			theme.Dim().Render("Switch to BSC Testnet to buy a Lazyswap Pass."),
		)
	case m.buying:
		body = append(body,
			theme.Text().Render("Minting pass…"),
			theme.Dim().Render("Waiting for on-chain confirmation."),
		)
	case m.status.HasValidPass:
		body = append(body,
			theme.Text().Bold(true).Render("✓ Pass active"),
			theme.Text().Render("Expires: "+m.status.ExpiresAt.Format("Jan 2, 2006")),
			theme.Dim().Render(remaining(m.status.ExpiresAt)),
		)
	default:
		if m.loaded {
			body = append(body, theme.Dim().Render("No active pass."))
		}
		body = append(body, "", m.buyButton())
	}

	if m.errMsg != "" {
		body = append(body, "", theme.Error().Render("Error: "+m.errMsg))
	}

	titled := lipgloss.JoinVertical(lipgloss.Left,
		theme.Text().Bold(true).Padding(0, 1).Render("Lazyswap Pass"),
		lipgloss.JoinVertical(lipgloss.Left, body...),
	)

	w, h := m.width-2, m.height-2
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return theme.Border(m.focused).Width(w).Height(h).Render(titled)
}

func (m Model) buyButton() string {
	label := fmt.Sprintf(" Buy Lazyswap Pass — 0.01 %s ", m.native)
	style := theme.Dim()
	if m.focused {
		style = theme.Text().Bold(true).Background(theme.YellowSel)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		style.Render("["+label+"]"),
		theme.Dim().Render("  Enter to buy · valid 1 year"),
	)
}

// remaining renders a human "~N days left" line for a future expiry.
func remaining(exp time.Time) string {
	d := time.Until(exp)
	if d <= 0 {
		return ""
	}
	return fmt.Sprintf("~%d days left", int(d.Hours()/24))
}
