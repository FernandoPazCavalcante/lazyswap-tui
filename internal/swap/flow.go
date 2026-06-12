package swap

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/applog"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/balance"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/dex"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/thorchain"
)

// btcToken is the pseudo-token shown in the ToToken slot of a THORchain quote.
var btcToken = TokenInfo{Symbol: "BTC", Address: "bitcoin", Decimals: 8}

// TokenInfo identifies a token in the swap flow. Address may be NativeSentinel.
type TokenInfo struct {
	Symbol   string
	Address  string
	Decimals uint8
}

// FlowQuote is the full USD-denominated quote returned by Flow.Quote.
type FlowQuote struct {
	FromToken TokenInfo
	ToToken   TokenInfo

	USDAmount          string // raw user input, e.g. "50"
	USDAmountFormatted string // "$50.00"

	FromTokenAmount    string // gross from-token amount (pre-fee)
	FromTokenPriceLine string // "@ $600.12/BNB"

	EstimatedOutput string // raw DEX quote for the net amount
	MinOutput       string // estimated * (1 - slippage/100)

	Slippage      float64
	NeedsApproval bool

	FeePercent         float64
	FeeAmount          string // formatted, e.g. "0.000125"
	NetFromTokenAmount string // gross - fee, formatted

	// THORchain-specific fields, set when IsThorchain is true (EVM → BTC).
	IsThorchain          bool
	ThorEstimatedSeconds int    // settlement estimate in seconds
	ThorFees             string // total fees in BTC, formatted
	BTCAddress           string // destination Bitcoin address (empty for estimate)
	ThorMemo             string // encoded THORchain memo (empty for estimate)
	EstimatedOutputSats  int64  // expected BTC output in sats (1e8 base units)
}

// FlowResult is the outcome of Flow.Execute.
type FlowResult struct {
	Success      bool
	TxHash       string
	FromToken    string
	ToToken      string
	InputAmount  string // formatted as "$<usd>"
	OutputAmount string
	GasUsed      string
	Err          string
}

// UsdConversion is the output of Flow.ConvertUsdToTokenAmount.
type UsdConversion struct {
	TokenAmount   string
	PricePerToken float64
}

// Flow orchestrates USD→token conversion, quoting, fee application, approval
// checks, and execution. Mirrors swap-flow.service.ts (THORchain excluded).
type Flow struct {
	chainKey string
	chain    chain.Config
	client   *ethclient.Client
}

// NewFlow dials the chain RPC and returns a configured Flow.
func NewFlow(chainKey string) (*Flow, error) {
	c := chain.Get(chainKey)
	client, err := ethclient.Dial(c.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", c.RPCURL, err)
	}
	return &Flow{chainKey: chainKey, chain: c, client: client}, nil
}

// Close releases the underlying RPC client.
func (f *Flow) Close() { f.client.Close() }

// NativeToken returns the SwapTokenInfo for the chain's gas token.
func (f *Flow) NativeToken() TokenInfo {
	return TokenInfo{
		Symbol:   f.chain.NativeSymbol,
		Address:  NativeSentinel,
		Decimals: f.chain.NativeDecimals,
	}
}

// ResolveAddress swaps the NativeSentinel for the chain's wrapped-native
// address; otherwise returns addr unchanged.
func (f *Flow) ResolveAddress(addr string) string {
	if addr == NativeSentinel {
		return f.chain.WrappedNative
	}
	return addr
}

