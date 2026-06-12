package swap

import "testing"

func TestApplySlippage(t *testing.T) {
	cases := []struct {
		in   string
		slip float64
		want string
	}{
		{"100", 1, "99.000000"},
		{"100", 0, "100.000000"},
		{"50", 0.5, "49.750000"},
		{"abc", 1, "0"},
	}
	for _, c := range cases {
		if got := ApplySlippage(c.in, c.slip); got != c.want {
			t.Errorf("ApplySlippage(%q, %g) = %q, want %q", c.in, c.slip, got, c.want)
		}
	}
}

func TestFormatSwapResult(t *testing.T) {
	ok := FlowResult{
		Success:      true,
		TxHash:       "0xabcdef0123456789xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		FromToken:    "BNB",
		ToToken:      "USDT",
		InputAmount:  "$50",
		OutputAmount: "30.0",
	}
	got := FormatSwapResult(ok)
	want := "Swap complete — $50 → 30.0 USDT  TX: 0xabcdef01..."
	if got != want {
		t.Errorf("FormatSwapResult ok mismatch:\n  got:  %q\n  want: %q", got, want)
	}

	fail := FlowResult{Success: false, Err: "boom"}
	if got := FormatSwapResult(fail); got != "Swap failed: boom" {
		t.Errorf("FormatSwapResult fail = %q", got)
	}

	failNoMsg := FlowResult{Success: false}
	if got := FormatSwapResult(failNoMsg); got != "Swap failed: unknown error" {
		t.Errorf("FormatSwapResult no-msg = %q", got)
	}
}

func TestFlowResolveAddress(t *testing.T) {
	f, err := NewFlow("bsc_testnet")
	if err != nil {
		t.Skipf("RPC dial unavailable: %v", err)
	}
	defer f.Close()

	if got := f.ResolveAddress(NativeSentinel); got != f.chain.WrappedNative {
		t.Errorf("ResolveAddress(native) = %q, want %q", got, f.chain.WrappedNative)
	}
	if got := f.ResolveAddress("0xabc"); got != "0xabc" {
		t.Errorf("ResolveAddress passthrough broke")
	}

	tok := f.NativeToken()
	if tok.Address != NativeSentinel {
		t.Errorf("NativeToken.Address = %q, want native", tok.Address)
	}
	if tok.Symbol == "" || tok.Decimals == 0 {
		t.Errorf("NativeToken missing fields: %+v", tok)
	}
}
