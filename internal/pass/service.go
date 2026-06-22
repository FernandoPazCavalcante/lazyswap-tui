// Package pass handles the LazySwapPass ERC-721 access pass: reading a wallet's
// pass validity/expiry and minting a new pass on-chain.
//
// On-chain only — it does not talk to the LazySwap backend. The pass contract
// address per chain lives in chain.Config.PassAddress (single source of truth);
// a chain without an address has no pass and New returns ErrNoPass.
package pass

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/FernandoPazCavalcante/lazyswap/internal/chain"
)

// ErrNoPass means the chain has no LazySwapPass deployment configured.
var ErrNoPass = errors.New("pass: not available on this chain")

// MintPriceWei is the LazySwapPass MINT_PRICE: 0.01 ether (an immutable
// contract constant), i.e. 1e16 wei of the chain's native token.
var MintPriceWei = big.NewInt(10_000_000_000_000_000)

// Status summarises a wallet's pass standing.
type Status struct {
	HasValidPass bool
	// ExpiresAt is the latest expiry across the wallet's passes; zero when the
	// wallet holds none.
	ExpiresAt time.Time
}

// Service is a long-lived pass reader/minter bound to a single chain.
type Service struct {
	chain   chain.Config
	client  *ethclient.Client
	address common.Address
}

// New dials the chain's RPC and returns a Service, or ErrNoPass when the chain
// has no configured pass contract.
func New(chainKey string) (*Service, error) {
	c := chain.Get(chainKey)
	if c.PassAddress == "" {
		return nil, ErrNoPass
	}
	client, err := ethclient.Dial(c.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", c.RPCURL, err)
	}
	return &Service{
		chain:   c,
		client:  client,
		address: common.HexToAddress(c.PassAddress),
	}, nil
}

// Close releases the underlying RPC connection.
func (s *Service) Close() { s.client.Close() }

// NativeSymbol is the gas-token symbol for the service's chain (e.g. tBNB).
func (s *Service) NativeSymbol() string { return s.chain.NativeSymbol }

// Status reads the wallet's passes and returns validity + latest expiry. It
// reads every owned pass (ERC721Enumerable) and keeps the furthest expiry, so a
// renewed wallet shows its longest coverage.
func (s *Service) Status(ctx context.Context, owner string) (Status, error) {
	c := bind.NewBoundContract(s.address, chain.LazySwapPassABI, s.client, nil, nil)
	addr := common.HexToAddress(owner)
	opts := &bind.CallOpts{Context: ctx}

	bal, err := s.callBigInt(c, opts, "balanceOf", addr)
	if err != nil {
		return Status{}, err
	}
	if !bal.IsInt64() {
		return Status{}, fmt.Errorf("balanceOf: value out of range: %s", bal)
	}

	var latest int64
	count := bal.Int64()
	for i := int64(0); i < count; i++ {
		tokenID, err := s.callBigInt(c, opts, "tokenOfOwnerByIndex", addr, big.NewInt(i))
		if err != nil {
			return Status{}, err
		}
		exp, err := s.callBigInt(c, opts, "expiresAt", tokenID)
		if err != nil {
			return Status{}, err
		}
		// A uint256 expiry beyond int64 (year 2262+) is "far future" → valid.
		e := int64(math.MaxInt64)
		if exp.IsInt64() {
			e = exp.Int64()
		}
		if e > latest {
			latest = e
		}
	}

	st := Status{HasValidPass: latest > time.Now().Unix()}
	if latest > 0 {
		st.ExpiresAt = time.Unix(latest, 0)
	}
	return st, nil
}

// Buy mints one pass from the given wallet, paying MintPriceWei, and blocks
// until the tx is mined. privateKeyHex may be 0x-prefixed.
func (s *Service) Buy(ctx context.Context, privateKeyHex string) (string, error) {
	pk, err := ethcrypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	auth, err := bind.NewKeyedTransactorWithChainID(pk, new(big.Int).SetUint64(s.chain.ChainID))
	if err != nil {
		return "", fmt.Errorf("transactor: %w", err)
	}
	auth.Context = ctx
	auth.Value = new(big.Int).Set(MintPriceWei)

	c := bind.NewBoundContract(s.address, chain.LazySwapPassABI, s.client, s.client, s.client)
	tx, err := c.Transact(auth, "mint")
	if err != nil {
		return "", fmt.Errorf("mint: %w", err)
	}
	receipt, err := bind.WaitMined(ctx, s.client, tx)
	if err != nil {
		return "", fmt.Errorf("mint wait: %w", err)
	}
	// WaitMined returns a receipt even for a reverted tx; Status==0 means the
	// mint failed on-chain (e.g. insufficient funds). Surface that as an error.
	if receipt.Status != types.ReceiptStatusSuccessful {
		return receipt.TxHash.Hex(), fmt.Errorf("mint reverted (tx %s)", receipt.TxHash.Hex())
	}
	return receipt.TxHash.Hex(), nil
}

// callBigInt runs a view call returning a single uint256.
func (s *Service) callBigInt(c *bind.BoundContract, opts *bind.CallOpts, method string, args ...interface{}) (*big.Int, error) {
	var out []interface{}
	if err := c.Call(opts, &out, method, args...); err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: empty result", method)
	}
	v, ok := out[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("%s: unexpected return type %T", method, out[0])
	}
	return v, nil
}
