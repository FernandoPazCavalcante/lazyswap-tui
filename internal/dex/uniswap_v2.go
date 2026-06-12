// Package dex provides DEX price providers used by the swap quoting flow.
//
// Mirrors src/blockchain/dex/. Currently only UniswapV2-compatible
// (Ethereum Uniswap V2, BSC PancakeSwap, …).
package dex

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/balance"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
)

// UniswapV2Provider quotes via a Uniswap V2-compatible router (getAmountsOut).
type UniswapV2Provider struct {
	chain  chain.Config
	client *ethclient.Client
}

// NewUniswapV2 wraps an existing RPC client + chain config.
func NewUniswapV2(client *ethclient.Client, chainKey string) *UniswapV2Provider {
	return &UniswapV2Provider{chain: chain.Get(chainKey), client: client}
}

// GetPrice returns how much toToken the given inputAmount of fromToken yields,
// as a plain decimal string. Mirrors UniswapV2PriceProvider.getPrice().
func (p *UniswapV2Provider) GetPrice(
	ctx context.Context,
	fromToken, toToken, inputAmount string,
) (string, error) {
	fromDec := p.resolveDecimals(fromToken)
	toDec := p.resolveDecimals(toToken)

	amountIn, err := balance.ParseUnits(inputAmount, fromDec)
	if err != nil {
		return "", fmt.Errorf("parse input amount: %w", err)
	}

	router := common.HexToAddress(p.chain.RouterAddress)
	path := []common.Address{
		common.HexToAddress(fromToken),
		common.HexToAddress(toToken),
	}

	contract := bind.NewBoundContract(router, chain.RouterABI, p.client, nil, nil)
	var out []interface{}
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &out, "getAmountsOut", amountIn, path); err != nil {
		return "", fmt.Errorf("getAmountsOut: %w", err)
	}
	if len(out) == 0 {
		return "", errors.New("getAmountsOut: empty result")
	}
	amounts, ok := out[0].([]*big.Int)
	if !ok || len(amounts) < 2 || amounts[1] == nil {
		return "", errors.New("getAmountsOut: missing output amount")
	}
	return balance.FormatUnits(amounts[1], toDec), nil
}

func (p *UniswapV2Provider) resolveDecimals(addr string) uint8 {
	lower := strings.ToLower(addr)
	for _, t := range p.chain.Tokens {
		if strings.ToLower(t.Address) == lower {
			return t.Decimals
		}
	}
	return 18
}
