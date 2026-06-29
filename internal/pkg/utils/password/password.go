package password

import (
	"crypto/rand"
	"crypto/subtle"

	"golang.org/x/crypto/argon2"
)

const (
	saltLen = 16
	// keyLen менять нельзя: Check по нему отделяет соль от ключа в уже сохранённых хэшах
	keyLen = 32

	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
)

func Hash(plainPassword string) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := argon2.IDKey([]byte(plainPassword), salt, argonTime, argonMemory, argonThreads, keyLen)
	return append(salt, key...), nil
}

func Check(storedHash []byte, plainPassword string) bool {
	if len(storedHash) <= keyLen {
		return false
	}
	salt := storedHash[:len(storedHash)-keyLen]
	key := argon2.IDKey([]byte(plainPassword), salt, argonTime, argonMemory, argonThreads, keyLen)
	candidate := append(append([]byte{}, salt...), key...)
	return subtle.ConstantTimeCompare(candidate, storedHash) == 1
}
