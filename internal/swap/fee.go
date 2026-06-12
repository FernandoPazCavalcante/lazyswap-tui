// Package swap orchestrates DEX quoting, approvals, and execution.
//
// Mirrors src/blockchain/swap/* from the Bun reference. THORchain (cross-chain
// BTC) is intentionally out of scope for this phase.
package swap

// SwapFeePercent is the fee taken from the input token on every swap
// (0.15 = 0.15%). Mirrors fee-config.ts.
const SwapFeePercent = 0.15

// FeeReceiver is the wallet that will eventually receive collected fees.
// Currently unused — fees remain in the user's wallet as unswapped tokens.
const FeeReceiver = "0x0000000000000000000000000000000000000000"

// FeeBreakdown is the result of CalcFee.
type FeeBreakdown struct {
	FeeAmount  float64
	NetAmount  float64
	FeePercent float64
}

// CalcFee deducts SwapFeePercent from gross and returns the breakdown.
func CalcFee(gross float64) FeeBreakdown {
	fee := gross * (SwapFeePercent / 100)
	return FeeBreakdown{
		FeeAmount:  fee,
		NetAmount:  gross - fee,
		FeePercent: SwapFeePercent,
	}
}
