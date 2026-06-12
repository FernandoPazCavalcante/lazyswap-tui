package wallet

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/crypto"
)

// hardhatMnemonic is the well-known Hardhat / Foundry default test phrase.
// First account at m/44'/60'/0'/0/0:
//
//	address     = 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
//	private key = 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80
const (
	hardhatMnemonic = "test test test test test test test test test test test junk"
	hardhatAddress  = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	hardhatPrivKey  = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
)

func newSvc(t *testing.T) *Service {
	t.Helper()
	dao, err := OpenAt(filepath.Join(t.TempDir(), "wallets.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = dao.Close() })

	key, _, err := crypto.DeriveKey("hunter2", make([]byte, crypto.SaltLength))
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	c, err := crypto.New(key)
	if err != nil {
		t.Fatalf("crypto.New: %v", err)
	}
	return NewService(dao, c)
}

func TestImportHardhatVector(t *testing.T) {
	svc := newSvc(t)

	w, err := svc.Import(hardhatMnemonic)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if w.Address != hardhatAddress {
		t.Errorf("address = %q, want %q", w.Address, hardhatAddress)
	}
	if w.PrivateKey != hardhatPrivKey {
		t.Errorf("private_key = %q, want %q", w.PrivateKey, hardhatPrivKey)
	}
	if w.Mnemonic != hardhatMnemonic {
		t.Errorf("mnemonic returned not as supplied")
	}
}

func TestImportTrimsWhitespace(t *testing.T) {
	svc := newSvc(t)

	w, err := svc.Import("  " + hardhatMnemonic + "\n")
	if err != nil {
		t.Fatalf("Import (with whitespace): %v", err)
	}
	if w.Address != hardhatAddress {
		t.Errorf("address = %q, want %q", w.Address, hardhatAddress)
	}
}

func TestImportRejectsInvalid(t *testing.T) {
	svc := newSvc(t)
	if _, err := svc.Import("not a real mnemonic phrase at all"); err == nil {
		t.Fatalf("expected error for invalid mnemonic")
	}
	if _, err := svc.Import(""); err == nil {
		t.Fatalf("expected error for empty mnemonic")
	}
}

func TestCreateProducesValidWallet(t *testing.T) {
	svc := newSvc(t)

	w, err := svc.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(w.Address, "0x") || len(w.Address) != 42 {
		t.Errorf("bad address: %q", w.Address)
	}
	if !strings.HasPrefix(w.PrivateKey, "0x") || len(w.PrivateKey) != 66 {
		t.Errorf("bad private key: %q", w.PrivateKey)
	}
	if words := len(strings.Fields(w.Mnemonic)); words != 12 {
		t.Errorf("mnemonic word count = %d, want 12", words)
	}
}

func TestFetchAllRoundtrip(t *testing.T) {
	svc := newSvc(t)

	w1, err := svc.Import(hardhatMnemonic)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	w2, err := svc.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	all, err := svc.FetchAll()
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("FetchAll len = %d, want 2", len(all))
	}

	byAddr := map[string]Wallet{all[0].Address: all[0], all[1].Address: all[1]}
	if got := byAddr[w1.Address].PrivateKey; got != hardhatPrivKey {
		t.Errorf("w1 private_key roundtrip: got %q want %q", got, hardhatPrivKey)
	}
	if got := byAddr[w2.Address].Mnemonic; got != w2.Mnemonic {
		t.Errorf("w2 mnemonic roundtrip mismatch")
	}
}

func TestDeleteRemovesRow(t *testing.T) {
	svc := newSvc(t)
	w, _ := svc.Create()

	if err := svc.Delete(w.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all, _ := svc.FetchAll()
	if len(all) != 0 {
		t.Errorf("expected empty after delete, got %d", len(all))
	}
}
