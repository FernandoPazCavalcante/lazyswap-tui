package login

import "unicode"

// ValidatePassword returns nil when the password satisfies all creation rules,
// or a user-facing error string otherwise. Only enforced on first-access.
//
// Rules match src/tui/handlers/login-handler.ts:
//   - at least 8 characters
//   - contains a lowercase letter
//   - contains an uppercase letter
//   - contains a digit
func ValidatePassword(pw string) string {
	if len(pw) < 8 {
		return "Password must be at least 8 characters."
	}
	var hasLower, hasUpper, hasDigit bool
	for _, r := range pw {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLower {
		return "Password must contain at least one lowercase letter."
	}
	if !hasUpper {
		return "Password must contain at least one uppercase letter."
	}
	if !hasDigit {
		return "Password must contain at least one digit."
	}
	return ""
}
