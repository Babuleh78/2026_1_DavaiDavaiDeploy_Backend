package password

import (
	"bytes"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestHashCheckRoundTrip(t *testing.T) {
	hash, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash returned error: %v", err)
	}
	if !Check(hash, "correct horse battery staple") {
		t.Error("Check rejected the correct password")
	}
	if Check(hash, "wrong password") {
		t.Error("Check accepted a wrong password")
	}
}

func TestHashUsesRandomSalt(t *testing.T) {
	h1, _ := Hash("same-password")
	h2, _ := Hash("same-password")
	if bytes.Equal(h1, h2) {
		t.Error("two hashes of the same password are identical — salt is not random")
	}
	// Both must still verify.
	if !Check(h1, "same-password") || !Check(h2, "same-password") {
		t.Error("randomly-salted hashes failed to verify")
	}
}

func TestHashLayout(t *testing.T) {
	hash, _ := Hash("x")
	if len(hash) != saltLen+keyLen {
		t.Errorf("expected hash length %d, got %d", saltLen+keyLen, len(hash))
	}
}

// TestCheckDoesNotPanicOnMalformedInput guards the original bug: CheckPass
// sliced passHash[:8] unconditionally and panicked on short/empty stored hashes.
func TestCheckDoesNotPanicOnMalformedInput(t *testing.T) {
	cases := map[string][]byte{
		"nil":              nil,
		"empty":            {},
		"shorter-than-key": make([]byte, keyLen-1),
		"exactly-key":      make([]byte, keyLen), // no room for salt
	}
	for name, blob := range cases {
		t.Run(name, func(t *testing.T) {
			if Check(blob, "anything") {
				t.Errorf("Check accepted malformed hash %q", name)
			}
		})
	}
}

// TestCheckBackwardCompatibleWithLegacySalt proves verification still works for
// hashes written by the old code, which used an 8-byte salt.
func TestCheckBackwardCompatibleWithLegacySalt(t *testing.T) {
	const legacySaltLen = 8
	pw := "legacy-user-password"
	salt := []byte("8bytesss")[:legacySaltLen]
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, keyLen)
	legacyHash := append(append([]byte{}, salt...), key...)

	if !Check(legacyHash, pw) {
		t.Error("Check failed to verify a legacy 8-byte-salt hash")
	}
	if Check(legacyHash, "nope") {
		t.Error("Check accepted a wrong password against a legacy hash")
	}
}
