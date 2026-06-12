package wallet

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func openTestDAO(t *testing.T) *DAO {
	t.Helper()
	dir := t.TempDir()
	dao, err := OpenAt(filepath.Join(dir, "wallets.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = dao.Close() })
	return dao
}

func TestInsertAndFetchAll(t *testing.T) {
	dao := openTestDAO(t)

	w := &Wallet{
		Address:    "0xabc",
		PrivateKey: "encrypted-priv",
		Mnemonic:   "encrypted-mnemonic",
	}
	if err := dao.Insert(w); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if w.ID == "" {
		t.Fatalf("expected UUIDv7 to be generated")
	}

	all, err := dao.FetchAll()
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("FetchAll len = %d, want 1", len(all))
	}
	got := all[0]
	if got.Address != "0xabc" || got.PrivateKey != "encrypted-priv" || got.Mnemonic != "encrypted-mnemonic" {
		t.Fatalf("FetchAll row mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not populated")
	}
}

func TestInsertWithoutMnemonic(t *testing.T) {
	dao := openTestDAO(t)

	w := &Wallet{Address: "0xdef", PrivateKey: "encrypted"}
	if err := dao.Insert(w); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	all, _ := dao.FetchAll()
	if all[0].Mnemonic != "" {
		t.Fatalf("expected empty mnemonic, got %q", all[0].Mnemonic)
	}
}

func TestInsertDuplicateAddressFails(t *testing.T) {
	dao := openTestDAO(t)

	if err := dao.Insert(&Wallet{Address: "0xabc", PrivateKey: "p"}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := dao.Insert(&Wallet{Address: "0xabc", PrivateKey: "q"}); err == nil {
		t.Fatalf("duplicate address insert should fail (UNIQUE)")
	}
}

func TestGetByAddress(t *testing.T) {
	dao := openTestDAO(t)
	_ = dao.Insert(&Wallet{Address: "0xabc", PrivateKey: "p"})

	got, err := dao.GetByAddress("0xabc")
	if err != nil {
		t.Fatalf("GetByAddress: %v", err)
	}
	if got.Address != "0xabc" {
		t.Fatalf("address = %q", got.Address)
	}

	if _, err := dao.GetByAddress("0x000"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetByAddress missing: want ErrNoRows, got %v", err)
	}
}

func TestDeleteByID(t *testing.T) {
	dao := openTestDAO(t)
	w := &Wallet{Address: "0xabc", PrivateKey: "p"}
	_ = dao.Insert(w)

	if err := dao.DeleteByID(w.ID); err != nil {
		t.Fatalf("DeleteByID: %v", err)
	}
	all, _ := dao.FetchAll()
	if len(all) != 0 {
		t.Fatalf("expected empty after delete, got %d rows", len(all))
	}
}

func TestAppConfigSaltAndSentinel(t *testing.T) {
	dao := openTestDAO(t)

	ok, err := dao.IsEncryptionInitialised()
	if err != nil || ok {
		t.Fatalf("fresh DB should not be initialised: ok=%v err=%v", ok, err)
	}

	if err := dao.SetSalt("deadbeef"); err != nil {
		t.Fatalf("SetSalt: %v", err)
	}
	if err := dao.SetSentinel("iv:tag:ct"); err != nil {
		t.Fatalf("SetSentinel: %v", err)
	}

	salt, ok, err := dao.GetSalt()
	if err != nil || !ok || salt != "deadbeef" {
		t.Fatalf("GetSalt: ok=%v salt=%q err=%v", ok, salt, err)
	}
	sent, ok, err := dao.GetSentinel()
	if err != nil || !ok || sent != "iv:tag:ct" {
		t.Fatalf("GetSentinel: ok=%v sent=%q err=%v", ok, sent, err)
	}

	ok, err = dao.IsEncryptionInitialised()
	if err != nil || !ok {
		t.Fatalf("after set: want initialised, got ok=%v err=%v", ok, err)
	}
}

func TestSetSaltOverwrites(t *testing.T) {
	dao := openTestDAO(t)
	_ = dao.SetSalt("aaaa")
	_ = dao.SetSalt("bbbb")
	got, _, _ := dao.GetSalt()
	if got != "bbbb" {
		t.Fatalf("salt = %q, want bbbb", got)
	}
}
