package vault

import (
	"fmt"

	"golang.org/x/crypto/argon2"
)

// KDFParams describes how the master password is stretched into a key. The
// parameters are stored in the vault header so they can be tuned or upgraded
// later while still opening old vaults.
type KDFParams struct {
	Algorithm   string
	Time        uint32
	Memory      uint32 // KiB
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
	Salt        []byte
}

// DefaultKDFParams returns conservative Argon2id parameters (~64 MiB, 3 passes).
func DefaultKDFParams() KDFParams {
	return KDFParams{
		Algorithm:   "argon2id",
		Time:        3,
		Memory:      64 * 1024,
		Parallelism: 4,
		SaltLen:     16,
		KeyLen:      32,
	}
}

func (p KDFParams) validate() error {
	if p.Algorithm != "argon2id" {
		return fmt.Errorf("vault: unsupported KDF %q", p.Algorithm)
	}
	if p.KeyLen != 32 {
		return fmt.Errorf("vault: unsupported key length %d", p.KeyLen)
	}
	if p.Time < 1 || p.Time > 20 {
		return fmt.Errorf("vault: KDF time %d out of range", p.Time)
	}
	// 8 MiB .. 2 GiB
	if p.Memory < 8*1024 || p.Memory > 2*1024*1024 {
		return fmt.Errorf("vault: KDF memory %d out of range", p.Memory)
	}
	if p.Parallelism < 1 || p.Parallelism > 64 {
		return fmt.Errorf("vault: KDF parallelism %d out of range", p.Parallelism)
	}
	if len(p.Salt) < 8 {
		return fmt.Errorf("vault: KDF salt too short")
	}
	return nil
}

// derive returns the key derived from password using the stored parameters.
func (p KDFParams) derive(password string) []byte {
	return argon2.IDKey([]byte(password), p.Salt, p.Time, p.Memory, p.Parallelism, p.KeyLen)
}
