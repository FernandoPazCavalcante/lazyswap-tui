package chain

import "testing"

func TestGetFallsBackToDefault(t *testing.T) {
	if c := Get("nonexistent"); c.ChainID == 0 {
		t.Fatalf("fallback chain has zero chainID")
	}
	if c := Get("bsc"); c.ChainID != 56 {
		t.Fatalf("bsc chainID = %d, want 56", c.ChainID)
	}
}

func TestABIsLoad(t *testing.T) {
	if _, ok := ERC20ABI.Methods["balanceOf"]; !ok {
		t.Fatalf("ERC20 ABI missing balanceOf")
	}
	if _, ok := RouterABI.Methods["getAmountsOut"]; !ok {
		t.Fatalf("Router ABI missing getAmountsOut")
	}
}

func TestEveryChainHasStablecoin(t *testing.T) {
	for key, c := range CHAINS {
		if c.StablecoinAddr == "" {
			t.Errorf("%s: empty stablecoin address", key)
		}
		if c.RouterAddress == "" {
			t.Errorf("%s: empty router address", key)
		}
	}
}

func TestSepoliaConfig(t *testing.T) {
	c, ok := CHAINS["sepolia"]
	if !ok {
		t.Fatal("sepolia chain not registered")
	}
	if c.ChainID != 11155111 {
		t.Errorf("sepolia chainID = %d, want 11155111", c.ChainID)
	}
	if c.NativeSymbol != "ETH" {
		t.Errorf("sepolia native = %q, want ETH", c.NativeSymbol)
	}
	// Pass is not deployed on Sepolia → Buy Pass stays inert there.
	if c.PassAddress != "" {
		t.Errorf("sepolia PassAddress = %q, want empty", c.PassAddress)
	}
	if c.WrappedNative == "" || c.RPCURL == "" {
		t.Error("sepolia missing WrappedNative or RPCURL")
	}
}

// OrderedKeys drives the network-switch cycle; it must stay in sync with CHAINS.
func TestOrderedKeysMatchChains(t *testing.T) {
	if len(OrderedKeys) != len(CHAINS) {
		t.Fatalf("OrderedKeys has %d entries, CHAINS has %d", len(OrderedKeys), len(CHAINS))
	}
	for _, k := range OrderedKeys {
		if _, ok := CHAINS[k]; !ok {
			t.Errorf("OrderedKeys references unknown chain %q", k)
		}
	}
}
