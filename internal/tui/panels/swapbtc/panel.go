// Package swapbtc implements the right-panel "Swap BTC" tab (tab 5): a
// cross-chain EVM → BTC swap form routed through THORchain.
//
// Mirrors src/tui/panels/swap-tab.ts. The form has three sub-fields — source
// token (list), USD amount, destination BTC address. Submitting emits a
// QuoteRequestMsg; the parent runs Flow.GetThorchainQuote and feeds back a
// QuoteResultMsg. A live 10s countdown re-quotes while the preview is shown.
// Confirming the preview emits ExecuteRequestMsg.
package swapbtc

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/balance"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/swap"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/theme"
)

// refreshSeconds is the quote-refresh countdown while the preview is shown.
const refreshSeconds = 10

// minBTCAddrLen is the shortest plausible Bitcoin address (matches swap-tab.ts).
const minBTCAddrLen = 25

// ─── Messages ──────────────────────────────────────────────────────────────────

// QuoteRequestMsg asks the parent for a THORchain quote (panel → parent).
type QuoteRequestMsg struct {
	From       swap.TokenInfo
	USDAmount  string
	BTCAddress string
}

// ExecuteRequestMsg asks the parent to broadcast the swap (panel → parent).
type ExecuteRequestMsg struct {
	From       swap.TokenInfo
	USDAmount  string
	BTCAddress string
}

// EstimateRequestMsg asks the parent for a destination-less price preview
// (panel → parent), fired when the user confirms a USD amount.
type EstimateRequestMsg struct {
	From      swap.TokenInfo
	USDAmount string
}

// EstimateResultMsg carries a price-preview estimate back (parent → panel).
type EstimateResultMsg struct {
	Quote swap.FlowQuote
	Err   error
}

// QuoteResultMsg carries a quote back to the panel (parent → panel).
type QuoteResultMsg struct {
	Quote swap.FlowQuote
	Err   error
}

// ExecutionResultMsg carries an execution result back (parent → panel).
type ExecutionResultMsg struct {
	Result swap.FlowResult
}

// TickMsg drives the preview re-quote countdown.
type TickMsg struct{}

// ─── State ───────────────────────────────────────────────────────────────────

type state int

const (
	stateForm state = iota
	stateQuoting
	statePreview
	stateExecuting
	stateDone
)

type field int

const (
	fieldToken field = iota
	fieldAmount
	fieldAddr
)

// Item wraps an owned token balance for the source list.
type Item struct {
	Symbol   string
	Address  string
	Decimals uint8
	Balance  string
	USDValue string
}

func (i Item) FilterValue() string { return i.Symbol }
func (i Item) Title() string {
	usd := i.USDValue
	if usd == "" {
		usd = "—"
	}
	return fmt.Sprintf("%-8s %14s   %s", i.Symbol, i.Balance, usd)
}
func (i Item) Description() string { return "" }

func (i Item) toTokenInfo() swap.TokenInfo {
	return swap.TokenInfo{Symbol: i.Symbol, Address: i.Address, Decimals: i.Decimals}
}

// Model owns the Swap BTC tab state.
type Model struct {
	tokens list.Model
	amount textinput.Model
	addr   textinput.Model

	state state
	field field

	from      swap.TokenInfo
	quote     *swap.FlowQuote
	quoteErr  string
	countdown int
	execRes   *swap.FlowResult

	// Price preview shown under the amount field (destination-less estimate).
	estimate    *swap.FlowQuote
	estimateErr string
	estimating  bool

	focused       bool
	width, height int
}

