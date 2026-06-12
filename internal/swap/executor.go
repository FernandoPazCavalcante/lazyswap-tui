package swap

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/balance"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
)

// NativeSentinel marks the chain's gas token in fromToken / toToken fields,
// matching the Bun reference's "native" string.
const NativeSentinel = "native"

// ExecutionResult is the receipt summary returned by every Execute* method.
type ExecutionResult struct {
	TxHash       string
	FromToken    string
	ToToken      string
	InputAmount  string
	OutputAmount string
	GasUsed      string
}

// Executor signs and broadcasts swap + approval txs against a Uniswap V2-like
// router. Mirrors swap-executor.service.ts.
type Executor struct {
	chain      chain.Config
	client     *ethclient.Client
	privateKey *ecdsa.PrivateKey
	from       common.Address
}

// NewExecutor wires the wallet's private key and the chain's RPC.
// privateKeyHex may be 0x-prefixed.
func NewExecutor(client *ethclient.Client, privateKeyHex, chainKey string) (*Executor, error) {
	pk, err := ethcrypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	pub, ok := pk.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("derive public key: unexpected type")
	}
	return &Executor{
		chain:      chain.Get(chainKey),
		client:     client,
		privateKey: pk,
		from:       ethcrypto.PubkeyToAddress(*pub),
	}, nil
}

// Address returns the wallet address derived from the configured private key.
func (e *Executor) Address() common.Address { return e.from }

// CheckAllowance returns the current allowance the wallet has granted to
// spender for tokenAddr, as a human-readable decimal string.
func (e *Executor) CheckAllowance(ctx context.Context, tokenAddr, spender string) (string, error) {
	token := common.HexToAddress(tokenAddr)
	spend := common.HexToAddress(spender)
	contract := bind.NewBoundContract(token, chain.ERC20ABI, e.client, nil, nil)

	var out []interface{}
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &out, "allowance", e.from, spend); err != nil {
		return "", fmt.Errorf("allowance: %w", err)
	}
	if len(out) == 0 {
		return "", errors.New("allowance: empty result")
	}
	raw, ok := out[0].(*big.Int)
	if !ok {
		return "", fmt.Errorf("allowance: unexpected return type %T", out[0])
	}
	return balance.FormatUnits(raw, e.resolveDecimals(tokenAddr)), nil
}

// ApproveToken authorises the chain's router to spend `amount` of tokenAddr,
// blocks until mined, and returns the tx hash.
func (e *Executor) ApproveToken(ctx context.Context, tokenAddr, amount string) (string, error) {
	dec := e.resolveDecimals(tokenAddr)
	raw, err := balance.ParseUnits(amount, dec)
	if err != nil {
		return "", fmt.Errorf("parse amount: %w", err)
	}

	auth, err := e.txOpts(ctx, nil)
	if err != nil {
		return "", err
	}
	contract := bind.NewBoundContract(common.HexToAddress(tokenAddr), chain.ERC20ABI, e.client, e.client, e.client)

	tx, err := contract.Transact(auth, "approve", common.HexToAddress(e.chain.RouterAddress), raw)
	if err != nil {
		return "", fmt.Errorf("approve: %w", err)
	}
	receipt, err := bind.WaitMined(ctx, e.client, tx)
	if err != nil {
		return "", fmt.Errorf("approve wait: %w", err)
	}
	return receipt.TxHash.Hex(), nil
}

