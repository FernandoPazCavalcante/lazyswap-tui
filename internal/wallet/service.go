// Wallet service layer. Mirrors src/wallet/{create,import,fetch,delete}-wallet.service.ts.
// All persisted fields are AES-256-GCM encrypted via crypto.Service.
package wallet

import (
	"fmt"
	"strings"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/crypto"
)

// Service bundles create / import / fetch / delete operations.
// One instance per logged-in session (holds the crypto key).
type Service struct {
	dao    *DAO
	crypto *crypto.Service
}

// NewService wires the DAO and the active crypto session.
func NewService(dao *DAO, c *crypto.Service) *Service {
	return &Service{dao: dao, crypto: c}
}

// Create generates a fresh wallet, persists it encrypted, and returns the
// plaintext Wallet (caller never sees ciphertext).
func (s *Service) Create() (*Wallet, error) {
	mnemonic, err := newMnemonic()
	if err != nil {
		return nil, err
	}
	return s.insertFromMnemonic(mnemonic)
}

// Import parses a BIP-39 phrase and persists the derived wallet.
// Whitespace is trimmed; the mnemonic is otherwise stored verbatim.
func (s *Service) Import(phrase string) (*Wallet, error) {
	trimmed := strings.TrimSpace(phrase)
	if trimmed == "" {
		return nil, fmt.Errorf("empty mnemonic")
	}
	return s.insertFromMnemonic(trimmed)
}

// FetchAll returns every persisted wallet with its private_key and mnemonic
// fields decrypted to plaintext.
func (s *Service) FetchAll() ([]Wallet, error) {
	rows, err := s.dao.FetchAll()
	if err != nil {
		return nil, err
	}
	out := make([]Wallet, 0, len(rows))
	for _, w := range rows {
		privHex, err := s.crypto.Decrypt(w.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt private_key for %s: %w", w.Address, err)
		}
		var mn string
		if w.Mnemonic != "" {
			mn, err = s.crypto.Decrypt(w.Mnemonic)
			if err != nil {
				return nil, fmt.Errorf("decrypt mnemonic for %s: %w", w.Address, err)
			}
		}
		out = append(out, Wallet{
			ID:         w.ID,
			Address:    w.Address,
			PrivateKey: privHex,
			Mnemonic:   mn,
			CreatedAt:  w.CreatedAt,
			UpdatedAt:  w.UpdatedAt,
		})
	}
	return out, nil
}

// Delete removes a wallet by ID. No-op if no row matches.
func (s *Service) Delete(id string) error {
	return s.dao.DeleteByID(id)
}

// ─── internals ───────────────────────────────────────────────────────────────

func (s *Service) insertFromMnemonic(mnemonic string) (*Wallet, error) {
	privHex, address, err := deriveEthereumWallet(mnemonic)
	if err != nil {
		return nil, err
	}

	encPriv, err := s.crypto.Encrypt(privHex)
	if err != nil {
		return nil, fmt.Errorf("encrypt private_key: %w", err)
	}
	encMnem, err := s.crypto.Encrypt(mnemonic)
	if err != nil {
		return nil, fmt.Errorf("encrypt mnemonic: %w", err)
	}

	w := &Wallet{
		Address:    address,
		PrivateKey: encPriv,
		Mnemonic:   encMnem,
	}
	if err := s.dao.Insert(w); err != nil {
		return nil, err
	}

	// Return the entity with plaintext fields.
	w.PrivateKey = privHex
	w.Mnemonic = mnemonic
	return w, nil
}
