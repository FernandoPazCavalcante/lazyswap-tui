package balance

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
)

// FormatUnits divides raw by 10^decimals and returns the result as a plain
// decimal string (no thousands separators). Mirrors ethers.formatUnits.
func FormatUnits(raw *big.Int, decimals uint8) string {
	if raw == nil {
		return "0"
	}
	if raw.Sign() == 0 {
		return "0"
	}
	str := raw.String()
	d := int(decimals)

	if len(str) <= d {
		// Pad with leading zeros and prefix with "0.".
		pad := strings.Repeat("0", d-len(str))
		s := "0." + pad + str
		return trimTrailingZeros(s)
	}
	intPart := str[:len(str)-d]
	fracPart := str[len(str)-d:]
	if fracPart == "" {
		return intPart
	}
	return trimTrailingZeros(intPart + "." + fracPart)
}

func trimTrailingZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// ParseUnits parses a decimal string into raw token units, multiplying by
// 10^decimals. Mirrors ethers.parseUnits. Excess fractional digits beyond
// `decimals` are rejected (matches ethers' strict mode).
func ParseUnits(s string, decimals uint8) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("parse units: empty input")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	intStr, fracStr := s, ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		intStr = s[:dot]
		fracStr = s[dot+1:]
	}
	if intStr == "" {
		intStr = "0"
	}
	for _, r := range intStr {
		if r < '0' || r > '9' {
			return nil, errors.New("parse units: invalid integer part")
		}
	}
	for _, r := range fracStr {
		if r < '0' || r > '9' {
			return nil, errors.New("parse units: invalid fractional part")
		}
	}
	if len(fracStr) > int(decimals) {
		return nil, errors.New("parse units: too many fractional digits")
	}
	fracStr += strings.Repeat("0", int(decimals)-len(fracStr))
	combined := intStr + fracStr
	combined = strings.TrimLeft(combined, "0")
	if combined == "" {
		return big.NewInt(0), nil
	}
	v, ok := new(big.Int).SetString(combined, 10)
	if !ok {
		return nil, errors.New("parse units: bigint conversion failed")
	}
	if neg {
		v.Neg(v)
	}
	return v, nil
}

// FormatBalance renders raw as a human-readable balance with commas, capped
// at six fractional digits. Matches the Bun formatBalance() output.
func FormatBalance(raw *big.Int, decimals uint8) string {
	s := FormatUnits(raw, decimals)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return "0.00"
	}
	return formatFloat(f, 2, 6)
}

// FormatUSD renders amount as "$1,234.56".
func FormatUSD(amount float64) string {
	return "$" + formatFloat(amount, 2, 2)
}

// formatFloat returns amount with thousands separators and a fractional
// digit count clamped to [minFrac, maxFrac]. Trailing zeros beyond minFrac
// are stripped.
func formatFloat(amount float64, minFrac, maxFrac int) string {
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}

	// Format with maxFrac decimals first, then trim down to minFrac.
	s := strconv.FormatFloat(amount, 'f', maxFrac, 64)
	parts := strings.SplitN(s, ".", 2)
	intStr := parts[0]
	fracStr := ""
	if len(parts) == 2 {
		fracStr = parts[1]
	}

	// Strip trailing zeros down to minFrac.
	for len(fracStr) > minFrac && strings.HasSuffix(fracStr, "0") {
		fracStr = fracStr[:len(fracStr)-1]
	}

	// Insert thousand separators into intStr.
	withCommas := insertCommas(intStr)

	if fracStr == "" {
		return sign + withCommas
	}
	return sign + withCommas + "." + fracStr
}

func insertCommas(s string) string {
	n := len(s)
	if n <= 3 {
		return s
	}
	var b strings.Builder
	pre := n % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if n > pre {
			b.WriteString(",")
		}
	}
	for i := pre; i < n; i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < n {
			b.WriteString(",")
		}
	}
	return b.String()
}

// ParseUSD strips "$" and "," and returns the float value. Empty / unparseable
// inputs return (0, false).
func ParseUSD(s string) (float64, bool) {
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// TotalUSD sums the per-token USD strings; returns "$0.00" if none parse.
func TotalUSD(values []string) string {
	var total float64
	var any bool
	for _, v := range values {
		if v == "" {
			continue
		}
		if f, ok := ParseUSD(v); ok {
			total += f
			any = true
		}
	}
	if !any {
		return ""
	}
	return FormatUSD(total)
}
