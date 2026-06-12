// Package chain holds EVM chain configuration and contract ABIs.
//
// Mirrors src/blockchain/chain-config.ts. The CHAINS map is the single source
// of truth for RPC URLs, router addresses, and well-known token addresses.
package chain

// TokenInfo identifies a token by its on-chain address + decimals.
type TokenInfo struct {
	Address  string
	Decimals uint8
	Symbol   string
}

// Config is the per-chain configuration.
type Config struct {
	ChainID           uint64
	Name              string
	RPCURL            string
	RouterAddress     string
	WrappedNative     string
	NativeSymbol      string
	NativeDecimals    uint8
	ExplorerAPIURL    string
	StablecoinAddr    string
	Tokens            map[string]TokenInfo
	RecommendedTokens []TokenInfo
}

// DefaultKey is the chain selected when none is otherwise specified.
const DefaultKey = "bsc"

// CHAINS is the registry of supported chains. Keys match the Bun reference.
var CHAINS = map[string]Config{
	"ethereum": {
		ChainID:        1,
		Name:           "Ethereum",
		RPCURL:         "https://ethereum-rpc.publicnode.com",
		RouterAddress:  "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D",
		WrappedNative:  "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
		NativeSymbol:   "ETH",
		NativeDecimals: 18,
		ExplorerAPIURL: "https://api.etherscan.io/api",
		StablecoinAddr: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", // USDC
		Tokens: map[string]TokenInfo{
			"USDC": {Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6, Symbol: "USDC"},
			"WETH": {Address: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", Decimals: 18, Symbol: "WETH"},
			"DAI":  {Address: "0x6B175474E89094C44Da98b954EedeAC495271d0F", Decimals: 18, Symbol: "DAI"},
			"USDT": {Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6, Symbol: "USDT"},
		},
		RecommendedTokens: []TokenInfo{
			{Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6, Symbol: "USDT"},
			{Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6, Symbol: "USDC"},
			{Address: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", Decimals: 18, Symbol: "WETH"},
			{Address: "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599", Decimals: 8, Symbol: "WBTC"},
			{Address: "0x6B175474E89094C44Da98b954EedeAC495271d0F", Decimals: 18, Symbol: "DAI"},
			{Address: "0x514910771AF9Ca656af840dff83E8264EcF986CA", Decimals: 18, Symbol: "LINK"},
			{Address: "0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984", Decimals: 18, Symbol: "UNI"},
			{Address: "0x95aD61b0a150d79219dCF64E1E6Cc01f0B64C4cE", Decimals: 18, Symbol: "SHIB"},
			{Address: "0x6982508145454Ce325dDbE47a25d4ec3d2311933", Decimals: 18, Symbol: "PEPE"},
			{Address: "0x7D1AfA7B718fb893dB30A3aBc0Cfc608AaCfeBB0", Decimals: 18, Symbol: "MATIC"},
			{Address: "0xcf0c122c6b73ff809c693db761e7baebe62b6a2e", Decimals: 9, Symbol: "FLOKI"},
			{Address: "0x4507cEf57C46789eF8d1a19EA45f4216bae2B528", Decimals: 18, Symbol: "TOKEN"},
		},
	},

	"bsc": {
		ChainID:        56,
		Name:           "BSC",
		RPCURL:         "https://bsc-rpc.publicnode.com",
		RouterAddress:  "0x10ED43C718714eb63d5aA57B78B54704E256024E", // PancakeSwap V2
		WrappedNative:  "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c",
		NativeSymbol:   "BNB",
		NativeDecimals: 18,
		ExplorerAPIURL: "https://api.bscscan.com/api",
		StablecoinAddr: "0x55d398326f99059fF775485246999027B3197955", // USDT
		Tokens: map[string]TokenInfo{
			"USDC": {Address: "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d", Decimals: 18, Symbol: "USDC"},
			"WBNB": {Address: "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c", Decimals: 18, Symbol: "WBNB"},
			"BUSD": {Address: "0xe9e7CEA3DedcA5984780Bafc599bD69ADd087D56", Decimals: 18, Symbol: "BUSD"},
			"USDT": {Address: "0x55d398326f99059fF775485246999027B3197955", Decimals: 18, Symbol: "USDT"},
			"CAKE": {Address: "0x0E09FaBB73Bd3Ade0a17ECC321fD13a19e81cE82", Decimals: 18, Symbol: "CAKE"},
		},
		RecommendedTokens: []TokenInfo{
			{Address: "0x55d398326f99059fF775485246999027B3197955", Decimals: 18, Symbol: "USDT"},
			{Address: "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d", Decimals: 18, Symbol: "USDC"},
			{Address: "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c", Decimals: 18, Symbol: "WBNB"},
			{Address: "0x7130d2A12B9BCbFAe4f2634d864A1Ee1Ce3Ead9c", Decimals: 18, Symbol: "BTCB"},
			{Address: "0x2170Ed0880ac9A755fd29B2688956BD959F933F8", Decimals: 18, Symbol: "ETH"},
			{Address: "0x0E09FaBB73Bd3Ade0a17ECC321fD13a19e81cE82", Decimals: 18, Symbol: "CAKE"},
			{Address: "0xbA2aE424d960c26247Dd6c32edC70B295C744C43", Decimals: 8, Symbol: "DOGE"},
			{Address: "0xF8A0BF9cF54Bb92F17374d9e9A321E6a111a51bD", Decimals: 18, Symbol: "LINK"},
			{Address: "0x1D2F0da169ceB9fC7B3144628dB156f3F6c60dBE", Decimals: 18, Symbol: "XRP"},
			{Address: "0x3EE2200Efb3400fAbB9AacF31297cBdD1d435D47", Decimals: 18, Symbol: "ADA"},
			{Address: "0xfb5b838b6cfeedc2873ab27866079ac55363d37e", Decimals: 9, Symbol: "FLOKI"},
			{Address: "0x4507cEf57C46789eF8d1a19EA45f4216bae2B528", Decimals: 18, Symbol: "TOKEN"},
		},
	},

	"bsc_testnet": {
		ChainID:        97,
		Name:           "BSC Testnet",
		RPCURL:         "https://bsc-testnet-rpc.publicnode.com",
		RouterAddress:  "0xD99D1c33F9fC3444f8101754aBC46c52416550D1",
		WrappedNative:  "0xae13d989daC2f0dEbFf460aC112a837C89BAa7cd",
		NativeSymbol:   "tBNB",
		NativeDecimals: 18,
		ExplorerAPIURL: "https://api-testnet.bscscan.com/api",
		StablecoinAddr: "0x337610d27c682E347C9cD60BD4b3b107C9d34dD1",
		Tokens: map[string]TokenInfo{
			"USDC": {Address: "0x64544969ed7EBf5f083679233325356EbE738930", Decimals: 18, Symbol: "USDC"},
			"WBNB": {Address: "0xae13d989daC2f0dEbFf460aC112a837C89BAa7cd", Decimals: 18, Symbol: "WBNB"},
			"USDT": {Address: "0x337610d27c682E347C9cD60BD4b3b107C9d34dD1", Decimals: 18, Symbol: "USDT"},
		},
		RecommendedTokens: []TokenInfo{
			{Address: "0x337610d27c682E347C9cD60BD4b3b107C9d34dD1", Decimals: 18, Symbol: "USDT"},
			{Address: "0x64544969ed7EBf5f083679233325356EbE738930", Decimals: 18, Symbol: "USDC"},
			{Address: "0xae13d989daC2f0dEbFf460aC112a837C89BAa7cd", Decimals: 18, Symbol: "WBNB"},
		},
	},
}

// OrderedKeys lists the chain keys in a stable display order. Go maps iterate
// randomly, so the network-switch cycle relies on this slice instead of ranging
// over CHAINS. Mirrors Object.keys(CHAINS) order in the Bun reference.
var OrderedKeys = []string{"ethereum", "bsc", "bsc_testnet"}

// Get returns the config for the given key, falling back to DefaultKey.
func Get(key string) Config {
	if c, ok := CHAINS[key]; ok {
		return c
	}
	return CHAINS[DefaultKey]
}

// NextKey returns the chain key following current in OrderedKeys, wrapping
// around. Unknown keys fall back to the first entry.
func NextKey(current string) string {
	for i, k := range OrderedKeys {
		if k == current {
			return OrderedKeys[(i+1)%len(OrderedKeys)]
		}
	}
	return OrderedKeys[0]
}