// ExecuteSwap performs token→token swap via swapExactTokensForTokens.
// Assumes prior approval — returns an error if allowance is insufficient.
func (e *Executor) ExecuteSwap(
	ctx context.Context,
	fromToken, toToken, inputAmount, minOutputAmount string,
	slippageTolerance float64,
) (ExecutionResult, error) {
	if err := validateSwapInputs(fromToken, toToken, inputAmount, slippageTolerance); err != nil {
		return ExecutionResult{}, err
	}

	current, err := e.CheckAllowance(ctx, fromToken, e.chain.RouterAddress)
	if err != nil {
		return ExecutionResult{}, err
	}
	cur, _ := strconv.ParseFloat(current, 64)
	want, _ := strconv.ParseFloat(inputAmount, 64)
	if cur < want {
		return ExecutionResult{}, fmt.Errorf(
			"insufficient allowance. Current: %s, Required: %s. Please approve token first.",
			current, inputAmount,
		)
	}

	fromDec := e.resolveDecimals(fromToken)
	toDec := e.resolveDecimals(toToken)
	amountIn, err := balance.ParseUnits(inputAmount, fromDec)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("parse inputAmount: %w", err)
	}
	amountOutMin, err := balance.ParseUnits(minOutputAmount, toDec)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("parse minOutputAmount: %w", err)
	}
	path := []common.Address{common.HexToAddress(fromToken), common.HexToAddress(toToken)}
	deadline := big.NewInt(time.Now().Unix() + 20*60)

	auth, err := e.txOpts(ctx, nil)
	if err != nil {
		return ExecutionResult{}, err
	}
	contract := bind.NewBoundContract(common.HexToAddress(e.chain.RouterAddress), chain.RouterABI, e.client, e.client, e.client)

	tx, err := contract.Transact(auth, "swapExactTokensForTokens", amountIn, amountOutMin, path, e.from, deadline)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("swap: %w", err)
	}
	receipt, err := bind.WaitMined(ctx, e.client, tx)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("swap wait: %w", err)
	}

	return ExecutionResult{
		TxHash:       receipt.TxHash.Hex(),
		FromToken:    fromToken,
		ToToken:      toToken,
		InputAmount:  inputAmount,
		OutputAmount: minOutputAmount,
		GasUsed:      strconv.FormatUint(receipt.GasUsed, 10),
	}, nil
}

// ExecuteBuyWithNative buys an ERC-20 with the chain's native gas token via
// swapExactETHForTokens (no approval required).
func (e *Executor) ExecuteBuyWithNative(
	ctx context.Context,
	toToken, nativeAmount, minOutputAmount string,
	slippageTolerance float64,
) (ExecutionResult, error) {
	if strings.TrimSpace(toToken) == "" {
		return ExecutionResult{}, errors.New("invalid toToken address")
	}
	amt, err := strconv.ParseFloat(nativeAmount, 64)
	if err != nil || amt <= 0 {
		return ExecutionResult{}, errors.New("invalid input amount")
	}
	if slippageTolerance < 0 || slippageTolerance > 100 {
		return ExecutionResult{}, errors.New("slippage tolerance must be between 0 and 100")
	}

	toDec := e.resolveDecimals(toToken)
	amountOutMin, err := balance.ParseUnits(minOutputAmount, toDec)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("parse minOutputAmount: %w", err)
	}
	amountInWei, err := balance.ParseUnits(nativeAmount, 18)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("parse nativeAmount: %w", err)
	}
	path := []common.Address{
		common.HexToAddress(e.chain.WrappedNative),
		common.HexToAddress(toToken),
	}
	deadline := big.NewInt(time.Now().Unix() + 20*60)

	auth, err := e.txOpts(ctx, amountInWei)
	if err != nil {
		return ExecutionResult{}, err
	}
	contract := bind.NewBoundContract(common.HexToAddress(e.chain.RouterAddress), chain.RouterABI, e.client, e.client, e.client)

	tx, err := contract.Transact(auth, "swapExactETHForTokens", amountOutMin, path, e.from, deadline)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("swapExactETHForTokens: %w", err)
	}
	receipt, err := bind.WaitMined(ctx, e.client, tx)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("swapExactETHForTokens wait: %w", err)
	}

	return ExecutionResult{
		TxHash:       receipt.TxHash.Hex(),
		FromToken:    NativeSentinel,
		ToToken:      toToken,
		InputAmount:  nativeAmount,
		OutputAmount: minOutputAmount,
		GasUsed:      strconv.FormatUint(receipt.GasUsed, 10),
	}, nil
}

