package swap

import (
	"math"
	"testing"
)

func TestCalcFee(t *testing.T) {
	cases := []struct {
		gross   float64
		fee     float64
		net     float64
	}{
		{0, 0, 0},
		{100, 0.15, 99.85},
		{1, 0.0015, 0.9985},
		{1000, 1.5, 998.5},
	}
	for _, c := range cases {
		got := CalcFee(c.gross)
		if math.Abs(got.FeeAmount-c.fee) > 1e-9 {
			t.Errorf("CalcFee(%g).FeeAmount = %g, want %g", c.gross, got.FeeAmount, c.fee)
		}
		if math.Abs(got.NetAmount-c.net) > 1e-9 {
			t.Errorf("CalcFee(%g).NetAmount = %g, want %g", c.gross, got.NetAmount, c.net)
		}
		if got.FeePercent != SwapFeePercent {
			t.Errorf("FeePercent = %g, want %g", got.FeePercent, SwapFeePercent)
		}
	}
}