// ConvertUsdToTokenAmount fetches a fresh DEX-derived price for `token` and
// returns how much of it the given USD buys, plus the implied per-token price.
// Tries direct stablecoin→token first, falls back to stablecoin→wrappedNative→token.
func (f *Flow) ConvertUsdToTokenAmount(ctx context.Context, token TokenInfo, usd float64) (UsdConversion, error) {
	stable := f.chain.StablecoinAddr
	stableDecimals := f.stablecoinDecimals()
	tokenAddr := f.ResolveAddress(token.Address)

	amountIn, err := balance.ParseUnits(strconv.FormatFloat(usd, 'f', int(stableDecimals), 64), stableDecimals)
	if err != nil {
		return UsdConversion{}, fmt.Errorf("parse usd: %w", err)
	}

	router := common.HexToAddress(f.chain.RouterAddress)
	contract := bind.NewBoundContract(router, chain.RouterABI, f.client, nil, nil)

	var tokenAmountRaw *big.Int

	// Try direct route: stable → token.
	if !strings.EqualFold(tokenAddr, stable) {
		if raw, err := callAmountsOut(ctx, contract, amountIn, []common.Address{
			common.HexToAddress(stable),
			common.HexToAddress(tokenAddr),
		}); err == nil {
			tokenAmountRaw = raw
		}
	}

	// Fallback: stable → wrappedNative → token.
	if tokenAmountRaw == nil && !strings.EqualFold(tokenAddr, f.chain.WrappedNative) {
		raw, err := callAmountsOut(ctx, contract, amountIn, []common.Address{
			common.HexToAddress(stable),
			common.HexToAddress(f.chain.WrappedNative),
			common.HexToAddress(tokenAddr),
		})
		if err == nil {
			tokenAmountRaw = raw
		}
	}

	// Last resort (token IS wrappedNative): stable → wrappedNative direct.
	if tokenAmountRaw == nil {
		raw, err := callAmountsOut(ctx, contract, amountIn, []common.Address{
			common.HexToAddress(stable),
			common.HexToAddress(tokenAddr),
		})
		if err == nil {
			tokenAmountRaw = raw
		}
	}

	if tokenAmountRaw == nil || tokenAmountRaw.Sign() == 0 {
		return UsdConversion{}, fmt.Errorf("cannot price %s — no liquidity path from stablecoin", token.Symbol)
	}

	tokenAmountStr := balance.FormatUnits(tokenAmountRaw, token.Decimals)
	tokenAmountF, _ := strconv.ParseFloat(tokenAmountStr, 64)
	pricePerToken := 0.0
	if tokenAmountF != 0 {
		pricePerToken = usd / tokenAmountF
	}
	return UsdConversion{TokenAmount: tokenAmountStr, PricePerToken: pricePerToken}, nil
}

// Quote fetches a full USD-denominated swap quote for UI preview.
func (f *Flow) Quote(
	ctx context.Context,
	fromToken, toToken TokenInfo,
	usdAmount string,
	slippage float64,
	walletAddress string,
) (FlowQuote, error) {
	usd, err := strconv.ParseFloat(usdAmount, 64)
	if err != nil || usd <= 0 {
		return FlowQuote{}, errors.New("USD amount must be a positive number")
	}

	conv, err := f.ConvertUsdToTokenAmount(ctx, fromToken, usd)
	if err != nil {
		return FlowQuote{}, err
	}

	gross, _ := strconv.ParseFloat(conv.TokenAmount, 64)
	fee := CalcFee(gross)
	netFromTokenAmount := strconv.FormatFloat(fee.NetAmount, 'f', 6, 64)

	fromAddr := f.ResolveAddress(fromToken.Address)
	toAddr := f.ResolveAddress(toToken.Address)

	provider := dex.NewUniswapV2(f.client, f.chainKey)
	quoteSvc := NewQuote(provider)
	dexQuote, err := quoteSvc.Get(ctx, fromAddr, toAddr, netFromTokenAmount)
	if err != nil {
		return FlowQuote{}, err
	}

	estimated, _ := strconv.ParseFloat(dexQuote.EstimatedOutput, 64)
	minOutput := strconv.FormatFloat(estimated*(1-slippage/100), 'f', 6, 64)

	needsApproval := false
	if fromToken.Address != NativeSentinel {
		needsApproval = f.checkNeedsApproval(ctx, fromAddr, walletAddress, fromToken.Decimals, conv.TokenAmount)
	}

	return FlowQuote{
		FromToken:          fromToken,
		ToToken:            toToken,
		USDAmount:          usdAmount,
		USDAmountFormatted: FormatUSD(usd),
		FromTokenAmount:    conv.TokenAmount,
		FromTokenPriceLine: fmt.Sprintf("@ %s/%s", FormatUSD(conv.PricePerToken), fromToken.Symbol),
		EstimatedOutput:    dexQuote.EstimatedOutput,
		MinOutput:          minOutput,
		Slippage:           slippage,
		NeedsApproval:      needsApproval,
		FeePercent:         fee.FeePercent,
		FeeAmount:          strconv.FormatFloat(fee.FeeAmount, 'f', 6, 64),
		NetFromTokenAmount: netFromTokenAmount,
	}, nil
}

