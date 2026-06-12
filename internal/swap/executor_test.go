package swap

import "testing"

func TestValidateSwapInputs(t *testing.T) {
	good := validateSwapInputs("0xfrom", "0xto", "1.5", 0.5)
	if good != nil {
		t.Errorf("unexpected err on valid inputs: %v", good)
	}

	bad := []struct {
		name                       string
		from, to, amt              string
		slip                       float64
	}{
		{"empty from", "", "0xto", "1", 0.5},
		{"empty to", "0xfrom", "  ", "1", 0.5},
		{"bad amount", "0xfrom", "0xto", "nope", 0.5},
		{"zero amount", "0xfrom", "0xto", "0", 0.5},
		{"neg amount", "0xfrom", "0xto", "-1", 0.5},
		{"low slip", "0xfrom", "0xto", "1", -0.1},
		{"high slip", "0xfrom", "0xto", "1", 100.1},
	}
	for _, c := range bad {
		if err := validateSwapInputs(c.from, c.to, c.amt, c.slip); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
}

func TestNewExecutorRejectsBadKey(t *testing.T) {
	if _, err := NewExecutor(nil, "not-a-hex-key", "bsc"); err == nil {
		t.Errorf("expected error for invalid private key")
	}
}
