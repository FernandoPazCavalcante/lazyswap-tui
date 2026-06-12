package mainscreen

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/crypto"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/overlays/importoverlay"
	settingspanel "github.com/FernandoPazCavalcante/lazyswap-tui/internal/tui/panels/settings"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/wallet"
)

const hardhatPhrase = "test test test test test test test test test test test junk"

func newModel(t *testing.T) Model {
	t.Helper()
	dao, err := wallet.OpenAt(filepath.Join(t.TempDir(), "wallets.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = dao.Close() })

	key, _, _ := crypto.DeriveKey("hunter2", make([]byte, crypto.SaltLength))
	c, _ := crypto.New(key)
	svc := wallet.NewService(dao, c)

	m := New(svc, nil, nil, "") // nil balance + swap services — RPC not exercised in unit tests
	m.SetSize(120, 30)
	return m
}

func TestCKeyEntersConfirmCreate(t *testing.T) {
	m := newModel(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if m.mode != modeConfirmCreate {
		t.Fatalf("mode = %v, want modeConfirmCreate", m.mode)
	}
}

func TestNKeyDuringConfirmCreateCancels(t *testing.T) {
	m := newModel(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.mode != modeNormal {
		t.Fatalf("after 'n' mode = %v, want modeNormal", m.mode)
	}
}

func TestYKeyDuringConfirmCreateIssuesCommand(t *testing.T) {
	m := newModel(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatalf("expected a create command")
	}
	msg := cmd()
	cm, ok := msg.(createdMsg)
	if !ok {
		t.Fatalf("expected createdMsg, got %T", msg)
	}
	if cm.err != nil {
		t.Fatalf("create failed: %v", cm.err)
	}
	if cm.w == nil || cm.w.Address == "" {
		t.Fatalf("created wallet missing")
	}
}

func TestDKeyWithoutSelectionStaysNormal(t *testing.T) {
	m := newModel(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.mode != modeNormal {
		t.Fatalf("d with no current should stay normal; got %v", m.mode)
	}
}

func TestIKeyEntersImportMode(t *testing.T) {
	m := newModel(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if m.mode != modeImport {
		t.Fatalf("after 'i' mode = %v, want modeImport", m.mode)
	}
}

func TestImportSubmitTriggersImportCmd(t *testing.T) {
	m := newModel(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	_, cmd := m.Update(importoverlay.SubmitMsg{Phrase: hardhatPhrase})
	if cmd == nil {
		t.Fatalf("expected import cmd")
	}
	msg := cmd()
	im, ok := msg.(importedMsg)
	if !ok {
		t.Fatalf("expected importedMsg, got %T", msg)
	}
	if im.err != nil {
		t.Fatalf("import failed: %v", im.err)
	}
}

func TestImportEmptyShowsErrorAndStaysInOverlay(t *testing.T) {
	m := newModel(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	m, cmd := m.Update(importoverlay.SubmitMsg{Phrase: ""})
	if cmd != nil {
		t.Fatalf("empty submit should not issue a cmd")
	}
	if m.mode != modeImport {
		t.Fatalf("empty submit should keep modeImport, got %v", m.mode)
	}
}

func TestImportCancelExitsOverlay(t *testing.T) {
	m := newModel(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	m, _ = m.Update(importoverlay.CancelMsg{})
	if m.mode != modeNormal {
		t.Fatalf("after cancel mode = %v, want modeNormal", m.mode)
	}
}

func TestRefreshedMsgPopulatesWallets(t *testing.T) {
	m := newModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	cmd = nil // unused
	_ = cmd

	// Run a create directly via the service so we have data, then feed a
	// refreshedMsg into the model and assert it absorbs it.
	w, err := m.svc.Create()
	if err != nil {
		t.Fatalf("svc.Create: %v", err)
	}

	m, _ = m.Update(walletsRefreshedMsg{wallets: []wallet.Wallet{*w}})
	if len(m.wallets) != 1 {
		t.Fatalf("expected 1 wallet in state, got %d", len(m.wallets))
	}
	if m.current == nil || m.current.Address != w.Address {
		t.Fatalf("current wallet not set to %s", w.Address)
	}
}

// modelWithWallet returns a model with one persisted, selected wallet.
func modelWithWallet(t *testing.T) Model {
	t.Helper()
	m := newModel(t)
	w, err := m.svc.Create()
	if err != nil {
		t.Fatalf("svc.Create: %v", err)
	}
	m, _ = m.Update(walletsRefreshedMsg{wallets: []wallet.Wallet{*w}})
	return m
}

func TestShowWalletMsgEntersQRMode(t *testing.T) {
	m := modelWithWallet(t)
	m, _ = m.Update(settingspanel.ShowWalletMsg{})
	if m.mode != modeWalletQR {
		t.Fatalf("mode = %v, want modeWalletQR", m.mode)
	}
}

func TestShowWalletMsgNoSelectionStaysNormal(t *testing.T) {
	m := newModel(t) // no wallets
	m, _ = m.Update(settingspanel.ShowWalletMsg{})
	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want modeNormal", m.mode)
	}
	if m.errMsg == "" {
		t.Fatal("expected an error message when no wallet is selected")
	}
}

func TestWalletQRModeEscCloses(t *testing.T) {
	m := modelWithWallet(t)
	m, _ = m.Update(settingspanel.ShowWalletMsg{})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeNormal {
		t.Fatalf("after esc mode = %v, want modeNormal", m.mode)
	}
}
