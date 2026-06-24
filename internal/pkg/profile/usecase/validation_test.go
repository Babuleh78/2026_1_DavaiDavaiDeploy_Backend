package usecase

import (
	"strings"
	"testing"
)

func TestIsValidLogin(t *testing.T) {
	cases := []struct {
		name  string
		login string
		want  bool
	}{
		{name: "valid login", login: "dancer1", want: true},
		{name: "min length boundary", login: "abcdef", want: true},
		{name: "max length boundary", login: "abcdefghijklmno", want: true},
		{name: "too short", login: "abcde", want: false},
		{name: "too long", login: "abcdefghijklmnop", want: false},
		{name: "empty", login: "", want: false},
		{name: "contains space", login: "dan cer", want: false},
		// Cyrillic is outside ValidChars even though length is fine.
		{name: "contains non-ascii", login: "танцор", want: false},
		{name: "allowed special chars", login: "a!@#$%", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidLogin(tc.login); got != tc.want {
				t.Errorf("isValidLogin(%q) = %v, want %v", tc.login, got, tc.want)
			}
		})
	}
}

// Guard: a login at the boundary should also be exercisable via strings length
// to document the 6..15 inclusive range the function enforces.
func TestIsValidLogin_LengthRange(t *testing.T) {
	if isValidLogin(strings.Repeat("a", 5)) {
		t.Error("5-char login should be invalid")
	}
	if !isValidLogin(strings.Repeat("a", 6)) {
		t.Error("6-char login should be valid")
	}
	if !isValidLogin(strings.Repeat("a", 15)) {
		t.Error("15-char login should be valid")
	}
	if isValidLogin(strings.Repeat("a", 16)) {
		t.Error("16-char login should be invalid")
	}
}
