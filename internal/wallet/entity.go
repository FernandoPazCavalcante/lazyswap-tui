// Package wallet owns wallet entities and persistence.
package wallet

import "time"

// Wallet represents a single user wallet. PrivateKey and Mnemonic are stored
// AES-256-GCM encrypted once encryption is initialised.
type Wallet struct {
	ID         string
	Address    string
	PrivateKey string // encrypted envelope when encryption is enabled
	Mnemonic   string // encrypted envelope, may be empty
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
