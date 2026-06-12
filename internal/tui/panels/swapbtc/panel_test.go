package swapbtc

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/balance"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/swap"
)

const validBTC = "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh" // 42 chars

func seeded() Model {
	m := New()
	m.SetSize(60, 30)
	m.SetBalances([]balance.TokenBalance{
		{Symbol: "BNB", Address: "native", Decimals: 18, Balance: "1.5", USDValue: "$900.00"},
	})
	return m
}

func keyRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func keyEnter() tea.KeyMsg         { return tea.KeyMsg{Type: tea.KeyEnter} }

func TestFormFlowEmitsQuoteRequest(t *testing.T) {
	m := seeded()

	// token → amount
	m, _ = m.Update(keyEnter())
	if m.field != fieldAmount {
		t.Fatalf("after enter on token, field = %v, want fieldAmount", m.field)
	}
	if !m.Capturing() {
		t.Fatal("should capture input while editing amount")
	}

	// type amount, advance to addr
	m, _ = m.Update(keyRunes("50"))
	m, _ = m.Update(keyEnter())
	if m.field != fieldAddr {
		t.Fatalf("after enter on amount, field = %v, want fieldAddr", m.field)
	}

	// type a valid BTC address, submit
	m, _ = m.Update(keyRunes(validBTC))
	m, cmd := m.Update(keyEnter())
	if m.state != stateQuoting {
		t.Fatalf("after submit, state = %v, want stateQuoting", m.state)
	}
	if cmd == nil {
		t.Fatal("expected a QuoteRequestMsg command")
	}
	msg, ok := cmd().(QuoteRequestMsg)
	if !ok {
		t.Fatalf("expected QuoteRequestMsg, got %T", cmd())
	}
	if msg.From.Symbol != "BNB" || msg.USDAmount != "50" || msg.BTCAddress != validBTC {
		t.Fatalf("bad request: %+v", msg)
	}
}

func TestSubmitWithInvalidInputsRejected(t *testing.T) {
	m := seeded()
	m, _ = m.Update(keyEnter()) // token → amount
	m, _ = m.Update(keyEnter()) // amount empty → addr
	m, _ = m.Update(keyRunes("short"))
	m, cmd := m.Update(keyEnter()) // submit with bad amount + short addr
	if cmd != nil {
		t.Fatal("invalid inputs should not emit a quote request")
	}
	if m.state != stateForm {
		t.Fatalf("state = %v, want stateForm", m.state)
	}
	if m.quoteErr == "" {
		t.Fatal("expected a validation error message")
	}
}

func TestQuoteResultEntersPreviewWithCountdown(t *testing.T) {
	m := seeded()
	m, cmd := m.Update(QuoteResultMsg{Quote: swap.FlowQuote{
		FromToken:       swap.TokenInfo{Symbol: "BNB"},
		EstimatedOutput: "0.01",
		MinOutput:       "0.0097",
	}})
	if m.state != statePreview {
		t.Fatalf("state = %v, want statePreview", m.state)
	}
	if m.countdown != refreshSeconds {
		t.Fatalf("countdown = %d, want %d", m.countdown, refreshSeconds)
	}
	if cmd == nil {
		t.Fatal("expected a tick command to start the countdown")
	}
	if _, ok := cmd().(TickMsg); !ok {
		t.Fatalf("expected TickMsg, got %T", cmd())
	}
}

func TestQuoteErrorReturnsToForm(t *testing.T) {
	m := seeded()
	m, _ = m.Update(QuoteResultMsg{Err: errTest{}})
	if m.state != stateForm {
		t.Fatalf("state = %v, want stateForm on quote error", m.state)
	}
	if m.quoteErr == "" {
		t.Fatal("expected quoteErr set")
	}
}