// ExecuteSellForNative sells an ERC-20 for the chain's native gas token via
// swapExactTokensForETH (requires prior approval).
func (e *Executor) ExecuteSellForNative(
	ctx context.Context,
	fromToken, inputAmount, minOutputAmount string,
	slippageTolerance float64,
) (ExecutionResult, error) {
	if strings.TrimSpace(fromToken) == "" {
		return ExecutionResult{}, errors.New("invalid fromToken address")
	}
	amt, err := strconv.ParseFloat(inputAmount, 64)
	if err != nil || amt <= 0 {
		return ExecutionResult{}, errors.New("invalid input amount")
	}
	if slippageTolerance < 0 || slippageTolerance > 100 {
		return ExecutionResult{}, errors.New("slippage tolerance must be between 0 and 100")
	}

	fromDec := e.resolveDecimals(fromToken)
	amountIn, err := balance.ParseUnits(inputAmount, fromDec)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("parse inputAmount: %w", err)
	}
	amountOutMin, err := balance.ParseUnits(minOutputAmount, 18)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("parse minOutputAmount: %w", err)
	}
	path := []common.Address{
		common.HexToAddress(fromToken),
		common.HexToAddress(e.chain.WrappedNative),
	}
	deadline := big.NewInt(time.Now().Unix() + 20*60)

	auth, err := e.txOpts(ctx, nil)
	if err != nil {
		return ExecutionResult{}, err
	}
	contract := bind.NewBoundContract(common.HexToAddress(e.chain.RouterAddress), chain.RouterABI, e.client, e.client, e.client)

	tx, err := contract.Transact(auth, "swapExactTokensForETH", amountIn, amountOutMin, path, e.from, deadline)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("swapExactTokensForETH: %w", err)
	}
	receipt, err := bind.WaitMined(ctx, e.client, tx)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("swapExactTokensForETH wait: %w", err)
	}

	return ExecutionResult{
		TxHash:       receipt.TxHash.Hex(),
		FromToken:    fromToken,
		ToToken:      NativeSentinel,
		InputAmount:  inputAmount,
		OutputAmount: minOutputAmount,
		GasUsed:      strconv.FormatUint(receipt.GasUsed, 10),
	}, nil
}

// ExecuteFullSwap routes to the correct swap variant based on whether either
// side is the native sentinel, handling approvals as needed.
func (e *Executor) ExecuteFullSwap(
	ctx context.Context,
	fromToken, toToken, inputAmount, minOutputAmount string,
	slippageTolerance float64,
) (ExecutionResult, error) {
	if strings.TrimSpace(fromToken) == "" {
		return ExecutionResult{}, errors.New("invalid fromToken address")
	}
	if strings.TrimSpace(toToken) == "" {
		return ExecutionResult{}, errors.New("invalid toToken address")
	}
	amt, err := strconv.ParseFloat(inputAmount, 64)
	if err != nil || amt <= 0 {
		return ExecutionResult{}, errors.New("invalid input amount")
	}
	if slippageTolerance < 0 || slippageTolerance > 100 {
		return ExecutionResult{}, errors.New("slippage tolerance must be between 0 and 100")
	}

	if fromToken == NativeSentinel {
		return e.ExecuteBuyWithNative(ctx, toToken, inputAmount, minOutputAmount, slippageTolerance)
	}

	// ERC-20 source — ensure allowance.
	current, err := e.CheckAllowance(ctx, fromToken, e.chain.RouterAddress)
	if err != nil {
		return ExecutionResult{}, err
	}
	cur, _ := strconv.ParseFloat(current, 64)
	want, _ := strconv.ParseFloat(inputAmount, 64)
	if cur < want {
		if _, err := e.ApproveToken(ctx, fromToken, inputAmount); err != nil {
			return ExecutionResult{}, fmt.Errorf("approve: %w", err)
		}
	}

	if toToken == NativeSentinel {
		return e.ExecuteSellForNative(ctx, fromToken, inputAmount, minOutputAmount, slippageTolerance)
	}
	return e.ExecuteSwap(ctx, fromToken, toToken, inputAmount, minOutputAmount, slippageTolerance)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func (e *Executor) txOpts(ctx context.Context, value *big.Int) (*bind.TransactOpts, error) {
	chainID := new(big.Int).SetUint64(e.chain.ChainID)
	opts, err := bind.NewKeyedTransactorWithChainID(e.privateKey, chainID)
	if err != nil {
		return nil, fmt.Errorf("transactor: %w", err)
	}
	opts.Context = ctx
	if value != nil {
		opts.Value = value
	}
	return opts, nil
}

func (e *Executor) resolveDecimals(addr string) uint8 {
	lower := strings.ToLower(addr)
	for _, t := range e.chain.Tokens {
		if strings.ToLower(t.Address) == lower {
			return t.Decimals
		}
	}
	return 18
}

func validateSwapInputs(fromToken, toToken, inputAmount string, slippageTolerance float64) error {
	if strings.TrimSpace(fromToken) == "" {
		return errors.New("invalid fromToken address")
	}
	if strings.TrimSpace(toToken) == "" {
		return errors.New("invalid toToken address")
	}
	amt, err := strconv.ParseFloat(inputAmount, 64)
	if err != nil || amt <= 0 {
		return errors.New("invalid input amount")
	}
	if slippageTolerance < 0 || slippageTolerance > 100 {
		return errors.New("slippage tolerance must be between 0 and 100")
	}
	return nil
}
