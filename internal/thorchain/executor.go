package thorchain

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/applog"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/balance"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
)

// routerABIJSON is the minimal THORchain router ABI — only depositWithExpiry,
// used for ERC-20 → BTC swaps.
const routerABIJSON = `[
  {"name":"depositWithExpiry","type":"function","stateMutability":"payable","inputs":[{"name":"vault","type":"address"},{"name":"asset","type":"address"},{"name":"amount","type":"uint256"},{"name":"memo","type":"string"},{"name":"expiry","type":"uint256"}],"outputs":[]}
]`

var routerABI abi.ABI

func init() {
	a, err := abi.JSON(strings.NewReader(routerABIJSON))
	if err != nil {
		panic("parse THORchain router ABI: " + err.Error())
	}
	routerABI = a
}

// Result summarises an executed THORchain swap.
type Result struct {
	TxHash      string
	FromToken   string
	ToToken     string // always "BTC"
	InputAmount string
	Memo        string
	GasUsed     string
}

// Executor signs and broadcasts the EVM transaction that initiates a THORchain
// swap. Mirrors thorchain-executor.ts.
type Executor struct {
	chain      chain.Config
	client     *ethclient.Client
	privateKey *ecdsa.PrivateKey
	from       common.Address
	chainID    *big.Int
}

// NewExecutor wires the wallet's private key and the chain's RPC client.
func NewExecutor(client *ethclient.Client, privateKeyHex, chainKey string) (*Executor, error) {
	pk, err := ethcrypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	pub, ok := pk.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("derive public key: unexpected type")
	}
	c := chain.Get(chainKey)
	return &Executor{
		chain:      c,
		client:     client,
		privateKey: pk,
		from:       ethcrypto.PubkeyToAddress(*pub),
		chainID:    new(big.Int).SetUint64(c.ChainID),
	}, nil
}

// SwapNativeToBtc sends a native-token (ETH / BNB) → BTC swap: a plain value
// transfer to the vault with the swap memo carried as UTF-8 bytes in tx.data.
func (e *Executor) SwapNativeToBtc(ctx context.Context, vaultAddress, nativeAmount, memo string) (Result, error) {
	if err := validate(vaultAddress, nativeAmount, memo); err != nil {
		return Result{}, err
	}
	applog.Tracef("thorchain.SwapNativeToBtc — vault=%s amount=%s memo=%s", vaultAddress, nativeAmount, memo)

	value, err := balance.ParseUnits(nativeAmount, 18)
	if err != nil {
		return Result{}, fmt.Errorf("parse native amount: %w", err)
	}
	to := common.HexToAddress(vaultAddress)
	data := []byte(memo)

	nonce, err := e.client.PendingNonceAt(ctx, e.from)
	if err != nil {
		return Result{}, fmt.Errorf("nonce: %w", err)
	}
	gasPrice, err := e.client.SuggestGasPrice(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("gas price: %w", err)
	}
	gasLimit := e.estimateGas(ctx, ethereum.CallMsg{From: e.from, To: &to, Value: value, Data: data})

	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &to,
		Value:    value,
		Gas:      gasLimit,
		GasPrice: gasPrice,
		Data:     data,
	})
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(e.chainID), e.privateKey)
	if err != nil {
		return Result{}, fmt.Errorf("sign tx: %w", err)
	}
	if err := e.client.SendTransaction(ctx, signed); err != nil {
		return Result{}, fmt.Errorf("send tx: %w", err)
	}
	receipt, err := bind.WaitMined(ctx, e.client, signed)
	if err != nil {
		return Result{}, fmt.Errorf("wait mined: %w", err)
	}

	return Result{
		TxHash:      receipt.TxHash.Hex(),
		FromToken:   e.chain.NativeSymbol,
		ToToken:     "BTC",
		InputAmount: nativeAmount,
		Memo:        memo,
		GasUsed:     strconv.FormatUint(receipt.GasUsed, 10),
	}, nil
}