func TestPreviewExecuteEmitsExecuteRequest(t *testing.T) {
	m := seeded()
	// Get a token selected + into preview.
	m, _ = m.Update(keyEnter())
	m, _ = m.Update(keyRunes("50"))
	m, _ = m.Update(keyEnter())
	m, _ = m.Update(keyRunes(validBTC))
	m, _ = m.Update(keyEnter()) // → stateQuoting, sets m.from
	m, _ = m.Update(QuoteResultMsg{Quote: swap.FlowQuote{FromToken: swap.TokenInfo{Symbol: "BNB"}}})

	_, cmd := m.Update(keyRunes("y"))
	if cmd == nil {
		t.Fatal("expected an ExecuteRequestMsg command")
	}
	msg, ok := cmd().(ExecuteRequestMsg)
	if !ok {
		t.Fatalf("expected ExecuteRequestMsg, got %T", cmd())
	}
	if msg.BTCAddress != validBTC || msg.USDAmount != "50" {
		t.Fatalf("bad execute request: %+v", msg)
	}
}

func TestTickExpiryRequotes(t *testing.T) {
	m := seeded()
	// Drive into preview with a real selected token first.
	m, _ = m.Update(keyEnter())
	m, _ = m.Update(keyRunes("50"))
	m, _ = m.Update(keyEnter())
	m, _ = m.Update(keyRunes(validBTC))
	m, _ = m.Update(keyEnter())
	m, _ = m.Update(QuoteResultMsg{Quote: swap.FlowQuote{FromToken: swap.TokenInfo{Symbol: "BNB"}}})

	m.countdown = 1
	m, cmd := m.Update(TickMsg{})
	if m.state != stateQuoting {
		t.Fatalf("after countdown expiry state = %v, want stateQuoting", m.state)
	}
	if cmd == nil {
		t.Fatal("expected a re-quote command")
	}
	if _, ok := cmd().(QuoteRequestMsg); !ok {
		t.Fatalf("expected QuoteRequestMsg on re-quote, got %T", cmd())
	}
}

func TestAmountConfirmStartsEstimate(t *testing.T) {
	m := seeded()
	m, _ = m.Update(keyEnter())     // token → amount
	m, _ = m.Update(keyRunes("50")) // type amount
	m, _ = m.Update(keyEnter())     // confirm amount → addr + estimate
	if m.field != fieldAddr {
		t.Fatalf("field = %v, want fieldAddr", m.field)
	}
	if !m.estimating {
		t.Fatal("confirming amount should start an estimate")
	}
	if m.from.Symbol != "BNB" {
		t.Fatalf("estimate source = %q, want BNB", m.from.Symbol)
	}
}

func TestEstimateResultRendersBTCAndSats(t *testing.T) {
	m := seeded()
	m.estimating = true
	m, _ = m.Update(EstimateResultMsg{Quote: swap.FlowQuote{
		EstimatedOutput:     "0.00072145",
		EstimatedOutputSats: 72145,
	}})
	if m.estimating {
		t.Fatal("estimate result should clear the estimating flag")
	}
	if m.estimate == nil {
		t.Fatal("estimate should be set")
	}
	line := m.estimateLine()
	if !strings.Contains(line, "0.00072145 BTC") {
		t.Fatalf("estimate line missing BTC amount: %q", line)
	}
	if !strings.Contains(line, "72,145 sats") {
		t.Fatalf("estimate line missing sats: %q", line)
	}
}

func TestTypingAmountClearsStaleEstimate(t *testing.T) {
	m := seeded()
	m, _ = m.Update(keyEnter()) // token → amount
	m.estimate = &swap.FlowQuote{EstimatedOutput: "0.001"}
	m, _ = m.Update(keyRunes("7")) // a keystroke invalidates the estimate
	if m.estimate != nil {
		t.Fatal("typing a new amount should drop the stale estimate")
	}
}

func TestFormatSats(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 1000: "1,000", 72145: "72,145", 100000000: "100,000,000"}
	for in, want := range cases {
		if got := formatSats(in); got != want {
			t.Fatalf("formatSats(%d) = %q, want %q", in, got, want)
		}
	}
}

type errTest struct{}

func (errTest) Error() string { return "quote failed" }
