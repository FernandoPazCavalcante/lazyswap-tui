// Package explorer queries Etherscan / BscScan-style APIs to discover the
// ERC-20 tokens a wallet has ever interacted with.
//
// Mirrors src/blockchain/explorer/explorer-api.ts. Only token discovery is
// implemented; transfer-history fetching lands when the activity panel is
// ported. All failures degrade gracefully to an empty slice so callers can
// continue with the chain-config preconfigured tokens.
package explorer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/applog"
	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/chain"
)

// TokenDiscovery is one unique ERC-20 contract the wallet has touched.
type TokenDiscovery struct {
	ContractAddress string
	Symbol          string
	Name            string
	Decimals        uint8
}

// rawTransfer mirrors the JSON shape returned by the explorer tokentx endpoint.
type rawTransfer struct {
	ContractAddress string `json:"contractAddress"`
	TokenSymbol     string `json:"tokenSymbol"`
	TokenName       string `json:"tokenName"`
	TokenDecimal    string `json:"tokenDecimal"`
}

type apiResponse struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

// Client is a thin HTTP wrapper. Use NewClient for the production default
// (10s timeout) or build one with a custom *http.Client for tests.
type Client struct {
	http *http.Client
}

// NewClient returns a Client with a 10s HTTP timeout.
func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 10 * time.Second}}
}

// WithHTTP swaps the underlying HTTP client (used by tests against httptest).
func (c *Client) WithHTTP(h *http.Client) *Client {
	c.http = h
	return c
}

// DiscoverTokens returns the deduplicated list of ERC-20 contracts the wallet
// has interacted with on the given chain. Returns ([], nil) on any non-fatal
// failure (network, parse, API error) so the caller can fall back to the
// chain-config token list. apiKey is required by all major explorers.
func (c *Client) DiscoverTokens(ctx context.Context, walletAddress, apiKey, chainKey string) ([]TokenDiscovery, error) {
	cfg := chain.Get(chainKey)
	if cfg.ExplorerAPIURL == "" {
		return nil, nil
	}

	endpoint := buildTokenTxURL(cfg.ExplorerAPIURL, walletAddress, apiKey, 0)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		applog.Errorf("explorer: network error — %v", err)
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		applog.Errorf("explorer: HTTP %d from %s", resp.StatusCode, cfg.ExplorerAPIURL)
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		applog.Errorf("explorer: read body — %v", err)
		return nil, nil
	}

	var parsed apiResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		applog.Errorf("explorer: parse JSON — %v", err)
		return nil, nil
	}

	if parsed.Status != "1" {
		// "No transactions found" is normal for new wallets — don't spam logs.
		if parsed.Message != "No transactions found" {
			applog.Warnf("explorer: API error — %s", parsed.Message)
		}
		return nil, nil
	}

	var transfers []rawTransfer
	if err := json.Unmarshal(parsed.Result, &transfers); err != nil {
		applog.Errorf("explorer: parse result array — %v", err)
		return nil, nil
	}

	seen := make(map[string]bool, len(transfers))
	out := make([]TokenDiscovery, 0, len(transfers))
	for _, t := range transfers {
		key := strings.ToLower(t.ContractAddress)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, TokenDiscovery{
			ContractAddress: t.ContractAddress,
			Symbol:          t.TokenSymbol,
			Name:            t.TokenName,
			Decimals:        parseDecimals(t.TokenDecimal),
		})
	}
	return out, nil
}

// buildTokenTxURL constructs the Etherscan-family tokentx URL.
func buildTokenTxURL(base, walletAddress, apiKey string, startBlock int) string {
	q := url.Values{}
	q.Set("module", "account")
	q.Set("action", "tokentx")
	q.Set("address", walletAddress)
	q.Set("startblock", fmt.Sprintf("%d", startBlock))
	q.Set("endblock", "99999999")
	q.Set("sort", "asc")
	q.Set("apikey", apiKey)
	return base + "?" + q.Encode()
}

func parseDecimals(s string) uint8 {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil || v < 0 || v > 255 {
		return 18
	}
	return uint8(v)
}
