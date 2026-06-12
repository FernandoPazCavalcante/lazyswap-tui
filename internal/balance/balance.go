// Package balance fetches native and ERC-20 token balances and prices them in
// USD via the chain's Uniswap-V2-compatible router.
//
// Mirrors src/blockchain/balance/balance.service.ts. Phase 4 omits the
// explorer-API token discovery fallback — only chain-config pre-registered
// tokens are queried. Discovery lands in a later phase.
package balance

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/explorer"
)

// NativeAddress is the sentinel used to mark the native gas token in the
// TokenBalance struct (matches the Bun reference's "native" string).
const NativeAddress = "native"

// TokenBalance is the unit returned to callers (UI / swap services).
type TokenBalance struct {
	Symbol     string
	Name       string
	Address    string // NativeAddress for the gas token
	Decimals   uint8
	Balance    string   // human-readable, e.g. "1,234.56"
	BalanceRaw *big.Int // raw wei / smallest unit
	USDValue   string   // formatted "$1,234.56", or "" when pricing failed
}

// Service is a long-lived balance fetcher bound to a single chain.
type Service struct {
	chainKey string
	chain    chain.Config
	client   *ethclient.Client
	explorer *explorer.Client
}

// New dials the chain's RPC and returns a configured Service.
func New(chainKey string) (*Service, error) {
	c := chain.Get(chainKey)
	client, err := ethclient.Dial(c.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", c.RPCURL, err)
	}
	return &Service{
		chainKey: chainKey,
		chain:    c,
		client:   client,
		explorer: explorer.NewClient(),
	}, nil
}

// Close releases the underlying RPC connection.
func (s *Service) Close() { s.client.Close() }

// Chain returns the resolved chain config.
func (s *Service) Chain() chain.Config { return s.chain }

// FetchAll queries native + every pre-registered ERC-20 balance, then prices
// each via the router. Empty balances are dropped except for the native token,
// which is always returned (even when zero) so the UI has something to show.
//
// If explorerAPIKey is non-empty, the wallet's ERC-20 transfer history is
// queried via the chain's block explorer to discover tokens beyond the
// preconfigured list. Discovery failures degrade silently to the chain-config
// token set.
func (s *Service) FetchAll(ctx context.Context, walletAddr, explorerAPIKey string) ([]TokenBalance, error) {
	if !common.IsHexAddress(walletAddr) {
		return nil, fmt.Errorf("invalid wallet address: %q", walletAddr)
	}
	addr := common.HexToAddress(walletAddr)

	// 1. Native balance.
	native, err := s.fetchNative(ctx, addr)
	if err != nil {
		// Soft-fail: return zero balance so the UI doesn't crash.
		native = TokenBalance{
			Symbol:     s.chain.NativeSymbol,
			Name:       s.chain.NativeSymbol,
			Address:    NativeAddress,
			Decimals:   s.chain.NativeDecimals,
			Balance:    "0.00",
			BalanceRaw: big.NewInt(0),
		}
	}

	// 2. Build the union of preconfigured + discovered tokens.
	tokenMap := make(map[string]chain.TokenInfo, len(s.chain.Tokens))
	for _, t := range s.chain.Tokens {
		tokenMap[strings.ToLower(t.Address)] = t
	}
	if explorerAPIKey != "" {
		discovered, _ := s.explorer.DiscoverTokens(ctx, walletAddr, explorerAPIKey, s.chainKey)
		for _, d := range discovered {
			key := strings.ToLower(d.ContractAddress)
			if _, exists := tokenMap[key]; exists {
				continue
			}
			tokenMap[key] = chain.TokenInfo{
				Address:  d.ContractAddress,
				Symbol:   d.Symbol,
				Decimals: d.Decimals,
			}
		}
	}

	// 3. ERC-20 balances (parallel).
	results := make([]TokenBalance, 0, 1+len(tokenMap))
	results = append(results, native)

	type erc20Result struct {
		balance TokenBalance
		err     error
	}
	resCh := make(chan erc20Result, len(tokenMap))
	var wg sync.WaitGroup
	for _, t := range tokenMap {
		wg.Add(1)
		go func(t chain.TokenInfo) {
			defer wg.Done()
			tb, err := s.fetchERC20(ctx, common.HexToAddress(t.Address), addr, t.Symbol, t.Symbol, t.Decimals)
			resCh <- erc20Result{balance: tb, err: err}
		}(t)
	}
	wg.Wait()
	close(resCh)

	for r := range resCh {
		if r.err != nil {
			// Drop tokens we couldn't query; matches Bun behavior.
			continue
		}
		if r.balance.BalanceRaw != nil && r.balance.BalanceRaw.Sign() > 0 {
			results = append(results, r.balance)
		}
	}

	// 4. USD pricing (parallel, best-effort).
	s.enrichWithUSD(ctx, results)

	return results, nil
}

// ─── Native balance ──────────────────────────────────────────────────────────