// Execute re-converts the USD amount to a fresh on-chain price, applies the
// LazySwap fee, requotes for minOutput, and dispatches the appropriate swap.
func (f *Flow) Execute(
	ctx context.Context,
	privateKey string,
	fromToken, toToken TokenInfo,
	usdAmount string,
	slippage float64,
) FlowResult {
	applog.Tracef("swap.Flow.Execute — %s → %s $%s slippage=%g%%",
		fromToken.Symbol, toToken.Symbol, usdAmount, slippage)

	usd, err := strconv.ParseFloat(usdAmount, 64)
	if err != nil || usd <= 0 {
		return failResult(fromToken, toToken, usdAmount, "USD amount must be a positive number")
	}

	executor, err := NewExecutor(f.client, privateKey, f.chainKey)
	if err != nil {
		return failResult(fromToken, toToken, usdAmount, err.Error())
	}

	conv, err := f.ConvertUsdToTokenAmount(ctx, fromToken, usd)
	if err != nil {
		return failResult(fromToken, toToken, usdAmount, err.Error())
	}

	gross, _ := strconv.ParseFloat(conv.TokenAmount, 64)
	fee := CalcFee(gross)
	netFromTokenAmount := strconv.FormatFloat(fee.NetAmount, 'f', 6, 64)

	fromAddr := f.ResolveAddress(fromToken.Address)
	toAddr := f.ResolveAddress(toToken.Address)

	provider := dex.NewUniswapV2(f.client, f.chainKey)
	quoteSvc := NewQuote(provider)
	dexQuote, err := quoteSvc.Get(ctx, fromAddr, toAddr, netFromTokenAmount)
	if err != nil {
		return failResult(fromToken, toToken, usdAmount, err.Error())
	}
	estimated, _ := strconv.ParseFloat(dexQuote.EstimatedOutput, 64)
	minOutput := strconv.FormatFloat(estimated*(1-slippage/100), 'f', 6, 64)

	result, err := executor.ExecuteFullSwap(ctx, fromToken.Address, toToken.Address, netFromTokenAmount, minOutput, slippage)
	if err != nil {
		applog.Error("swap.Flow.Execute failed", err)
		return failResult(fromToken, toToken, usdAmount, err.Error())
	}

	applog.Infof("swap executed — txHash=%s in=%s out=%s gas=%s",
		result.TxHash, result.InputAmount, result.OutputAmount, result.GasUsed)

	return FlowResult{
		Success:      true,
		TxHash:       result.TxHash,
		FromToken:    fromToken.Symbol,
		ToToken:      toToken.Symbol,
		InputAmount:  fmt.Sprintf("$%s", usdAmount),
		OutputAmount: result.OutputAmount,
		GasUsed:      result.GasUsed,
	}
}

// ── THORchain cross-chain (EVM → BTC) ────────────────────────────────────────

