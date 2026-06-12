package balance

import (
	"math/big"
	"testing"
)

func bi(s string) *big.Int {
	v, _ := new(big.Int).SetString(s, 10)
	return v
}

func TestParseUnits(t *testing.T) {
	cases := []struct {
		in       string
		decimals uint8
		want     string
	}{
		{"0", 18, "0"},
		{"1", 18, "1000000000000000000"},
		{"1.23", 18, "1230000000000000000"},
		{"0.000001", 6, "1"},
		{"12.345678", 6, "12345678"},
		{"  0.5  ", 6, "500000"},
		{".5", 6, "500000"},
	}
	for _, c := range cases {
		got, err := ParseUnits(c.in, c.decimals)
		if err != nil {
			t.Errorf("ParseUnits(%q, %d) err: %v", c.in, c.decimals, err)
			continue
		}
		if got.String() != c.want {
			t.Errorf("ParseUnits(%q, %d) = %s, want %s", c.in, c.decimals, got.String(), c.want)
		}
	}
}

func TestParseUnitsErrors(t *testing.T) {
	bad := []struct {
		in       string
		decimals uint8
	}{
		{"", 18},
		{"abc", 18},
		{"1.2345", 2},
		{"1.2.3", 18},
	}
	for _, c := range bad {
		if _, err := ParseUnits(c.in, c.decimals); err == nil {
			t.Errorf("ParseUnits(%q, %d) expected error", c.in, c.decimals)
		}
	}
}

func TestFormatUnits(t *testing.T) {
	cases := []struct {
		raw      string
		decimals uint8
		want     string
	}{
		{"0", 18, "0"},
		{"1000000000000000000", 18, "1"},
		{"1230000000000000000", 18, "1.23"},
		{"1234567890000000000", 18, "1.23456789"},
		{"1", 6, "0.000001"},
		{"500000", 6, "0.5"},
		{"12345678", 6, "12.345678"},
	}
	for _, c := range cases {
		if got := FormatUnits(bi(c.raw), c.decimals); got != c.want {
			t.Errorf("FormatUnits(%s, %d) = %q, want %q", c.raw, c.decimals, got, c.want)
		}
	}
}

func TestFormatBalance(t *testing.T) {
	cases := []struct {
		raw      string
		decimals uint8
		want     string
	}{
		{"0", 18, "0.00"},
		{"1000000000000000000", 18, "1.00"},
		{"1234560000000000000000", 18, "1,234.56"},
		{"12345670000000000000", 18, "12.34567"},
	}
	for _, c := range cases {
		if got := FormatBalance(bi(c.raw), c.decimals); got != c.want {
			t.Errorf("FormatBalance(%s, %d) = %q, want %q", c.raw, c.decimals, got, c.want)
		}
	}
}

func TestFormatUSD(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "$0.00"},
		{1, "$1.00"},
		{1234.5, "$1,234.50"},
		{1234567.89, "$1,234,567.89"},
	}
	for _, c := range cases {
		if got := FormatUSD(c.in); got != c.want {
			t.Errorf("FormatUSD(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseUSD(t *testing.T) {
	f, ok := ParseUSD("$1,234.56")
	if !ok || f != 1234.56 {
		t.Errorf("ParseUSD($1,234.56) = (%v, %v)", f, ok)
	}
	if _, ok := ParseUSD(""); ok {
		t.Errorf("ParseUSD('') should not be ok")
	}
	if _, ok := ParseUSD("not-a-number"); ok {
		t.Errorf("ParseUSD bad input should not be ok")
	}
}

func TestTotalUSD(t *testing.T) {
	got := TotalUSD([]string{"$10.00", "$5.50", "", "$0.25"})
	if got != "$15.75" {
		t.Errorf("TotalUSD = %q, want $15.75", got)
	}
	if TotalUSD([]string{"", ""}) != "" {
		t.Errorf("TotalUSD of empties should be empty")
	}
}
