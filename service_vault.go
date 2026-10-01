package main

import (
	"fmt"
	"os"

	"github.com/equals-chan/ElectricHamster/internal/vault"
)

// VaultService is the frontend-facing API for the encrypted password vault.
type VaultService struct{ rt *Runtime }

// Status reports whether the vault file exists and is currently unlocked.
func (s *VaultService) Status() VaultStatus {
	_, err := os.Stat(s.rt.vaultPath)
	v := s.rt.currentVault()
	return VaultStatus{
		Path:     s.rt.vaultPath,
		Exists:   err == nil,
		Unlocked: v != nil && v.IsUnlocked(),
	}
}

// Create initialises a new vault with the given master password.
func (s *VaultService) Create(master string) error {
	if s.rt.currentVault() != nil {
		return fmt.Errorf("vault is already open")
	}
	if _, err := os.Stat(s.rt.vaultPath); err == nil {
		return fmt.Errorf("vault already exists; unlock it instead")
	}
	v, err := vault.Create(s.rt.vaultPath, master, vault.DefaultKDFParams())
	if err != nil {
		return err
	}
	s.rt.setVault(v)
	s.rt.emit("vault:changed", s.Status())
	return nil
}

// Unlock opens and unlocks the vault.
func (s *VaultService) Unlock(master string) error {
	v := s.rt.currentVault()
	if v == nil {
		opened, err := vault.Open(s.rt.vaultPath)
		if err != nil {
			return err
		}
		v = opened
		s.rt.setVault(v)
	}
	if err := v.Unlock(master); err != nil {
		return err
	}
	s.rt.emit("vault:changed", s.Status())
	return nil
}

// Lock wipes the data key from memory.
func (s *VaultService) Lock() error {
	if v := s.rt.currentVault(); v != nil {
		v.Lock()
	}
	s.rt.emit("vault:changed", s.Status())
	return nil
}

// List returns vault entries (no secrets).
func (s *VaultService) List() ([]ItemView, error) {
	v := s.rt.currentVault()
	if v == nil {
		return []ItemView{}, nil
	}
	items, err := v.List()
	if err != nil {
		return nil, err
	}
	out := make([]ItemView, 0, len(items))
	for _, it := range items {
		out = append(out, ItemView{ID: it.ID, Label: it.Label, CreatedAt: it.CreatedAt.Format("2006-01-02 15:04:05")})
	}
	return out, nil
}

// Reveal returns the plaintext password for an item.
func (s *VaultService) Reveal(id string) (string, error) {
	v := s.rt.currentVault()
	if v == nil {
		return "", fmt.Errorf("vault is not open")
	}
	item, err := v.Get(id)
	if err != nil {
		return "", err
	}
	return item.Password, nil
}

// Generate returns a random password suggestion.
func (s *VaultService) Generate(length int, symbols bool) (string, error) {
	charset := vault.CharsetAlnum
	if symbols {
		charset = vault.CharsetSymbols
	}
	return vault.GeneratePasswordWith(length, charset)
}

// Update changes an entry's label and password (the note is preserved).
func (s *VaultService) Update(id, label, password string) error {
	v := s.rt.currentVault()
	if v == nil {
		return fmt.Errorf("vault is not open")
	}
	return v.Update(id, label, password)
}
