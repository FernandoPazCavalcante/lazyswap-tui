package settings

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func enter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }
func down() tea.KeyMsg  { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}} }

func TestNetworkEnterEmitsNextChain(t *testing.T) {
	m := New(0.5, "bsc", "BSC")
	m, _ = m.Update(down()) // move cursor to Network
	_, cmd := m.Update(enter())
	if cmd == nil {
		t.Fatal("expected a NetworkChangeMsg command")
	}
	msg, ok := cmd().(NetworkChangeMsg)
	if !ok {
		t.Fatalf("expected NetworkChangeMsg, got %T", cmd())
	}
	if msg.ChainKey != "bsc_testnet" {
		t.Fatalf("next chain after bsc = %q, want bsc_testnet", msg.ChainKey)
	}
}

func TestWalletEnterEmitsShowWallet(t *testing.T) {
	m := New(0.5, "bsc", "BSC")
	m, _ = m.Update(down()) // Slippage -> Network
	m, _ = m.Update(down()) // Network -> Wallet
	_, cmd := m.Update(enter())
	if cmd == nil {
		t.Fatal("expected a ShowWalletMsg command")
	}
	if _, ok := cmd().(ShowWalletMsg); !ok {
		t.Fatalf("expected ShowWalletMsg, got %T", cmd())
	}
}

func TestSlippageEnterEntersEditMode(t *testing.T) {
	m := New(0.5, "bsc", "BSC")
	if m.Capturing() {
		t.Fatal("should not be capturing before edit")
	}
	m, cmd := m.Update(enter()) // cursor starts on Slippage
	if !m.Capturing() {
		t.Fatal("enter on slippage should enter edit mode")
	}
	if cmd == nil {
		t.Fatal("expected textinput.Blink cmd")
	}
}

func TestSlippageCommitEmitsChange(t *testing.T) {
	m := New(0.5, "bsc", "BSC")
	m, _ = m.Update(enter())    // enter edit (prefilled "0.5")
	m, cmd := m.Update(enter()) // commit prefilled value
	if m.Capturing() {
		t.Fatal("commit should exit edit mode")
	}
	if cmd == nil {
		t.Fatal("expected SlippageChangedMsg command")
	}
	msg, ok := cmd().(SlippageChangedMsg)
	if !ok {
		t.Fatalf("expected SlippageChangedMsg, got %T", cmd())
	}
	if msg.Value != 0.5 {
		t.Fatalf("committed slippage = %v, want 0.5", msg.Value)
	}
}

func TestSlippageInvalidRejected(t *testing.T) {
	m := New(0.5, "bsc", "BSC")
	m, _ = m.Update(enter())                                            // edit mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}) // "0.5x"
	m, cmd := m.Update(enter())                                         // commit invalid
	if cmd != nil {
		t.Fatal("invalid slippage should not emit a change")
	}
	if m.slippage != 0.5 {
		t.Fatalf("slippage should stay 0.5, got %v", m.slippage)
	}
}

func TestSlippageEscCancels(t *testing.T) {
	m := New(0.5, "bsc", "BSC")
	m, _ = m.Update(enter())                         // edit mode
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // cancel
	if m.Capturing() {
		t.Fatal("esc should exit edit mode")
	}
	if cmd != nil {
		t.Fatal("esc should not emit a change")
	}
}
