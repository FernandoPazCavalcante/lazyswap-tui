// Package swapoverlay implements the modal swap flow.
//
// State machine: stepFrom → stepTo → stepAmount → stepPreview → stepExecuting
// → stepDone. ESC steps back; from stepFrom, ESC closes the overlay.
//
// Mirrors src/tui/panels/swap-panel.ts but greatly simplified: no THORchain,
// no custom-address entry (recommended + owned only). The parent screen owns
// the quoting / execution commands; this overlay only renders state and emits
// intent messages.
package swapoverlay

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/balance"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/swap"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/theme"
)

// ─── Public messages ─────────────────────────────────────────────────────────

// CancelMsg signals the parent to close the overlay.
type CancelMsg struct{}

// QuoteRequestMsg is emitted when the user finalises from/to/amount.
// The parent should run a Flow.Quote and send back QuoteResultMsg.
type QuoteRequestMsg struct {
	From      swap.TokenInfo
	To        swap.TokenInfo
	USDAmount string
}

// ExecuteRequestMsg is emitted when the user confirms the preview.
// The parent should run Flow.Execute and send back ExecutionResultMsg.
type ExecuteRequestMsg struct {
	From      swap.TokenInfo
	To        swap.TokenInfo
	USDAmount string
}

// ─── Input messages (sent by parent) ─────────────────────────────────────────

// QuoteResultMsg carries the result of a QuoteRequestMsg.
type QuoteResultMsg struct {
	Quote swap.FlowQuote
	Err   error
}

// ExecutionResultMsg carries the result of an ExecuteRequestMsg.
type ExecutionResultMsg struct {
	Result swap.FlowResult
}

// ─── State ───────────────────────────────────────────────────────────────────

type step int

const (
	stepFrom step = iota
	stepTo
	stepAmount
	stepPreview
	stepExecuting
	stepDone
)

// Item wraps a token row (either an owned balance or a recommended seed).
type Item struct {
	Symbol   string
	Address  string // NativeSentinel or 0x…
	Decimals uint8
	Balance  string // human, may be "0.00"
	USDValue string // may be ""
	Owned    bool
}

func (i Item) FilterValue() string { return i.Symbol }
func (i Item) Title() string {
	usd := i.USDValue
	if usd == "" {
		usd = "—"
	}
	return fmt.Sprintf("%-8s %14s   %s", i.Symbol, i.Balance, usd)
}
func (i Item) Description() string {
	if i.Owned {
		return ""
	}
	return "★ Recommended"
}

// toTokenInfo flattens an Item into the swap-layer struct.
func (i Item) toTokenInfo() swap.TokenInfo {
	return swap.TokenInfo{Symbol: i.Symbol, Address: i.Address, Decimals: i.Decimals}
}

// Model is the overlay state.
type Model struct {
	from, to      list.Model
	amount        textinput.Model
	step          step
	width, height int

	selectedFrom Item
	selectedTo   Item
	usdAmount    string

	quote    *swap.FlowQuote
	quoteErr string
	execMsg  string // status banner during stepExecuting
	execRes  *swap.FlowResult
}

// New builds the overlay. balances should be the current wallet's tokens;
// c provides the recommended-token list + native metadata for the to-side.
func New(balances []balance.TokenBalance, c chain.Config) Model {
	fromItems := buildFromItems(balances)
	toItems := buildToItems(balances, c)

	ti := textinput.New()
	ti.Placeholder = "USD amount (e.g. 50)"
	ti.Prompt = "$ "
	ti.CharLimit = 16
	ti.Width = 24

	return Model{
		from:   newList("Swap from", fromItems),
		to:     newList("Swap to", toItems),
		amount: ti,
		step:   stepFrom,
	}
}

// Init kicks off the text-input cursor when the amount step is reached.
func (m Model) Init() tea.Cmd { return nil }

// SetSize lays out the inner widgets.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	listW := clampInt(w-8, 30, 80)
	listH := clampInt(h-12, 6, 18)
	m.from.SetSize(listW, listH)
	m.to.SetSize(listW, listH)
	m.amount.Width = clampInt(w-12, 12, 32)
}

