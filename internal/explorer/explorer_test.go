package explorer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
)

// stubServer returns an httptest server replying with body for every request,
// and a *Client pointed at it (after overriding the chain's ExplorerAPIURL).
func stubServer(t *testing.T, status int, body string) (*Client, string, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	c := NewClient().WithHTTP(srv.Client())

	// Temporarily rewrite the bsc chain's explorer URL to the stub.
	orig := chain.CHAINS["bsc"]
	patched := orig
	patched.ExplorerAPIURL = srv.URL
	chain.CHAINS["bsc"] = patched

	cleanup := func() {
		chain.CHAINS["bsc"] = orig
		srv.Close()
	}
	return c, srv.URL, cleanup
}

func TestDiscoverTokensHappyPath(t *testing.T) {
	body := `{"status":"1","message":"OK","result":[
		{"contractAddress":"0xAAA","tokenSymbol":"USDT","tokenName":"Tether","tokenDecimal":"18"},
		{"contractAddress":"0xaaa","tokenSymbol":"USDT","tokenName":"Tether","tokenDecimal":"18"},
		{"contractAddress":"0xBBB","tokenSymbol":"CAKE","tokenName":"PancakeSwap","tokenDecimal":"18"}
	]}`
	c, _, cleanup := stubServer(t, 200, body)
	defer cleanup()

	got, err := c.DiscoverTokens(context.Background(), "0xWallet", "key", "bsc")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 unique tokens, got %d (%+v)", len(got), got)
	}
	if got[0].Symbol != "USDT" || got[1].Symbol != "CAKE" {
		t.Errorf("unexpected ordering / symbols: %+v", got)
	}
	if got[0].Decimals != 18 {
		t.Errorf("decimals = %d, want 18", got[0].Decimals)
	}
}

func TestDiscoverTokensNoTransactionsFoundSilent(t *testing.T) {
	body := `{"status":"0","message":"No transactions found","result":[]}`
	c, _, cleanup := stubServer(t, 200, body)
	defer cleanup()

	got, err := c.DiscoverTokens(context.Background(), "0xWallet", "key", "bsc")
	if err != nil || got != nil && len(got) != 0 {
		t.Fatalf("expected ([] / nil, nil), got (%+v, %v)", got, err)
	}
}

func TestDiscoverTokensHTTPErrorReturnsEmpty(t *testing.T) {
	c, _, cleanup := stubServer(t, 500, "boom")
	defer cleanup()

	got, err := c.DiscoverTokens(context.Background(), "0xWallet", "key", "bsc")
	if err != nil {
		t.Fatalf("expected nil err (graceful), got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %+v", got)
	}
}

func TestDiscoverTokensMalformedJSONReturnsEmpty(t *testing.T) {
	c, _, cleanup := stubServer(t, 200, "{not-json")
	defer cleanup()

	got, err := c.DiscoverTokens(context.Background(), "0xWallet", "key", "bsc")
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %+v", got)
	}
}

func TestBuildTokenTxURL(t *testing.T) {
	u := buildTokenTxURL("https://api.example/api", "0xabc", "KEY123", 5)
	for _, want := range []string{
		"module=account",
		"action=tokentx",
		"address=0xabc",
		"startblock=5",
		"endblock=99999999",
		"sort=asc",
		"apikey=KEY123",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("URL missing %q: %s", want, u)
		}
	}
}

func TestParseDecimalsFallback(t *testing.T) {
	cases := map[string]uint8{
		"6":   6,
		"18":  18,
		"":    18,
		"abc": 18,
		"-1":  18,
		"300": 18,
	}
	for in, want := range cases {
		if got := parseDecimals(in); got != want {
			t.Errorf("parseDecimals(%q) = %d, want %d", in, got, want)
		}
	}
}
