package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

var (
	aadDEK = []byte("electric-hamster/vault/dek/v1")
)

func aadItem(id string) []byte {
	return []byte("electric-hamster/vault/item/v1:" + id)
}

func aadNote(id string) []byte {
	return []byte("electric-hamster/vault/note/v1:" + id)
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// seal encrypts plaintext with AES-256-GCM and returns a fresh nonce and the
// ciphertext (including the authentication tag).
func seal(key, plaintext, aad []byte) (nonce, ciphertext []byte, err error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, nil, err
	}
	nonce, err = randomBytes(gcm.NonceSize())
	if err != nil {
		return nil, nil, err
	}
	return nonce, gcm.Seal(nil, nonce, plaintext, aad), nil
}

// open decrypts ciphertext produced by seal. An authentication failure is
// returned as-is so callers can decide whether it means "wrong password" or
// "corrupt data".
func open(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("vault: invalid nonce length %d", len(nonce))
	}
	return gcm.Open(nil, nonce, ciphertext, aad)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
