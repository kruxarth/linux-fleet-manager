package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func EqualHash(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func Bearer(header string) (string, bool) {
	const p = "Bearer "
	if len(header) < len(p) || header[:len(p)] != p {
		return "", false
	}
	t := header[len(p):]
	if t == "" {
		return "", false
	}
	return t, true
}
