package login

import "testing"

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		in   string
		want string // empty = accepted
	}{
		{"Aa1xxxxx", ""}, // ok
		{"short1A", "Password must be at least 8 characters."},
		{"NOLOWER1", "Password must contain at least one lowercase letter."},
		{"nouppr12", "Password must contain at least one uppercase letter."},
		{"NoDigits", "Password must contain at least one digit."},
		{"", "Password must be at least 8 characters."},
	}
	for _, c := range cases {
		if got := ValidatePassword(c.in); got != c.want {
			t.Errorf("ValidatePassword(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