// GetThorchainQuote converts USD → from-token via the DEX, applies the LazySwap
// fee, then asks Midgard how much BTC the net amount yields. Mirrors
// swap-flow.service.ts getThorchainQuote.
func (f *Flow) GetThorchainQuote(
	ctx context.Context,
	fromToken TokenInfo,
	usdAmount, btcAddress string,
) (FlowQuote, error) {
	usd, err := strconv.ParseFloat(usdAmount, 64)
	if err != nil || usd <= 0 {
		return FlowQuote{}, errors.New("USD amount must be a positive number")
	}
	if strings.TrimSpace(btcAddress) == "" {
		return FlowQuote{}, errors.New("Bitcoin address is required")
	}

	conv, err := f.ConvertUsdToTokenAmount(ctx, fromToken, usd)
	if err != nil {
		return FlowQuote{}, err
	}
	gross, _ := strconv.ParseFloat(conv.TokenAmount, 64)
	fee := CalcFee(gross)
	netFromTokenAmount := strconv.FormatFloat(fee.NetAmount, 'f', 6, 64)

	fromAsset, err := f.thorAssetString(fromToken)
	if err != nil {
		return FlowQuote{}, err
	}
	thorAmount, err := thorchain.ToThorBaseUnits(netFromTokenAmount)
	if err != nil {
		return FlowQuote{}, err
	}

	quote, err := thorchain.NewProvider().GetSwapQuote(ctx, fromAsset, thorAmount, btcAddress)
	if err != nil {
		return FlowQuote{}, err
	}

	return FlowQuote{
		FromToken:            fromToken,
		ToToken:              btcToken,
		USDAmount:            usdAmount,
		USDAmountFormatted:   FormatUSD(usd),
		FromTokenAmount:      conv.TokenAmount,
		FromTokenPriceLine:   fmt.Sprintf("@ %s/%s", FormatUSD(conv.PricePerToken), fromToken.Symbol),
		EstimatedOutput:      quote.ExpectedOutput,
		MinOutput:            quote.MinOutput,
		Slippage:             0, // THORchain enforces slippage via the memo min-output
		NeedsApproval:        fromToken.Address != NativeSentinel,
		FeePercent:           fee.FeePercent,
		FeeAmount:            strconv.FormatFloat(fee.FeeAmount, 'f', 6, 64),
		NetFromTokenAmount:   netFromTokenAmount,
		IsThorchain:          true,
		ThorEstimatedSeconds: quote.EstimatedSeconds,
		ThorFees:             quote.TotalFees,
		BTCAddress:           btcAddress,
		ThorMemo:             quote.Memo,
		EstimatedOutputSats:  quote.ExpectedSats,
	}, nil
}

// EstimateThorchain returns a destination-less price preview for fromToken →
// BTC: USD → token (DEX) + LazySwap fee, then a THORchain quote with no memo.
// Used to show "≈ X BTC (Y sats)" as soon as the user enters a USD amount,
// before they type a Bitcoin address.
func (f *Flow) EstimateThorchain(
	ctx context.Context,
	fromToken TokenInfo,
	usdAmount string,
) (FlowQuote, error) {
	usd, err := strconv.ParseFloat(usdAmount, 64)
	if err != nil || usd <= 0 {
		return FlowQuote{}, errors.New("USD amount must be a positive number")
	}

	conv, err := f.ConvertUsdToTokenAmount(ctx, fromToken, usd)
	if err != nil {
		return FlowQuote{}, err
	}
	gross, _ := strconv.ParseFloat(conv.TokenAmount, 64)
	fee := CalcFee(gross)
	netFromTokenAmount := strconv.FormatFloat(fee.NetAmount, 'f', 6, 64)

	fromAsset, err := f.thorAssetString(fromToken)
	if err != nil {
		return FlowQuote{}, err
	}
	thorAmount, err := thorchain.ToThorBaseUnits(netFromTokenAmount)
	if err != nil {
		return FlowQuote{}, err
	}

	quote, err := thorchain.NewProvider().GetSwapQuote(ctx, fromAsset, thorAmount, "")
	if err != nil {
		return FlowQuote{}, err
	}

	return FlowQuote{
		FromToken:            fromToken,
		ToToken:              btcToken,
		USDAmount:            usdAmount,
		USDAmountFormatted:   FormatUSD(usd),
		FromTokenAmount:      conv.TokenAmount,
		FromTokenPriceLine:   fmt.Sprintf("@ %s/%s", FormatUSD(conv.PricePerToken), fromToken.Symbol),
		EstimatedOutput:      quote.ExpectedOutput,
		MinOutput:            quote.MinOutput,
		NeedsApproval:        fromToken.Address != NativeSentinel,
		FeePercent:           fee.FeePercent,
		FeeAmount:            strconv.FormatFloat(fee.FeeAmount, 'f', 6, 64),
		NetFromTokenAmount:   netFromTokenAmount,
		IsThorchain:          true,
		ThorEstimatedSeconds: quote.EstimatedSeconds,
		ThorFees:             quote.TotalFees,
		EstimatedOutputSats:  quote.ExpectedSats,
	}, nil
}