// New builds an empty panel. SetBalances seeds the source token list.
func New() Model {
	amt := textinput.New()
	amt.Placeholder = "e.g. 50"
	amt.Prompt = "$ "
	amt.CharLimit = 16
	amt.Width = 20

	addr := textinput.New()
	addr.Placeholder = "bc1q… or 1… or 3…"
	addr.Prompt = ""
	addr.CharLimit = 62
	addr.Width = 44

	return Model{
		tokens: newList("Swap to BTC — choose source"),
		amount: amt,
		addr:   addr,
		state:  stateForm,
		field:  fieldToken,
	}
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// SetSize lays out the inner widgets within the outer dimensions.
func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	listW := w - 4
	if listW < 10 {
		listW = 10
	}
	// Reserve rows for the chrome the form always draws around the list:
	// border (2) + title + 2 blanks + help + amount label/input + estimate
	// line + addr label/input + an optional error line. Keeps it within height.
	listH := h - 14
	if listH < 3 {
		listH = 3
	}
	m.tokens.SetSize(listW, listH)
	m.amount.Width = clampInt(w-8, 10, 24)
	m.addr.Width = clampInt(w-8, 12, 48)
}

// SetFocused toggles the focused border color.
func (m *Model) SetFocused(b bool) { m.focused = b }

// SetBalances rebuilds the source token list from the wallet's balances.
func (m *Model) SetBalances(balances []balance.TokenBalance) {
	items := make([]list.Item, 0, len(balances))
	for _, b := range balances {
		items = append(items, Item{
			Symbol:   b.Symbol,
			Address:  b.Address,
			Decimals: b.Decimals,
			Balance:  b.Balance,
			USDValue: b.USDValue,
		})
	}
	m.tokens.SetItems(items)
}

// Capturing reports whether a text field is currently focused (so digits and
// letters are typed rather than treated as global shortcuts).
func (m Model) Capturing() bool {
	return m.state == stateForm && (m.field == fieldAmount || m.field == fieldAddr)
}

// Update advances the form / preview state machine.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {

	case EstimateResultMsg:
		m.estimating = false
		if msg.Err != nil {
			m.estimate = nil
			m.estimateErr = msg.Err.Error()
			return m, nil
		}
		q := msg.Quote
		m.estimate = &q
		m.estimateErr = ""
		return m, nil

	case QuoteResultMsg:
		if msg.Err != nil {
			m.quote = nil
			m.quoteErr = msg.Err.Error()
			m.state = stateForm
			m.field = fieldAddr
			m.addr.Focus()
			return m, textinput.Blink
		}
		q := msg.Quote
		m.quote = &q
		m.quoteErr = ""
		m.state = statePreview
		m.countdown = refreshSeconds
		return m, tick()

	case ExecutionResultMsg:
		r := msg.Result
		m.execRes = &r
		m.state = stateDone
		return m, nil

	case TickMsg:
		if m.state != statePreview {
			return m, nil
		}
		m.countdown--
		if m.countdown <= 0 {
			m.state = stateQuoting
			return m, m.requestQuote()
		}
		return m, tick()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyMsg) (Model, tea.Cmd) {
	if k.Type == tea.KeyEsc {
		return m.handleEsc()
	}

	switch m.state {
	case stateForm:
		return m.handleFormKey(k)

	case statePreview:
		switch strings.ToLower(k.String()) {
		case "y", "enter":
			if m.quoteErr != "" || m.quote == nil {
				return m, nil
			}
			m.state = stateExecuting
			return m, m.requestExecute()
		case "n":
			m.state = stateForm
			m.field = fieldAddr
			m.addr.Focus()
			return m, textinput.Blink
		}

	case stateExecuting:
		// Block input while the tx is in flight.

	case stateDone:
		// Any key resets the form for another swap.
		m.reset()
		return m, nil
	}
	return m, nil
}

func (m Model) handleEsc() (Model, tea.Cmd) {
	switch m.state {
	case stateForm:
		if m.field != fieldToken {
			m.amount.Blur()
			m.addr.Blur()
			m.field = fieldToken
		}
	case statePreview, stateDone:
		m.reset()
	}
	return m, nil
}

