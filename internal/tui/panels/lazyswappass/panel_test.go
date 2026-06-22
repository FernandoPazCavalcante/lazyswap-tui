package lazyswappass

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/FernandoPazCavalcante/lazyswap/internal/pass"
)

func enter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }

// emitsBuy runs Update with Enter and reports whether a BuyRequestMsg was emitted.
func emitsBuy(m Model) (Model, bool) {
	m2, cmd := m.Update(enter())
	if cmd == nil {
		return m2, false
	}
	_, ok := cmd().(BuyRequestMsg)
	return m2, ok
}

func TestEnter_BuysWhenAvailableAndNoPass(t *testing.T) {
	m := New()
	m.SetAvailable(true, "tBNB")
	m.SetStatus(pass.Status{}) // loaded, no valid pass
	m2, ok := emitsBuy(m)
	if !ok {
		t.Fatal("expected BuyRequestMsg when available with a loaded, no-pass status")
	}
	if !m2.buying {
		t.Error("expected buying=true after buy request")
	}
}

func TestEnter_NoBuyBeforeStatusLoaded(t *testing.T) {
	m := New()
	m.SetAvailable(true, "tBNB") // available but status not yet fetched
	if _, ok := emitsBuy(m); ok {
		t.Error("should not mint before the first status fetch completes")
	}
}

func TestEnter_NoBuyWhenHasValidPass(t *testing.T) {
	m := New()
	m.SetAvailable(true, "tBNB")
	m.SetStatus(pass.Status{HasValidPass: true, ExpiresAt: time.Now().Add(300 * 24 * time.Hour)})
	if _, ok := emitsBuy(m); ok {
		t.Error("should not buy while a valid pass is held")
	}
}

func TestEnter_NoBuyWhenUnavailable(t *testing.T) {
	m := New() // available defaults false
	if _, ok := emitsBuy(m); ok {
		t.Error("should not buy when pass unavailable on chain")
	}
}

func TestEnter_NoBuyWhileBuying(t *testing.T) {
	m := New()
	m.SetAvailable(true, "tBNB")
	m.SetStatus(pass.Status{}) // loaded, no pass
	m.SetBuying(true)
	if _, ok := emitsBuy(m); ok {
		t.Error("should not start a second buy while one is in flight")
	}
}

func TestSetError_StopsBuying(t *testing.T) {
	m := New()
	m.SetBuying(true)
	m.SetError("boom")
	if m.buying {
		t.Error("SetError should clear buying")
	}
	if m.errMsg != "boom" {
		t.Errorf("errMsg = %q, want boom", m.errMsg)
	}
}

func TestSetStatus_ClearsBuyingAndError(t *testing.T) {
	m := New()
	m.SetBuying(true)
	m.SetError("boom")
	m.SetStatus(pass.Status{HasValidPass: true, ExpiresAt: time.Now().Add(24 * time.Hour)})
	if m.buying || m.errMsg != "" || !m.loaded {
		t.Errorf("SetStatus state = {buying:%v err:%q loaded:%v}, want {false \"\" true}", m.buying, m.errMsg, m.loaded)
	}
}