// Update advances the state machine.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {

	case QuoteResultMsg:
		if msg.Err != nil {
			m.quoteErr = msg.Err.Error()
			m.quote = nil
		} else {
			m.quote = &msg.Quote
			m.quoteErr = ""
		}
		m.step = stepPreview
		return m, nil

	case ExecutionResultMsg:
		r := msg.Result
		m.execRes = &r
		m.step = stepDone
		return m, nil

	case tea.KeyMsg:
		// Global ESC: step back, or cancel from the first step.
		if msg.Type == tea.KeyEsc {
			return m.stepBack()
		}

		switch m.step {
		case stepFrom:
			if msg.Type == tea.KeyEnter {
				if it, ok := m.from.SelectedItem().(Item); ok {
					m.selectedFrom = it
					m.step = stepTo
					return m, nil
				}
			}
			var cmd tea.Cmd
			m.from, cmd = m.from.Update(msg)
			return m, cmd

		case stepTo:
			if msg.Type == tea.KeyEnter {
				if it, ok := m.to.SelectedItem().(Item); ok {
					if strings.EqualFold(it.Address, m.selectedFrom.Address) {
						// Refuse same-token swap; keep cursor on the to list.
						return m, nil
					}
					m.selectedTo = it
					m.step = stepAmount
					m.amount.Focus()
					return m, textinput.Blink
				}
			}
			var cmd tea.Cmd
			m.to, cmd = m.to.Update(msg)
			return m, cmd

		case stepAmount:
			if msg.Type == tea.KeyEnter {
				v := strings.TrimSpace(m.amount.Value())
				if v == "" {
					return m, nil
				}
				m.usdAmount = v
				m.amount.Blur()
				return m, func() tea.Msg {
					return QuoteRequestMsg{
						From:      m.selectedFrom.toTokenInfo(),
						To:        m.selectedTo.toTokenInfo(),
						USDAmount: v,
					}
				}
			}
			var cmd tea.Cmd
			m.amount, cmd = m.amount.Update(msg)
			return m, cmd

		case stepPreview:
			switch strings.ToLower(msg.String()) {
			case "y", "enter":
				if m.quoteErr != "" {
					return m, nil
				}
				m.step = stepExecuting
				m.execMsg = "Submitting transaction…"
				return m, func() tea.Msg {
					return ExecuteRequestMsg{
						From:      m.selectedFrom.toTokenInfo(),
						To:        m.selectedTo.toTokenInfo(),
						USDAmount: m.usdAmount,
					}
				}
			case "n":
				return m.stepBack()
			}

		case stepExecuting:
			// Block all input while a tx is in flight; only ESC exits via stepBack.

		case stepDone:
			// Any key dismisses.
			return m, func() tea.Msg { return CancelMsg{} }
		}
	}
	return m, nil
}

// stepBack rewinds one step, or emits CancelMsg if we're already at stepFrom.
func (m Model) stepBack() (Model, tea.Cmd) {
	switch m.step {
	case stepFrom, stepExecuting:
		return m, func() tea.Msg { return CancelMsg{} }
	case stepTo:
		m.step = stepFrom
	case stepAmount:
		m.amount.Blur()
		m.step = stepTo
	case stepPreview:
		m.step = stepAmount
		m.amount.Focus()
		m.quote = nil
		m.quoteErr = ""
		return m, textinput.Blink
	case stepDone:
		return m, func() tea.Msg { return CancelMsg{} }
	}
	return m, nil
}

