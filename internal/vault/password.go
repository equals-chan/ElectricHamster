package vault

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	// CharsetAlnum is the default, 7-Zip-friendly password alphabet.
	CharsetAlnum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	// CharsetSymbols adds punctuation, excluding characters that complicate
	// shell/argument handling.
	CharsetSymbols = CharsetAlnum + "!@#$%^&*()-_=+[]{};:,.?"
)

// GeneratePassword returns a cryptographically random password of the given
// length using CharsetAlnum.
func GeneratePassword(length int) (string, error) {
	return GeneratePasswordWith(length, CharsetAlnum)
}

// GeneratePasswordWith returns a random password drawn from charset using
// rejection-free big.Int sampling (no modulo bias).
func GeneratePasswordWith(length int, charset string) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("vault: password length must be positive")
	}
	if len(charset) < 2 {
		return "", fmt.Errorf("vault: charset must have at least 2 characters")
	}
	max := big.NewInt(int64(len(charset)))
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = charset[n.Int64()]
	}
	return string(out), nil
}