// ExecuteThorchain re-quotes at execution time (fresh price + memo), fetches the
// current inbound vault, and sends the EVM transaction. Mirrors
// swap-flow.service.ts executeThorchain.
func (f *Flow) ExecuteThorchain(
	ctx context.Context,
	privateKey string,
	fromToken TokenInfo,
	usdAmount, btcAddress string,
) FlowResult {
	applog.Tracef("swap.Flow.ExecuteThorchain — %s → BTC $%s btc=%s",
		fromToken.Symbol, usdAmount, btcAddress)

	usd, err := strconv.ParseFloat(usdAmount, 64)
	if err != nil || usd <= 0 {
		return failResult(fromToken, btcToken, usdAmount, "USD amount must be a positive number")
	}
	if strings.TrimSpace(btcAddress) == "" {
		return failResult(fromToken, btcToken, usdAmount, "Bitcoin address is required")
	}

	executor, err := thorchain.NewExecutor(f.client, privateKey, f.chainKey)
	if err != nil {
		return failResult(fromToken, btcToken, usdAmount, err.Error())
	}

	conv, err := f.ConvertUsdToTokenAmount(ctx, fromToken, usd)
	if err != nil {
		return failResult(fromToken, btcToken, usdAmount, err.Error())
	}
	gross, _ := strconv.ParseFloat(conv.TokenAmount, 64)
	fee := CalcFee(gross)
	netFromTokenAmount := strconv.FormatFloat(fee.NetAmount, 'f', 6, 64)

	fromAsset, err := f.thorAssetString(fromToken)
	if err != nil {
		return failResult(fromToken, btcToken, usdAmount, err.Error())
	}
	thorAmount, err := thorchain.ToThorBaseUnits(netFromTokenAmount)
	if err != nil {
		return failResult(fromToken, btcToken, usdAmount, err.Error())
	}

	provider := thorchain.NewProvider()
	quote, err := provider.GetSwapQuote(ctx, fromAsset, thorAmount, btcAddress)
	if err != nil {
		return failResult(fromToken, btcToken, usdAmount, err.Error())
	}
	// Always fetch a fresh inbound vault — THORchain rotates them.
	inbound, err := provider.GetInboundAddress(ctx, f.chainKey)
	if err != nil {
		return failResult(fromToken, btcToken, usdAmount, err.Error())
	}

	var res thorchain.Result
	if fromToken.Address == NativeSentinel {
		res, err = executor.SwapNativeToBtc(ctx, inbound.VaultAddress, netFromTokenAmount, quote.Memo)
	} else {
		routerAddress := inbound.Router
		if routerAddress == "" {
			routerAddress = f.chain.RouterAddress
		}
		res, err = executor.SwapErc20ToBtc(ctx, routerAddress, inbound.VaultAddress,
			fromToken.Address, fromToken.Symbol, fromToken.Decimals, netFromTokenAmount, quote.Memo)
	}
	if err != nil {
		applog.Error("swap.Flow.ExecuteThorchain failed", err)
		return failResult(fromToken, btcToken, usdAmount, err.Error())
	}

	applog.Infof("thorchain swap executed — txHash=%s expectedBtc=%s gas=%s",
		res.TxHash, quote.ExpectedOutput, res.GasUsed)

	return FlowResult{
		Success:      true,
		TxHash:       res.TxHash,
		FromToken:    fromToken.Symbol,
		ToToken:      "BTC",
		InputAmount:  fmt.Sprintf("$%s", usdAmount),
		OutputAmount: quote.ExpectedOutput,
		GasUsed:      res.GasUsed,
	}
}

