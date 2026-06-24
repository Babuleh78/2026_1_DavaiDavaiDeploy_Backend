// Package password centralises argon2id password hashing and verification.
//
// Storage format: the random salt is prepended to the derived key, i.e.
// stored = salt || argon2id(password, salt). The key length is fixed
// (keyLen), so Check recovers the salt as everything before the last keyLen
// bytes. This keeps verification backward-compatible with hashes produced by
// older code that used a shorter salt.
package password

import (
	"crypto/rand"
	"crypto/subtle"

	"golang.org/x/crypto/argon2"
)

const (
	// saltLen is the salt size for newly created hashes. Older stored hashes
	// may use a different (shorter) salt; Check derives the salt length from
	// the stored blob so it stays compatible.
	saltLen = 16
	// keyLen is the argon2 output length. Do NOT change it: Check relies on it
	// to split salt from key in already-stored hashes.
	keyLen = 32

	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
)

// Hash derives an argon2id hash of plainPassword with a fresh random salt and
// returns salt || key.
func Hash(plainPassword string) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := argon2.IDKey([]byte(plainPassword), salt, argonTime, argonMemory, argonThreads, keyLen)
	return append(salt, key...), nil
}

// Check reports whether plainPassword matches the stored hash. It is safe
// against malformed/short input (no panic) and compares in constant time.
func Check(storedHash []byte, plainPassword string) bool {
	// A valid blob is salt (>=1 byte) followed by a keyLen-byte key.
	if len(storedHash) <= keyLen {
		return false
	}
	salt := storedHash[:len(storedHash)-keyLen]
	key := argon2.IDKey([]byte(plainPassword), salt, argonTime, argonMemory, argonThreads, keyLen)
	candidate := append(append([]byte{}, salt...), key...)
	return subtle.ConstantTimeCompare(candidate, storedHash) == 1
}