func (m Model) handleFormKey(k tea.KeyMsg) (Model, tea.Cmd) {
	switch m.field {
	case fieldToken:
		if k.Type == tea.KeyEnter {
			if _, ok := m.tokens.SelectedItem().(Item); ok {
				m.field = fieldAmount
				m.amount.Focus()
				return m, textinput.Blink
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.tokens, cmd = m.tokens.Update(k)
		return m, cmd

	case fieldAmount:
		if k.Type == tea.KeyEnter {
			m.amount.Blur()
			m.field = fieldAddr
			m.addr.Focus()
			// Fire a destination-less estimate so the BTC/sats preview shows
			// before the user types an address.
			return m, tea.Batch(textinput.Blink, m.requestEstimate())
		}
		// A keystroke changes the amount — drop the now-stale estimate.
		m.estimate = nil
		m.estimateErr = ""
		var cmd tea.Cmd
		m.amount, cmd = m.amount.Update(k)
		return m, cmd

	case fieldAddr:
		if k.Type == tea.KeyEnter {
			if !m.inputsValid() {
				m.quoteErr = "Enter a positive USD amount and a valid BTC address."
				return m, nil
			}
			m.addr.Blur()
			m.state = stateQuoting
			m.quoteErr = ""
			return m, m.requestQuote()
		}
		var cmd tea.Cmd
		m.addr, cmd = m.addr.Update(k)
		return m, cmd
	}
	return m, nil
}

// ─── intent helpers ────────────────────────────────────────────────────────────

func (m *Model) requestQuote() tea.Cmd {
	it, ok := m.tokens.SelectedItem().(Item)
	if !ok {
		return nil
	}
	m.from = it.toTokenInfo()
	from := m.from
	usd := strings.TrimSpace(m.amount.Value())
	btc := strings.TrimSpace(m.addr.Value())
	return func() tea.Msg {
		return QuoteRequestMsg{From: from, USDAmount: usd, BTCAddress: btc}
	}
}

func (m *Model) requestEstimate() tea.Cmd {
	it, ok := m.tokens.SelectedItem().(Item)
	if !ok {
		return nil
	}
	usd := strings.TrimSpace(m.amount.Value())
	if v, err := strconv.ParseFloat(usd, 64); err != nil || v <= 0 {
		return nil
	}
	m.from = it.toTokenInfo()
	from := m.from
	m.estimating = true
	m.estimateErr = ""
	return func() tea.Msg {
		return EstimateRequestMsg{From: from, USDAmount: usd}
	}
}

func (m *Model) requestExecute() tea.Cmd {
	usd := strings.TrimSpace(m.amount.Value())
	btc := strings.TrimSpace(m.addr.Value())
	from := m.from
	return func() tea.Msg {
		return ExecuteRequestMsg{From: from, USDAmount: usd, BTCAddress: btc}
	}
}

func (m Model) inputsValid() bool {
	usd, err := strconv.ParseFloat(strings.TrimSpace(m.amount.Value()), 64)
	if err != nil || usd <= 0 {
		return false
	}
	return len(strings.TrimSpace(m.addr.Value())) >= minBTCAddrLen
}

func (m *Model) reset() {
	m.state = stateForm
	m.field = fieldToken
	m.quote = nil
	m.quoteErr = ""
	m.execRes = nil
	m.countdown = 0
	m.estimate = nil
	m.estimateErr = ""
	m.estimating = false
	m.amount.Blur()
	m.addr.Blur()
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return TickMsg{} })
}

// ─── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	body := m.bodyForState()
	help := theme.Dim().Render(m.helpForState())
	titled := lipgloss.JoinVertical(lipgloss.Left,
		theme.Text().Bold(true).Padding(0, 1).Render("Swap BTC (via THORchain)"),
		"",
		body,
		"",
		help,
	)
	w, h := m.width-2, m.height-2
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	box := theme.Border(m.focused).Width(w).Height(h).Render(titled)
	// Hard-cap to the allotted footprint so a tall form can never overflow and
	// push the tab header / wallet panel off-screen.
	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(box)
}

