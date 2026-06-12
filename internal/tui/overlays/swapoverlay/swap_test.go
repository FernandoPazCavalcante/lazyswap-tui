package swapoverlay

import (
	"errors"
	"math/big"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/balance"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/swap"
)

func sampleBalances() []balance.TokenBalance {
	return []balance.TokenBalance{
		{Symbol: "BNB", Address: balance.NativeAddress, Decimals: 18, Balance: "0.5", BalanceRaw: big.NewInt(500000000000000000), USDValue: "$300.00"},
		{Symbol: "USDT", Address: "0x55d398326f99059fF775485246999027B3197955", Decimals: 18, Balance: "100", BalanceRaw: big.NewInt(100), USDValue: "$100.00"},
	}
}

func newOverlay(t *testing.T) Model {
	t.Helper()
	c := chain.Get("bsc")
	m := New(sampleBalances(), c)
	m.SetSize(120, 30)
	return m
}

func keyEnter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }
func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}
func keyEsc() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEsc} }

func TestEscFromStepFromEmitsCancel(t *testing.T) {
	m := newOverlay(t)
	_, cmd := m.Update(keyEsc())
	if cmd == nil {
		t.Fatalf("expected cancel cmd")
	}
	if _, ok := cmd().(CancelMsg); !ok {
		t.Fatalf("expected CancelMsg, got %T", cmd())
	}
}

func TestEnterAdvancesThroughSteps(t *testing.T) {
	m := newOverlay(t)
	if m.step != stepFrom {
		t.Fatalf("initial step %v", m.step)
	}

	m, _ = m.Update(keyEnter()) // pick first item in from list (BNB)
	if m.step != stepTo {
		t.Fatalf("after from-enter step %v, want stepTo", m.step)
	}
	if m.selectedFrom.Symbol != "BNB" {
		t.Fatalf("selectedFrom = %q", m.selectedFrom.Symbol)
	}

	// to-list defaults to first item; for BSC recommended starts with native.
	// Move down once so we don't pick the same token.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(keyEnter())
	if m.step != stepAmount {
		t.Fatalf("after to-enter step %v, want stepAmount", m.step)
	}
	if m.selectedTo.Symbol == "" || m.selectedTo.Symbol == m.selectedFrom.Symbol {
		t.Fatalf("selectedTo = %q (same as from %q)", m.selectedTo.Symbol, m.selectedFrom.Symbol)
	}
}

func TestAmountSubmitEmitsQuoteRequest(t *testing.T) {
	m := newOverlay(t)
	m, _ = m.Update(keyEnter())              // from
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(keyEnter())              // to
	for _, r := range "50" {
		m, _ = m.Update(keyRune(r))
	}
	_, cmd := m.Update(keyEnter())
	if cmd == nil {
		t.Fatalf("expected quote-request cmd")
	}
	req, ok := cmd().(QuoteRequestMsg)
	if !ok {
		t.Fatalf("expected QuoteRequestMsg, got %T", cmd())
	}
	if req.USDAmount != "50" {
		t.Errorf("USDAmount = %q, want 50", req.USDAmount)
	}
}

func TestQuoteResultPopulatesPreviewAndExecuteFires(t *testing.T) {
	m := newOverlay(t)
	m, _ = m.Update(keyEnter())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(keyEnter())
	for _, r := range "50" {
		m, _ = m.Update(keyRune(r))
	}
	m, _ = m.Update(keyEnter()) // emits QuoteRequestMsg

	q := swap.FlowQuote{
		FromToken:          m.selectedFrom.toTokenInfo(),
		ToToken:            m.selectedTo.toTokenInfo(),
		USDAmount:          "50",
		USDAmountFormatted: "$50.00",
		FromTokenAmount:    "0.08",
		EstimatedOutput:    "12.0",
		MinOutput:          "11.94",
		Slippage:           0.5,
	}
	m, _ = m.Update(QuoteResultMsg{Quote: q})
	if m.step != stepPreview {
		t.Fatalf("after quote result step %v, want stepPreview", m.step)
	}
	if m.quote == nil || m.quote.MinOutput != "11.94" {
		t.Errorf("quote not stored: %+v", m.quote)
	}

	_, cmd := m.Update(keyRune('y'))
	if cmd == nil {
		t.Fatalf("expected execute-request cmd")
	}
	if _, ok := cmd().(ExecuteRequestMsg); !ok {
		t.Fatalf("expected ExecuteRequestMsg, got %T", cmd())
	}
}

func TestQuoteResultErrShownStaysInPreview(t *testing.T) {
	m := newOverlay(t)
	m, _ = m.Update(keyEnter())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(keyEnter())
	for _, r := range "50" {
		m, _ = m.Update(keyRune(r))
	}
	m, _ = m.Update(keyEnter())

	m, _ = m.Update(QuoteResultMsg{Err: errors.New("liquidity")})
	if m.step != stepPreview {
		t.Fatalf("step = %v, want stepPreview", m.step)
	}
	if m.quoteErr == "" {
		t.Errorf("quoteErr empty after err result")
	}
	// 'y' must NOT fire execute when there's an error.
	_, cmd := m.Update(keyRune('y'))
	if cmd != nil {
		t.Errorf("'y' on error preview should not fire execute, got %T", cmd())
	}
}

func TestSameTokenRefused(t *testing.T) {
	m := newOverlay(t)
	m, _ = m.Update(keyEnter()) // pick BNB as from
	// First item in `to` list is also BNB (native). Hitting enter must NOT advance.
	prev := m.step
	m, _ = m.Update(keyEnter())
	if m.step != prev {
		t.Fatalf("same-token selection advanced step from %v to %v", prev, m.step)
	}
}

func TestExecutionResultGoesToDone(t *testing.T) {
	m := newOverlay(t)
	m.step = stepExecuting
	m, _ = m.Update(ExecutionResultMsg{Result: swap.FlowResult{Success: true, TxHash: "0xabc"}})
	if m.step != stepDone {
		t.Fatalf("after execution result step %v, want stepDone", m.step)
	}
	// Any key dismisses.
	_, cmd := m.Update(keyRune('x'))
	if cmd == nil {
		t.Fatalf("expected cancel cmd on stepDone keypress")
	}
	if _, ok := cmd().(CancelMsg); !ok {
		t.Fatalf("expected CancelMsg, got %T", cmd())
	}
}
