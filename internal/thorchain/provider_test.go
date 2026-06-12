package thorchain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildNativeAssetString(t *testing.T) {
	got, err := BuildNativeAssetString("bsc", "bnb")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "BSC.BNB" {
		t.Fatalf("got %q, want BSC.BNB", got)
	}
	if _, err := BuildNativeAssetString("polygon", "MATIC"); err == nil {
		t.Fatal("expected error for unsupported chain")
	}
}

func TestBuildErc20AssetString(t *testing.T) {
	got, err := BuildErc20AssetString("ethereum", "usdc", "0xA0b8")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "ETH.USDC-0xA0b8" {
		t.Fatalf("got %q, want ETH.USDC-0xA0b8", got)
	}
}

func TestToThorBaseUnits(t *testing.T) {
	got, err := ToThorBaseUnits("0.5")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "50000000" { // 0.5 * 1e8
		t.Fatalf("got %q, want 50000000", got)
	}
	if _, err := ToThorBaseUnits("0"); err == nil {
		t.Fatal("expected error for non-positive amount")
	}
}

func TestGetSwapQuoteParsesAndAppliesSlippage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/quote/swap" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// 1_000_000 sats == 0.01 BTC expected out, no memo provided.
		_, _ = w.Write([]byte(`{"expected_amount_out":"1000000","fees":{"total":"20000"},"outbound_delay_seconds":540}`))
	}))
	defer srv.Close()

	p := NewProvider()
	p.MidgardURL = srv.URL
	q, err := p.GetSwapQuote(context.Background(), "BSC.BNB", "50000000", "bc1qexampleaddressxxxxxxxxxxxxxx")
	if err != nil {
		t.Fatalf("GetSwapQuote: %v", err)
	}
	if q.ExpectedOutput != "0.01000000" {
		t.Fatalf("ExpectedOutput = %q, want 0.01000000", q.ExpectedOutput)
	}
	// 1_000_000 * (1 - 0.03) = 970000 sats = 0.0097 BTC.
	if q.MinOutput != "0.00970000" {
		t.Fatalf("MinOutput = %q, want 0.00970000", q.MinOutput)
	}
	if q.TotalFees != "0.00020000" {
		t.Fatalf("TotalFees = %q, want 0.00020000", q.TotalFees)
	}
	if q.EstimatedSeconds != 540 {
		t.Fatalf("EstimatedSeconds = %d, want 540", q.EstimatedSeconds)
	}
	want := "=:BTC.BTC:bc1qexampleaddressxxxxxxxxxxxxxx:970000"
	if q.Memo != want {
		t.Fatalf("Memo = %q, want %q", q.Memo, want)
	}
}

func TestGetSwapQuoteSurfacesMidgardError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":"trading halted"}`))
	}))
	defer srv.Close()

	p := NewProvider()
	p.MidgardURL = srv.URL
	if _, err := p.GetSwapQuote(context.Background(), "BSC.BNB", "1", "bc1q"); err == nil {
		t.Fatal("expected error from Midgard error field")
	}
}

func TestGetInboundAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"chain":"ETH","address":"0xeth","halted":false,"router":"0xrouter"},
			{"chain":"BSC","address":"0xbsc","halted":false,"router":"0xbscrouter"}
		]`))
	}))
	defer srv.Close()

	p := NewProvider()
	p.NodeURL = srv.URL
	in, err := p.GetInboundAddress(context.Background(), "bsc")
	if err != nil {
		t.Fatalf("GetInboundAddress: %v", err)
	}
	if in.VaultAddress != "0xbsc" || in.Router != "0xbscrouter" {
		t.Fatalf("got vault=%q router=%q", in.VaultAddress, in.Router)
	}
}

func TestGetInboundAddressHalted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"chain":"BSC","address":"0xbsc","halted":true}]`))
	}))
	defer srv.Close()

	p := NewProvider()
	p.NodeURL = srv.URL
	if _, err := p.GetInboundAddress(context.Background(), "bsc"); err == nil {
		t.Fatal("expected halted error")
	}
}