// View renders the overlay centred over the available area.
func (m Model) View() string {
	body := m.bodyForStep()
	help := theme.Dim().Render(m.helpForStep())

	box := theme.Border(true).
		Padding(1, 2).
		Render(lipgloss.JoinVertical(lipgloss.Left,
			theme.Text().Bold(true).Render(m.titleForStep()),
			"",
			body,
			"",
			help,
		))

	if m.width == 0 || m.height == 0 {
		return box
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) titleForStep() string {
	switch m.step {
	case stepFrom:
		return "Swap — choose source token"
	case stepTo:
		return fmt.Sprintf("Swap — choose destination (from %s)", m.selectedFrom.Symbol)
	case stepAmount:
		return fmt.Sprintf("Swap %s → %s — USD amount", m.selectedFrom.Symbol, m.selectedTo.Symbol)
	case stepPreview:
		return fmt.Sprintf("Swap %s → %s — preview", m.selectedFrom.Symbol, m.selectedTo.Symbol)
	case stepExecuting:
		return "Swap — executing"
	case stepDone:
		return "Swap — result"
	}
	return ""
}

func (m Model) bodyForStep() string {
	switch m.step {
	case stepFrom:
		return m.from.View()
	case stepTo:
		return m.to.View()
	case stepAmount:
		return m.amount.View()
	case stepPreview:
		return m.previewBody()
	case stepExecuting:
		return theme.Text().Render(m.execMsg)
	case stepDone:
		return m.doneBody()
	}
	return ""
}

func (m Model) previewBody() string {
	if m.quoteErr != "" {
		return theme.Error().Render("Quote failed: " + m.quoteErr)
	}
	if m.quote == nil {
		return theme.Dim().Render("Loading quote…")
	}
	q := m.quote
	lines := []string{
		fmt.Sprintf("USD            %s", q.USDAmountFormatted),
		fmt.Sprintf("From           %s %s   %s", q.FromTokenAmount, q.FromToken.Symbol, q.FromTokenPriceLine),
		fmt.Sprintf("Fee (%.2f%%)    %s %s", q.FeePercent, q.FeeAmount, q.FromToken.Symbol),
		fmt.Sprintf("Net swap input %s %s", q.NetFromTokenAmount, q.FromToken.Symbol),
		fmt.Sprintf("Estimated      %s %s", q.EstimatedOutput, q.ToToken.Symbol),
		fmt.Sprintf("Min received   %s %s   (slippage %.2f%%)", q.MinOutput, q.ToToken.Symbol, q.Slippage),
	}
	if q.NeedsApproval {
		lines = append(lines, theme.Dim().Render("Note: ERC-20 approval required — will be sent automatically."))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m Model) doneBody() string {
	if m.execRes == nil {
		return ""
	}
	return swap.FormatSwapResult(*m.execRes)
}

func (m Model) helpForStep() string {
	switch m.step {
	case stepFrom, stepTo:
		return "↑/↓ navigate  ·  enter: select  ·  esc: back"
	case stepAmount:
		return "enter: get quote  ·  esc: back"
	case stepPreview:
		return "y / enter: execute  ·  n / esc: back"
	case stepExecuting:
		return "waiting for receipt…"
	case stepDone:
		return "any key to close"
	}
	return ""
}

// ─── Item building ───────────────────────────────────────────────────────────

func buildFromItems(balances []balance.TokenBalance) []list.Item {
	out := make([]list.Item, 0, len(balances))
	for _, b := range balances {
		// Native uses the sentinel address; ERC-20s carry their on-chain addr.
		out = append(out, list.Item(Item{
			Symbol:   b.Symbol,
			Address:  b.Address,
			Decimals: b.Decimals,
			Balance:  b.Balance,
			USDValue: b.USDValue,
			Owned:    true,
		}))
	}
	return out
}

func buildToItems(balances []balance.TokenBalance, c chain.Config) []list.Item {
	// Index owned balances by lowercase address for fast lookup.
	owned := make(map[string]balance.TokenBalance, len(balances))
	for _, b := range balances {
		owned[strings.ToLower(b.Address)] = b
	}

	// Native first.
	out := []list.Item{
		Item{
			Symbol:   c.NativeSymbol,
			Address:  balance.NativeAddress,
			Decimals: c.NativeDecimals,
			Balance:  pickBalance(owned[balance.NativeAddress]),
			USDValue: owned[balance.NativeAddress].USDValue,
			Owned:    contains(owned, balance.NativeAddress),
		},
	}

	// Recommended tokens (filter out native sentinel collisions).
	seen := map[string]bool{strings.ToLower(balance.NativeAddress): true}
	for _, r := range c.RecommendedTokens {
		key := strings.ToLower(r.Address)
		if seen[key] {
			continue
		}
		seen[key] = true
		b, isOwned := owned[key]
		out = append(out, Item{
			Symbol:   r.Symbol,
			Address:  r.Address,
			Decimals: r.Decimals,
			Balance:  pickBalance(b),
			USDValue: b.USDValue,
			Owned:    isOwned,
		})
	}

	// Then any owned tokens not already listed.
	for _, b := range balances {
		key := strings.ToLower(b.Address)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Item{
			Symbol:   b.Symbol,
			Address:  b.Address,
			Decimals: b.Decimals,
			Balance:  b.Balance,
			USDValue: b.USDValue,
			Owned:    true,
		})
	}
	return out
}

func pickBalance(b balance.TokenBalance) string {
	if b.Balance == "" {
		return "0.00"
	}
	return b.Balance
}

func contains(owned map[string]balance.TokenBalance, addr string) bool {
	_, ok := owned[strings.ToLower(addr)]
	return ok
}

// ─── list helpers ────────────────────────────────────────────────────────────

func newList(title string, items []list.Item) list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(0)

	dim := lipgloss.NewStyle().Foreground(theme.YellowDim).Padding(0, 0, 0, 2)
	// Selected rows use the shared YellowSel highlight block (matches the
	// Settings / Wallets / Tokens selection style).
	sel := lipgloss.NewStyle().Foreground(theme.Yellow).Background(theme.YellowSel).Padding(0, 0, 0, 2).Bold(true)
	delegate.Styles.NormalTitle = dim
	delegate.Styles.SelectedTitle = sel
	delegate.Styles.NormalDesc = lipgloss.NewStyle().Foreground(theme.YellowDim).Padding(0, 0, 0, 2)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().Foreground(theme.YellowDim).Background(theme.YellowSel).Padding(0, 0, 0, 2)

	l := list.New(items, delegate, 60, 10)
	l.Title = title
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Padding(0, 1)
	return l
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