// thorAssetString builds the THORchain asset identifier for fromToken.
func (f *Flow) thorAssetString(fromToken TokenInfo) (string, error) {
	if fromToken.Address == NativeSentinel {
		return thorchain.BuildNativeAssetString(f.chainKey, fromToken.Symbol)
	}
	return thorchain.BuildErc20AssetString(f.chainKey, fromToken.Symbol, fromToken.Address)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func (f *Flow) stablecoinDecimals() uint8 {
	lower := strings.ToLower(f.chain.StablecoinAddr)
	for _, t := range f.chain.Tokens {
		if strings.ToLower(t.Address) == lower {
			return t.Decimals
		}
	}
	return 18
}

func (f *Flow) checkNeedsApproval(ctx context.Context, tokenAddr, walletAddr string, decimals uint8, requiredAmount string) bool {
	if !common.IsHexAddress(walletAddr) {
		return true
	}
	contract := bind.NewBoundContract(common.HexToAddress(tokenAddr), chain.ERC20ABI, f.client, nil, nil)
	var out []interface{}
	err := contract.Call(&bind.CallOpts{Context: ctx}, &out, "allowance",
		common.HexToAddress(walletAddr), common.HexToAddress(f.chain.RouterAddress))
	if err != nil || len(out) == 0 {
		return true
	}
	raw, ok := out[0].(*big.Int)
	if !ok {
		return true
	}
	current, _ := strconv.ParseFloat(balance.FormatUnits(raw, decimals), 64)
	want, _ := strconv.ParseFloat(requiredAmount, 64)
	return current < want
}

func callAmountsOut(ctx context.Context, contract *bind.BoundContract, amountIn *big.Int, path []common.Address) (*big.Int, error) {
	var out []interface{}
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &out, "getAmountsOut", amountIn, path); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("empty result")
	}
	amounts, ok := out[0].([]*big.Int)
	if !ok || len(amounts) == 0 {
		return nil, errors.New("unexpected amounts type")
	}
	return amounts[len(amounts)-1], nil
}

func failResult(from, to TokenInfo, usdAmount, msg string) FlowResult {
	return FlowResult{
		Success:      false,
		Err:          msg,
		FromToken:    from.Symbol,
		ToToken:      to.Symbol,
		InputAmount:  fmt.Sprintf("$%s", usdAmount),
		OutputAmount: "0",
	}
}

// ── pure helpers (exported for UI / tests) ──────────────────────────────────

// ApplySlippage returns estimated * (1 - slip/100) as a 6-decimal string.
// Returns "0" for unparseable inputs (matches the Bun reference).
func ApplySlippage(estimated string, slippagePercent float64) string {
	v, err := strconv.ParseFloat(estimated, 64)
	if err != nil {
		return "0"
	}
	return strconv.FormatFloat(v*(1-slippagePercent/100), 'f', 6, 64)
}

// FormatUSD renders amount as "$1,234.56". Reuses the balance package helper.
func FormatUSD(amount float64) string { return balance.FormatUSD(amount) }

// FormatSwapResult renders a status-bar line for the UI.
func FormatSwapResult(r FlowResult) string {
	if !r.Success {
		msg := r.Err
		if msg == "" {
			msg = "unknown error"
		}
		return fmt.Sprintf("Swap failed: %s", msg)
	}
	shortHash := r.TxHash
	if len(shortHash) > 10 {
		shortHash = shortHash[:10]
	}
	return fmt.Sprintf("Swap complete — %s → %s %s  TX: %s...",
		r.InputAmount, r.OutputAmount, r.ToToken, shortHash)
}
