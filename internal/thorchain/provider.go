package thorchain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/applog"
)

// SwapQuote is the cross-chain quote returned by the Midgard API.
type SwapQuote struct {
	ExpectedOutput   string // expected BTC output, e.g. "0.00123456"
	ExpectedSats     int64  // expected BTC output in sats (1e8 base units)
	MinOutput        string // minimum BTC after MemoSlippage, formatted
	TotalFees        string // total fees denominated in BTC, formatted
	EstimatedSeconds int    // estimated settlement time
	GasLimit         string // recommended EVM gas limit
	Memo             string // memo to embed in the EVM transaction (empty for estimate-only)
}

// InboundAddress is the active vault for a chain, from THORnode.
type InboundAddress struct {
	VaultAddress string
	Chain        string
	Router       string
}

// Provider talks to the public Midgard + THORnode APIs. The base URLs are
// fields so tests can point them at an httptest server.
type Provider struct {
	MidgardURL string
	NodeURL    string
	client     *http.Client
}

// NewProvider returns a Provider wired to the public endpoints.
func NewProvider() *Provider {
	return &Provider{
		MidgardURL: MidgardURL,
		NodeURL:    NodeURL,
		client:     &http.Client{Timeout: 20 * time.Second},
	}
}

// ─── Raw API shapes ────────────────────────────────────────────────────────────

type midgardQuoteResponse struct {
	ExpectedAmountOut string `json:"expected_amount_out"`
	Fees              struct {
		Total string `json:"total"`
		Asset string `json:"asset"`
	} `json:"fees"`
	OutboundDelaySeconds int    `json:"outbound_delay_seconds"`
	Memo                 string `json:"memo"`
	InboundAddress       string `json:"inbound_address"`
	Error                string `json:"error"`
}

type thorNodeInboundEntry struct {
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Halted  bool   `json:"halted"`
	Router  string `json:"router"`
}

// ─── Quote ───────────────────────────────────────────────────────────────────

// GetSwapQuote asks Midgard how much BTC fromAmount (1e8 base units) of
// fromAsset yields. Mirrors ThorchainProvider.getSwapQuote. When btcAddress is
// empty the quote is estimate-only: the destination is omitted and no memo is
// built (the expected/min output are still returned for a price preview).
func (p *Provider) GetSwapQuote(ctx context.Context, fromAsset, fromAmount, btcAddress string) (SwapQuote, error) {
	q := url.Values{}
	q.Set("from_asset", fromAsset)
	q.Set("to_asset", BTCAsset)
	q.Set("amount", fromAmount)
	if btcAddress != "" {
		q.Set("destination", btcAddress)
	}
	endpoint := p.MidgardURL + "/v2/quote/swap?" + q.Encode()

	applog.Tracef("thorchain.GetSwapQuote — GET %s", endpoint)

	body, err := p.get(ctx, endpoint)
	if err != nil {
		return SwapQuote{}, err
	}

	var data midgardQuoteResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return SwapQuote{}, fmt.Errorf("decode Midgard quote: %w", err)
	}
	if data.Error != "" {
		return SwapQuote{}, fmt.Errorf("Midgard quote error: %s", data.Error)
	}
	if data.ExpectedAmountOut == "" {
		return SwapQuote{}, fmt.Errorf("Midgard returned no expected_amount_out")
	}

	expectedSats, ok := new(big.Int).SetString(data.ExpectedAmountOut, 10)
	if !ok {
		return SwapQuote{}, fmt.Errorf("Midgard expected_amount_out not an integer: %q", data.ExpectedAmountOut)
	}
	expectedF, _ := new(big.Float).SetInt(expectedSats).Float64()
	minSats := int64(math.Floor(expectedF * (1 - MemoSlippage)))

	feeSats := 0.0
	if data.Fees.Total != "" {
		feeSats, _ = strconv.ParseFloat(data.Fees.Total, 64)
	}

	estimatedSeconds := data.OutboundDelaySeconds
	if estimatedSeconds == 0 {
		estimatedSeconds = BTCEstimatedSeconds
	}

	// Memo is only needed to execute (requires a destination). For an
	// estimate-only quote (no btcAddress) we leave it empty.
	memo := data.Memo
	if memo == "" && btcAddress != "" {
		memo = fmt.Sprintf("=:%s:%s:%d", BTCAsset, btcAddress, minSats)
	}

	return SwapQuote{
		ExpectedOutput:   formatSats(expectedF),
		ExpectedSats:     expectedSats.Int64(),
		MinOutput:        formatSats(float64(minSats)),
		TotalFees:        formatSats(feeSats),
		EstimatedSeconds: estimatedSeconds,
		GasLimit:         "300000",
		Memo:             memo,
	}, nil
}

