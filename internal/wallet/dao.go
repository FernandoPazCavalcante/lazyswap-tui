// SQLite-backed persistence for wallets + app config.
//
// Mirrors src/wallet/wallet.dao.ts schema verbatim so a v0.1.x SQLite file
// drops in unchanged.
package wallet

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/paths"
)

// DAO is a thin wrapper around *sql.DB exposing the wallet/app_config tables.
type DAO struct {
	db *sql.DB
}

// Open the wallets.db located in the resolved data dir and apply the schema.
func Open() (*DAO, error) {
	dir, err := paths.EnsureDataDir()
	if err != nil {
		return nil, fmt.Errorf("ensure data dir: %w", err)
	}
	return OpenAt(filepath.Join(dir, "wallets.db"))
}

// OpenAt opens a SQLite database at the given path. Used by tests.
func OpenAt(path string) (*DAO, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if err := applySchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &DAO{db: db}, nil
}

func (d *DAO) Close() error { return d.db.Close() }

func applySchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS wallets (
			id TEXT PRIMARY KEY,
			address TEXT NOT NULL UNIQUE,
			private_key TEXT NOT NULL,
			mnemonic TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS app_config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}
	return nil
}

// ─── wallets table ───────────────────────────────────────────────────────────

// Insert stores a new wallet. If w.ID is empty, a UUIDv7 is generated. If
// CreatedAt/UpdatedAt are zero, time.Now().UTC() is used.
func (d *DAO) Insert(w *Wallet) error {
	if w.Address == "" {
		return errors.New("address required")
	}
	if w.PrivateKey == "" {
		return errors.New("private_key required")
	}
	if w.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("uuidv7: %w", err)
		}
		w.ID = id.String()
	}
	now := time.Now().UTC()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	if w.UpdatedAt.IsZero() {
		w.UpdatedAt = now
	}

	_, err := d.db.Exec(
		`INSERT INTO wallets (id, address, private_key, mnemonic, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		w.ID, w.Address, w.PrivateKey, nullString(w.Mnemonic),
		w.CreatedAt.Format(time.RFC3339Nano),
		w.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

// FetchAll returns every wallet, ordered by creation time descending
// (newest first, matching the Bun reference).
func (d *DAO) FetchAll() ([]Wallet, error) {
	rows, err := d.db.Query(
		`SELECT id, address, private_key, COALESCE(mnemonic, ''), created_at, updated_at
		 FROM wallets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Wallet
	for rows.Next() {
		var w Wallet
		var createdStr, updatedStr string
		if err := rows.Scan(&w.ID, &w.Address, &w.PrivateKey, &w.Mnemonic, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		if w.CreatedAt, err = time.Parse(time.RFC3339Nano, createdStr); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		if w.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedStr); err != nil {
			return nil, fmt.Errorf("parse updated_at: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetByAddress returns the wallet matching the given address, or sql.ErrNoRows.
func (d *DAO) GetByAddress(address string) (*Wallet, error) {
	var w Wallet
	var createdStr, updatedStr string
	err := d.db.QueryRow(
		`SELECT id, address, private_key, COALESCE(mnemonic, ''), created_at, updated_at
		 FROM wallets WHERE address = ?`, address,
	).Scan(&w.ID, &w.Address, &w.PrivateKey, &w.Mnemonic, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}
	if w.CreatedAt, err = time.Parse(time.RFC3339Nano, createdStr); err != nil {
		return nil, err
	}
	if w.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedStr); err != nil {
		return nil, err
	}
	return &w, nil
}

// DeleteByID removes a single wallet row. No-op if no row matches.
func (d *DAO) DeleteByID(id string) error {
	_, err := d.db.Exec(`DELETE FROM wallets WHERE id = ?`, id)
	return err
}

// ─── app_config table ────────────────────────────────────────────────────────

const (
	keyEncryptionSalt   = "encryption_salt"
	keyPasswordSentinel = "password_sentinel"
)

func (d *DAO) getConfig(key string) (string, bool, error) {
	var v string
	err := d.db.QueryRow(`SELECT value FROM app_config WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (d *DAO) setConfig(key, value string) error {
	_, err := d.db.Exec(
		`INSERT INTO app_config (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// GetSalt returns the hex-encoded PBKDF2 salt, or ("", false) if not initialised.
func (d *DAO) GetSalt() (string, bool, error)     { return d.getConfig(keyEncryptionSalt) }
func (d *DAO) SetSalt(saltHex string) error       { return d.setConfig(keyEncryptionSalt, saltHex) }
func (d *DAO) GetSentinel() (string, bool, error) { return d.getConfig(keyPasswordSentinel) }
func (d *DAO) SetSentinel(envelope string) error  { return d.setConfig(keyPasswordSentinel, envelope) }

// IsEncryptionInitialised is true once both salt and sentinel are persisted.
func (d *DAO) IsEncryptionInitialised() (bool, error) {
	_, saltOK, err := d.GetSalt()
	if err != nil {
		return false, err
	}
	_, sentOK, err := d.GetSentinel()
	if err != nil {
		return false, err
	}
	return saltOK && sentOK, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