func (s *Service) fetchNative(ctx context.Context, addr common.Address) (TokenBalance, error) {
	bal, err := s.client.BalanceAt(ctx, addr, nil)
	if err != nil {
		return TokenBalance{}, err
	}
	return TokenBalance{
		Symbol:     s.chain.NativeSymbol,
		Name:       s.chain.NativeSymbol,
		Address:    NativeAddress,
		Decimals:   s.chain.NativeDecimals,
		Balance:    FormatBalance(bal, s.chain.NativeDecimals),
		BalanceRaw: bal,
	}, nil
}

// ─── ERC-20 balance ──────────────────────────────────────────────────────────

func (s *Service) fetchERC20(
	ctx context.Context,
	tokenAddr, walletAddr common.Address,
	symbol, name string,
	decimals uint8,
) (TokenBalance, error) {
	contract := bind.NewBoundContract(tokenAddr, chain.ERC20ABI, s.client, nil, nil)

	var out []interface{}
	err := contract.Call(&bind.CallOpts{Context: ctx}, &out, "balanceOf", walletAddr)
	if err != nil {
		return TokenBalance{}, fmt.Errorf("balanceOf %s: %w", symbol, err)
	}
	if len(out) == 0 {
		return TokenBalance{}, errors.New("empty balanceOf result")
	}
	raw, ok := out[0].(*big.Int)
	if !ok {
		return TokenBalance{}, fmt.Errorf("balanceOf %s: unexpected return type %T", symbol, out[0])
	}

	return TokenBalance{
		Symbol:     symbol,
		Name:       name,
		Address:    tokenAddr.Hex(),
		Decimals:   decimals,
		Balance:    FormatBalance(raw, decimals),
		BalanceRaw: raw,
	}, nil
}

// ─── USD pricing ─────────────────────────────────────────────────────────────

func (s *Service) enrichWithUSD(ctx context.Context, tokens []TokenBalance) {
	router := common.HexToAddress(s.chain.RouterAddress)
	stable := strings.ToLower(s.chain.StablecoinAddr)
	stableDecimals := s.findStablecoinDecimals()

	pCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := range tokens {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			tokens[i].USDValue = s.priceOne(pCtx, router, &tokens[i], stable, stableDecimals)
		}()
	}
	wg.Wait()
}

func (s *Service) priceOne(
	ctx context.Context,
	router common.Address,
	t *TokenBalance,
	stable string,
	stableDecimals uint8,
) string {
	if t.BalanceRaw == nil || t.BalanceRaw.Sign() == 0 {
		return FormatUSD(0)
	}

	// Stablecoin self → 1:1.
	if t.Address != NativeAddress && strings.EqualFold(t.Address, stable) {
		f, _ := new(big.Float).SetInt(t.BalanceRaw).Float64()
		divisor := new(big.Float).SetFloat64(pow10(int(t.Decimals)))
		v, _ := new(big.Float).Quo(new(big.Float).SetFloat64(f), divisor).Float64()
		return FormatUSD(v)
	}

	// Resolve the "from" address for the router path. For native, swap into
	// the wrapped variant; the path can't include the bare native sentinel.
	fromAddr := t.Address
	if t.Address == NativeAddress {
		fromAddr = s.chain.WrappedNative
	}
	if strings.EqualFold(fromAddr, stable) {
		// Wrapped-native and stablecoin happen to coincide — fall back to self.
		v, _ := ParseUSD("$" + t.Balance)
		return FormatUSD(v)
	}

	// Probe how many stablecoin units 1 full token unit yields.
	oneToken := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(t.Decimals)), nil)
	path := []common.Address{common.HexToAddress(fromAddr), common.HexToAddress(stable)}

	contract := bind.NewBoundContract(router, chain.RouterABI, s.client, nil, nil)
	var out []interface{}
	err := contract.Call(&bind.CallOpts{Context: ctx}, &out, "getAmountsOut", oneToken, path)
	if err != nil || len(out) == 0 {
		return ""
	}
	amounts, ok := out[0].([]*big.Int)
	if !ok || len(amounts) < 2 {
		return ""
	}

	pricePer, err := bigIntToFloat(amounts[1], stableDecimals)
	if err != nil {
		return ""
	}
	tokenAmt, err := bigIntToFloat(t.BalanceRaw, t.Decimals)
	if err != nil {
		return ""
	}
	return FormatUSD(pricePer * tokenAmt)
}

func (s *Service) findStablecoinDecimals() uint8 {
	lower := strings.ToLower(s.chain.StablecoinAddr)
	for _, t := range s.chain.Tokens {
		if strings.ToLower(t.Address) == lower {
			return t.Decimals
		}
	}
	// Conservative default that matches USDC on Ethereum.
	return 6
}

// ─── numeric helpers ─────────────────────────────────────────────────────────

func bigIntToFloat(v *big.Int, decimals uint8) (float64, error) {
	if v == nil {
		return 0, errors.New("nil bigint")
	}
	str := FormatUnits(v, decimals)
	var f float64
	_, err := fmt.Sscanf(str, "%f", &f)
	if err != nil {
		return 0, err
	}
	return f, nil
}

func pow10(n int) float64 {
	v := 1.0
	for i := 0; i < n; i++ {
		v *= 10
	}
	return v
}
