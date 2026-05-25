package validate

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateUsername(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want error
	}{
		{"too short", "ab", ErrUsernameLen},
		{"too long", strings.Repeat("a", 33), ErrUsernameLen},
		{"with slash", "ab/c", ErrUsernameForbiddenCh},
		{"with at", "u@host", ErrUsernameForbiddenCh},
		{"with quote", "u'name", ErrUsernameForbiddenCh},
		{"ok", "alice_01", nil},
		{"ok cjk", "用户名01", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidateUsername(c.in); !errors.Is(got, c.want) {
				t.Errorf("ValidateUsername(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want error
	}{
		{"too short", "Aa1", ErrPasswordTooShort},
		{"too long", strings.Repeat("Aa1", 50), ErrPasswordTooLong},
		{"missing lower", "ABCDEFG1", ErrPasswordMissingLower},
		{"missing upper", "abcdefg1", ErrPasswordMissingUpper},
		{"missing digit", "Abcdefgh", ErrPasswordMissingDigit},
		{"strong", "Twilight1Pass", nil},
		{"strong with symbol", "Tw1!Light_pass", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidatePasswordStrength(c.in); !errors.Is(got, c.want) {
				t.Errorf("ValidatePasswordStrength(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestValidatePasswordLegacy(t *testing.T) {
	if err := ValidatePasswordLegacy("12345678"); err != nil {
		t.Errorf("legacy 8-char numeric should pass, got %v", err)
	}
	if err := ValidatePasswordLegacy("short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("expected ErrPasswordTooShort, got %v", err)
	}
}
