package thorchain

import (
	"context"
	"errors"
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
		if r.URL.Path != "/thorchain/quote/swap" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// 1_000_000 sats == 0.01 BTC expected out, no memo provided.
		_, _ = w.Write([]byte(`{"expected_amount_out":"1000000","fees":{"total":"20000"},"outbound_delay_seconds":540,"recommended_min_amount_in":"123456"}`))
	}))
	defer srv.Close()

	p := NewProvider()
	p.NodeURL = srv.URL
	q, err := p.GetSwapQuote(context.Background(), "BSC.BNB", "50000000", "bc1qexampleaddressxxxxxxxxxxxxxx")
	if err != nil {
		t.Fatalf("GetSwapQuote: %v", err)
	}
	if q.RecommendedMinIn != 123456 {
		t.Fatalf("RecommendedMinIn = %d, want 123456", q.RecommendedMinIn)
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

func TestGetSwapQuoteSurfacesQuoteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// THORnode surfaces errors via "message" (e.g. dust threshold), not "error".
		_, _ = w.Write([]byte(`{"message":"amount less than dust threshold"}`))
	}))
	defer srv.Close()

	p := NewProvider()
	p.NodeURL = srv.URL
	if _, err := p.GetSwapQuote(context.Background(), "BSC.BNB", "1", "bc1q"); err == nil {
		t.Fatal("expected error from THORnode message field")
	}
}

func TestGetSwapQuoteBelowMin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// THORnode rejects a below-minimum amount with the min embedded in the body.
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":3,"message":"amount less than min swap amount (recommended_min_amount_in: 401682894): invalid request"}`))
	}))
	defer srv.Close()

	p := NewProvider()
	p.NodeURL = srv.URL
	_, err := p.GetSwapQuote(context.Background(), "BSC.USDT-0x55d3", "47688700", "bc1qexampleaddressxxxxxxxxxxxxxx")
	var bm *BelowMinError
	if !errors.As(err, &bm) {
		t.Fatalf("expected *BelowMinError, got %v", err)
	}
	if bm.MinIn != 401682894 {
		t.Fatalf("MinIn = %d, want 401682894", bm.MinIn)
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
