package usecase

import (
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"
	uuid "github.com/satori/go.uuid"
)

func newTestUsecase(t *testing.T) *AuthUsecase {
	t.Helper()
	// repo is nil: token generation/parsing does not touch it.
	return NewAuthUsecase(nil, "test-secret-at-least-32-bytes-long-xxxxx")
}

func TestGenerateAndParseToken(t *testing.T) {
	uc := newTestUsecase(t)
	id := uuid.NewV4()

	tokenStr, err := uc.GenerateToken(id, "alice", 3)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	parsed, err := uc.ParseToken(tokenStr)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("parsed token is not valid")
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims are not jwt.MapClaims")
	}
	if claims["login"] != "alice" {
		t.Errorf("login claim = %v, want alice", claims["login"])
	}
	if v, _ := claims["version"].(float64); int(v) != 3 {
		t.Errorf("version claim = %v, want 3", claims["version"])
	}
}

func TestParseTokenRejectsTamperedSignature(t *testing.T) {
	uc := newTestUsecase(t)
	tokenStr, err := uc.GenerateToken(uuid.NewV4(), "bob", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Flip the last character of the signature.
	tampered := tokenStr[:len(tokenStr)-1]
	if tokenStr[len(tokenStr)-1] == 'a' {
		tampered += "b"
	} else {
		tampered += "a"
	}

	parsed, err := uc.ParseToken(tampered)
	if err == nil && parsed != nil && parsed.Valid {
		t.Error("ParseToken accepted a token with a tampered signature")
	}
}

func TestParseTokenRejectsWrongSecret(t *testing.T) {
	uc := newTestUsecase(t)
	tokenStr, _ := uc.GenerateToken(uuid.NewV4(), "carol", 1)

	other := &AuthUsecase{secret: "a-completely-different-signing-secret-value"}
	parsed, err := other.ParseToken(tokenStr)
	if err == nil && parsed != nil && parsed.Valid {
		t.Error("token forged under one secret was accepted under another")
	}
}
