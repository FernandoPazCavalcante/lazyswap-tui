package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/crypto"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/screens/login"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/wallet"
)

func newRoot(t *testing.T) Root {
	t.Helper()
	dao, err := wallet.OpenAt(filepath.Join(t.TempDir(), "wallets.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = dao.Close() })
	r, err := NewRoot(dao, "")
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	return r
}

func TestRootStartsOnLogin(t *testing.T) {
	r := newRoot(t)
	if r.screen != screenLogin {
		t.Fatalf("expected screenLogin, got %v", r.screen)
	}
}

func TestRootSwitchesToMainOnLoginSuccess(t *testing.T) {
	r := newRoot(t)
	svc, err := crypto.New(make([]byte, crypto.KeyLength))
	if err != nil {
		t.Fatalf("crypto.New: %v", err)
	}

	m, _ := r.Update(login.LoginSuccessMsg{Service: svc})
	rr, ok := m.(Root)
	if !ok {
		t.Fatalf("Update returned non-Root: %T", m)
	}
	if rr.screen != screenMain {
		t.Fatalf("expected screenMain, got %v", rr.screen)
	}
	if rr.svc != svc {
		t.Fatalf("expected stored service")
	}
}

func TestCtrlCQuits(t *testing.T) {
	r := newRoot(t)
	_, cmd := r.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected quit cmd")
	}
	// tea.Quit is a function that returns tea.QuitMsg{} when invoked.
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
}
