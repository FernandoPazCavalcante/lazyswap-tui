package wallet

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/tyler-smith/go-bip39"
)

// EthereumDerivationPath is the BIP-44 default for Ethereum (m/44'/60'/0'/0/0).
// Matches ethers.js Wallet.fromPhrase / Wallet.createRandom defaults.
var EthereumDerivationPath = []uint32{
	hdkeychain.HardenedKeyStart + 44,
	hdkeychain.HardenedKeyStart + 60,
	hdkeychain.HardenedKeyStart + 0,
	0,
	0,
}

// deriveEthereumWallet produces the (0x-prefixed) private key hex and
// EIP-55 checksummed address from a BIP-39 mnemonic. Returns an error if the
// mnemonic fails validation.
func deriveEthereumWallet(mnemonic string) (privHex, address string, err error) {
	if !bip39.IsMnemonicValid(mnemonic) {
		return "", "", errors.New("invalid BIP-39 mnemonic")
	}

	seed := bip39.NewSeed(mnemonic, "")
	master, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return "", "", fmt.Errorf("new master: %w", err)
	}

	key := master
	for _, idx := range EthereumDerivationPath {
		key, err = key.Derive(idx)
		if err != nil {
			return "", "", fmt.Errorf("derive %d: %w", idx, err)
		}
	}

	ecPriv, err := key.ECPrivKey()
	if err != nil {
		return "", "", fmt.Errorf("ec priv: %w", err)
	}
	privBytes := ecPriv.Serialize()

	ecdsaKey, err := ethcrypto.ToECDSA(privBytes)
	if err != nil {
		return "", "", fmt.Errorf("to ecdsa: %w", err)
	}
	addr := ethcrypto.PubkeyToAddress(ecdsaKey.PublicKey)

	return "0x" + hex.EncodeToString(privBytes), addr.Hex(), nil
}

// newMnemonic generates a fresh 12-word BIP-39 mnemonic.
func newMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(128) // 128 bits → 12 words
	if err != nil {
		return "", fmt.Errorf("entropy: %w", err)
	}
	m, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("mnemonic: %w", err)
	}
	return m, nil
}
