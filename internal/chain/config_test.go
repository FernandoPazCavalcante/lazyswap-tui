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
