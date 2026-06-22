package pass

import (
	"errors"
	"testing"

	"github.com/FernandoPazCavalcante/lazyswap/internal/chain"
)

func TestNew_NoPassChains(t *testing.T) {
	// Mainnet + Ethereum have no deployed pass yet → ErrNoPass.
	for _, key := range []string{"ethereum", "bsc"} {
		if _, err := New(key); !errors.Is(err, ErrNoPass) {
			t.Errorf("New(%q) err = %v, want ErrNoPass", key, err)
		}
	}
}

func TestNew_TestnetAvailable(t *testing.T) {
	// Dial over HTTP is lazy (no network), so this constructs cleanly.
	s, err := New("bsc_testnet")
	if err != nil {
		t.Fatalf("New(bsc_testnet) err = %v, want nil", err)
	}
	defer s.Close()
	if got := s.NativeSymbol(); got != "tBNB" {
		t.Errorf("NativeSymbol = %q, want tBNB", got)
	}
}

func TestMintPriceWei(t *testing.T) {
	if got := MintPriceWei.String(); got != "10000000000000000" {
		t.Errorf("MintPriceWei = %s, want 10000000000000000 (0.01e18)", got)
	}
}

func TestPassABIHasMethods(t *testing.T) {
	for _, m := range []string{"mint", "hasValidPass", "balanceOf", "tokenOfOwnerByIndex", "expiresAt"} {
		if _, ok := chain.LazySwapPassABI.Methods[m]; !ok {
			t.Errorf("LazySwapPassABI missing method %q", m)
		}
	}
}