func (m Model) bodyForState() string {
	switch m.state {
	case stateForm:
		return m.formBody()
	case stateQuoting:
		return theme.Dim().Render("  Estimating via THORchain…")
	case statePreview:
		return m.previewBody()
	case stateExecuting:
		return theme.Text().Render("  Submitting transaction…")
	case stateDone:
		if m.execRes != nil {
			return swap.FormatSwapResult(*m.execRes)
		}
	}
	return ""
}

func (m Model) formBody() string {
	amountLabel := "  Amount (USD $):"
	addrLabel := "  Destination BTC address:"
	if m.field == fieldAmount {
		amountLabel = theme.Text().Bold(true).Render("> Amount (USD $):")
	}
	if m.field == fieldAddr {
		addrLabel = theme.Text().Bold(true).Render("> Destination BTC address:")
	}
	rows := []string{
		m.tokens.View(),
		"",
		amountLabel,
		"  " + m.amount.View(),
		m.estimateLine(),
		addrLabel,
		"  " + m.addr.View(),
	}
	if m.quoteErr != "" {
		rows = append(rows, "", theme.Error().Render("  "+m.quoteErr))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// estimateLine renders the live BTC/sats price preview under the amount field.
// Always returns one row so the form height stays stable.
func (m Model) estimateLine() string {
	switch {
	case m.estimating:
		return theme.Dim().Render("  estimating…")
	case m.estimateErr != "":
		return theme.Dim().Render("  (estimate unavailable)")
	case m.estimate != nil:
		return theme.Text().Render(fmt.Sprintf("  ≈ %s BTC  (%s sats)",
			m.estimate.EstimatedOutput, formatSats(m.estimate.EstimatedOutputSats)))
	default:
		return ""
	}
}

func (m Model) previewBody() string {
	if m.quoteErr != "" {
		return theme.Error().Render("  Quote failed: " + m.quoteErr)
	}
	if m.quote == nil {
		return theme.Dim().Render("  Loading quote…")
	}
	q := m.quote
	eta := "~10 min"
	if q.ThorEstimatedSeconds > 0 {
		eta = fmt.Sprintf("~%d min", (q.ThorEstimatedSeconds+59)/60)
	}
	lines := []string{
		fmt.Sprintf("  %s ≈ %s %s %s", q.USDAmountFormatted, q.FromTokenAmount, q.FromToken.Symbol, q.FromTokenPriceLine),
		fmt.Sprintf("  Fee (%.2f%%)    %s %s", q.FeePercent, q.FeeAmount, q.FromToken.Symbol),
		fmt.Sprintf("  %s %s → %s BTC", q.NetFromTokenAmount, q.FromToken.Symbol, q.EstimatedOutput),
		fmt.Sprintf("  Min output: %s BTC (3%% THORchain slippage)", q.MinOutput),
		fmt.Sprintf("  Fees: %s BTC  |  ETA: %s", q.ThorFees, eta),
	}
	if q.NeedsApproval {
		lines = append(lines, theme.Dim().Render("  ⚠ Token approval required (auto-handled on execute)"))
	}
	lines = append(lines,
		fmt.Sprintf("  Destination: %s", q.BTCAddress),
		theme.Dim().Render(fmt.Sprintf("  Price refreshes in %ds", m.countdown)),
	)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m Model) helpForState() string {
	switch m.state {
	case stateForm:
		switch m.field {
		case fieldToken:
			return "↑/↓ select token  ·  enter: next  ·  esc: back"
		default:
			return "type value  ·  enter: next/quote  ·  esc: back"
		}
	case statePreview:
		return "y/enter: execute  ·  n/esc: edit"
	case stateExecuting:
		return "waiting for receipt…"
	case stateDone:
		return "any key to start another"
	}
	return ""
}

// ─── list / numeric helpers ─────────────────────────────────────────────────────

func newList(title string) list.Model {
	l := list.New(nil, theme.ListDelegate{}, 40, 6)
	l.Title = title
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Padding(0, 1)
	return l
}

// formatSats renders a sats amount with thousands separators, e.g. 72145 →
// "72,145".
func formatSats(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
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