// SwapErc20ToBtc approves the THORchain router then calls depositWithExpiry to
// initiate an ERC-20 → BTC swap.
func (e *Executor) SwapErc20ToBtc(
	ctx context.Context,
	routerAddress, vaultAddress, tokenAddress, tokenSymbol string,
	tokenDecimals uint8,
	tokenAmount, memo string,
) (Result, error) {
	if err := validate(vaultAddress, tokenAmount, memo); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(routerAddress) == "" {
		return Result{}, errors.New("THORchain router address is required")
	}
	applog.Tracef("thorchain.SwapErc20ToBtc — router=%s vault=%s token=%s amount=%s",
		routerAddress, vaultAddress, tokenSymbol, tokenAmount)

	amountWei, err := balance.ParseUnits(tokenAmount, tokenDecimals)
	if err != nil {
		return Result{}, fmt.Errorf("parse token amount: %w", err)
	}

	// Step 1: approve the router to spend the token.
	tokenContract := bind.NewBoundContract(common.HexToAddress(tokenAddress), chain.ERC20ABI, e.client, e.client, e.client)
	approveAuth, err := e.txOpts(ctx)
	if err != nil {
		return Result{}, err
	}
	approveTx, err := tokenContract.Transact(approveAuth, "approve", common.HexToAddress(routerAddress), amountWei)
	if err != nil {
		return Result{}, fmt.Errorf("approve: %w", err)
	}
	if _, err := bind.WaitMined(ctx, e.client, approveTx); err != nil {
		return Result{}, fmt.Errorf("approve wait: %w", err)
	}

	// Step 2: depositWithExpiry on the router.
	router := bind.NewBoundContract(common.HexToAddress(routerAddress), routerABI, e.client, e.client, e.client)
	depositAuth, err := e.txOpts(ctx)
	if err != nil {
		return Result{}, err
	}
	expiry := big.NewInt(time.Now().Unix() + 15*60)
	depositTx, err := router.Transact(depositAuth, "depositWithExpiry",
		common.HexToAddress(vaultAddress),
		common.HexToAddress(tokenAddress),
		amountWei,
		memo,
		expiry,
	)
	if err != nil {
		return Result{}, fmt.Errorf("depositWithExpiry: %w", err)
	}
	receipt, err := bind.WaitMined(ctx, e.client, depositTx)
	if err != nil {
		return Result{}, fmt.Errorf("depositWithExpiry wait: %w", err)
	}

	return Result{
		TxHash:      receipt.TxHash.Hex(),
		FromToken:   tokenSymbol,
		ToToken:     "BTC",
		InputAmount: tokenAmount,
		Memo:        memo,
		GasUsed:     strconv.FormatUint(receipt.GasUsed, 10),
	}, nil
}

// ─── helpers ───────────────────────────────────────────────────────────────────

func (e *Executor) txOpts(ctx context.Context) (*bind.TransactOpts, error) {
	opts, err := bind.NewKeyedTransactorWithChainID(e.privateKey, e.chainID)
	if err != nil {
		return nil, fmt.Errorf("transactor: %w", err)
	}
	opts.Context = ctx
	return opts, nil
}

// estimateGas returns an estimate with a 20% buffer, falling back to a fixed
// limit when the node refuses to estimate (memo-carrying txs sometimes do).
func (e *Executor) estimateGas(ctx context.Context, msg ethereum.CallMsg) uint64 {
	est, err := e.client.EstimateGas(ctx, msg)
	if err != nil || est == 0 {
		return 300000
	}
	return est * 12 / 10
}

func validate(vault, amount, memo string) error {
	if strings.TrimSpace(vault) == "" {
		return errors.New("THORchain vault address is required")
	}
	n, err := strconv.ParseFloat(amount, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("invalid swap amount: %s", amount)
	}
	if strings.TrimSpace(memo) == "" {
		return errors.New("THORchain memo is required")
	}
	return nil
}