// GetInboundAddress fetches the current vault address for chainKey. THORchain
// rotates vaults periodically — always call this fresh, never cache.
func (p *Provider) GetInboundAddress(ctx context.Context, chainKey string) (InboundAddress, error) {
	thorChain, ok := EVMChainToThor[chainKey]
	if !ok {
		return InboundAddress{}, fmt.Errorf(
			"chain %q is not supported by THORchain (supported: %s)",
			chainKey, strings.Join(supportedChains(), ", "))
	}

	endpoint := p.NodeURL + "/thorchain/inbound_addresses"
	applog.Tracef("thorchain.GetInboundAddress — GET %s", endpoint)

	body, err := p.get(ctx, endpoint)
	if err != nil {
		return InboundAddress{}, err
	}

	var entries []thorNodeInboundEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return InboundAddress{}, fmt.Errorf("decode inbound_addresses: %w", err)
	}

	for _, e := range entries {
		if !strings.EqualFold(e.Chain, thorChain) {
			continue
		}
		if e.Halted {
			return InboundAddress{}, fmt.Errorf(
				"THORchain inbound is halted for %q — cross-chain swaps temporarily unavailable", thorChain)
		}
		return InboundAddress{VaultAddress: e.Address, Chain: e.Chain, Router: e.Router}, nil
	}
	return InboundAddress{}, fmt.Errorf("THORchain has no inbound address for %q (halted or unsupported)", thorChain)
}

// ─── Asset / unit helpers ──────────────────────────────────────────────────────

// BuildNativeAssetString builds the THORchain asset for a native token, e.g.
// ("bsc", "BNB") → "BSC.BNB".
func BuildNativeAssetString(chainKey, symbol string) (string, error) {
	thorChain, ok := EVMChainToThor[chainKey]
	if !ok {
		return "", fmt.Errorf("unsupported chain key for THORchain: %s", chainKey)
	}
	return thorChain + "." + strings.ToUpper(symbol), nil
}

// BuildErc20AssetString builds the THORchain asset for an ERC-20, e.g.
// ("ethereum","USDC","0xA0b8…") → "ETH.USDC-0xA0b8…".
func BuildErc20AssetString(chainKey, symbol, address string) (string, error) {
	thorChain, ok := EVMChainToThor[chainKey]
	if !ok {
		return "", fmt.Errorf("unsupported chain key for THORchain: %s", chainKey)
	}
	return fmt.Sprintf("%s.%s-%s", thorChain, strings.ToUpper(symbol), address), nil
}

// ToThorBaseUnits converts a human-readable amount to THORchain's universal 1e8
// base units, regardless of the source token's decimals.
func ToThorBaseUnits(amount string) (string, error) {
	num, err := strconv.ParseFloat(amount, 64)
	if err != nil || num <= 0 {
		return "", fmt.Errorf("invalid amount for THORchain conversion: %s", amount)
	}
	return strconv.FormatInt(int64(math.Floor(num*1e8)), 10), nil
}

// ─── internals ─────────────────────────────────────────────────────────────────

func (p *Provider) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("THORchain request failed (%d): %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// formatSats renders a sats float as a fixed 8-decimal BTC string.
func formatSats(sats float64) string {
	return strconv.FormatFloat(sats/1e8, 'f', 8, 64)
}

func supportedChains() []string {
	out := make([]string, 0, len(EVMChainToThor))
	for k := range EVMChainToThor {
		out = append(out, k)
	}
	return out
}
