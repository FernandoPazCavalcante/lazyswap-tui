// Package thorchain implements cross-chain EVM → BTC swaps via THORchain.
//
// Mirrors src/blockchain/thorchain/{thorchain-config,thorchain-provider,
// thorchain-executor}.ts. The provider talks to the public Midgard + THORnode
// APIs for quotes and inbound vault addresses; the executor signs and
// broadcasts the EVM transaction that initiates the swap.
package thorchain

// Public THORchain endpoints. Never hardcode these elsewhere — import from here.
const (
	// MidgardURL serves swap quotes and pool data.
	MidgardURL = "https://midgard.ninerealms.com"
	// NodeURL serves inbound vault addresses.
	NodeURL = "https://thornode.ninerealms.com"

	// BTCAsset is the canonical THORchain asset string for Bitcoin.
	BTCAsset = "BTC.BTC"

	// MemoSlippage is the tolerance applied to the quote when building the swap
	// memo: minOutput = estimated * (1 - MemoSlippage). 0.03 == 3%.
	MemoSlippage = 0.03

	// BTCEstimatedSeconds is the fallback settlement estimate (~10 min) shown in
	// the preview when Midgard does not return outbound_delay_seconds.
	BTCEstimatedSeconds = 600

	// AffiliateBPS is the THORchain affiliate fee in basis points (0 = none).
	AffiliateBPS = 0
)

// EVMChainToThor maps a LazySwap chain key to its THORchain chain identifier.
// Only chains THORchain supports for inbound swaps appear here.
var EVMChainToThor = map[string]string{
	"ethereum": "ETH",
	"bsc":      "BSC",
}
