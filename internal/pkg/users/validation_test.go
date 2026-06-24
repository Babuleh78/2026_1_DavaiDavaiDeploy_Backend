package users

import "testing"

func TestValidation(t *testing.T) {
	cases := []struct {
		name     string
		login    string
		password string
		wantMsg  string
		wantOK   bool
	}{
		{name: "valid credentials", login: "alice1", password: "password1", wantMsg: "Ok", wantOK: true},
		{name: "login too short", login: "abcde", password: "password1", wantMsg: "Invalid login length", wantOK: false},
		{name: "login too long", login: "abcdefghijklmnop", password: "password1", wantMsg: "Invalid login length", wantOK: false},
		{name: "login at min boundary", login: "abcdef", password: "password1", wantMsg: "Ok", wantOK: true},
		{name: "login at max boundary", login: "abcdefghijklmno", password: "password1", wantMsg: "Ok", wantOK: true},
		{name: "password too short", login: "alice1", password: "pass123", wantMsg: "Invalid password length", wantOK: false},
		{name: "password at min boundary", login: "alice1", password: "pass1234", wantMsg: "Ok", wantOK: true},
		{name: "login with invalid char (space)", login: "alic e", password: "password1", wantMsg: "Login contains invalid characters", wantOK: false},
		{name: "password with invalid char (space)", login: "alice1", password: "pass word", wantMsg: "Password contains invalid characters", wantOK: false},
		// Length is checked before character content; a too-short login with a bad
		// char still reports the length problem.
		{name: "length checked before chars", login: "a b", password: "password1", wantMsg: "Invalid login length", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, ok := Validation(tc.login, tc.password)
			if ok != tc.wantOK || msg != tc.wantMsg {
				t.Errorf("Validation(%q, %q) = (%q, %v), want (%q, %v)",
					tc.login, tc.password, msg, ok, tc.wantMsg, tc.wantOK)
			}
		})
	}
}
