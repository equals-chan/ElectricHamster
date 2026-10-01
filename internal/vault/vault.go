// Package vault implements an encrypted password store. A master password is
// stretched with Argon2id into a key (KEK) that wraps a random data key (DEK);
// item passwords are encrypted with the DEK using AES-256-GCM. This indirection
// lets the master password change without re-encrypting every item.
package vault

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	ehdb "github.com/equals-chan/ElectricHamster/internal/db"
	"github.com/equals-chan/ElectricHamster/internal/id"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const formatVersion = 1

var (
	ErrNotExist      = errors.New("vault: does not exist")
	ErrExists        = errors.New("vault: already exists")
	ErrLocked        = errors.New("vault: locked")
	ErrWrongPassword = errors.New("vault: wrong master password")
	ErrEmptyPassword = errors.New("vault: master password must not be empty")
	ErrEmptyLabel    = errors.New("vault: label must not be empty")
	ErrNotFound      = errors.New("vault: item not found")
)

// Item is a decrypted password entry.
type Item struct {
	ID        string
	Label     string
	Password  string
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ItemMeta is a password entry without its secret.
type ItemMeta struct {
	ID        string
	Label     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Vault is opened from disk and starts locked; call Unlock with the master
// password before reading or writing secrets.
type Vault struct {
	path   string
	conn   *sql.DB
	params KDFParams

	mu       sync.RWMutex
	dek      []byte
	unlocked bool
}

// Create initialises a brand new vault at path and returns it unlocked.
func Create(path, masterPassword string, params KDFParams) (*Vault, error) {
	if masterPassword == "" {
		return nil, ErrEmptyPassword
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
		return nil, ErrExists
	}
	if params.Algorithm == "" {
		params = DefaultKDFParams()
	}
	if params.SaltLen == 0 {
		params.SaltLen = DefaultKDFParams().SaltLen
	}
	if params.KeyLen == 0 {
		params.KeyLen = DefaultKDFParams().KeyLen
	}

	conn, err := ehdb.Open(path)
	if err != nil {
		return nil, err
	}
	if err := ehdb.Migrate(conn, migrationsFS, "migrations"); err != nil {
		conn.Close()
		return nil, err
	}
	var exists int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM meta`).Scan(&exists); err != nil {
		conn.Close()
		return nil, err
	}
	if exists > 0 {
		conn.Close()
		return nil, ErrExists
	}

	salt, err := randomBytes(int(params.SaltLen))
	if err != nil {
		conn.Close()
		return nil, err
	}
	params.Salt = salt
	if err := params.validate(); err != nil {
		conn.Close()
		return nil, err
	}

	kek := params.derive(masterPassword)
	defer zero(kek)
	dek, err := randomBytes(int(params.KeyLen))
	if err != nil {
		conn.Close()
		return nil, err
	}
	kekNonce, dekCiphertext, err := seal(kek, dek, aadDEK)
	if err != nil {
		conn.Close()
		return nil, err
	}
	now := nowUTC()
	if _, err := conn.Exec(`INSERT INTO meta
		(id, format_version, kdf, kdf_time, kdf_memory, kdf_parallelism, key_len,
		 kdf_salt, kek_nonce, dek_ciphertext, created_at, updated_at)
		VALUES (1,?,?,?,?,?,?,?,?,?,?,?)`,
		formatVersion, params.Algorithm, params.Time, params.Memory, params.Parallelism, params.KeyLen,
		params.Salt, kekNonce, dekCiphertext, now, now); err != nil {
		conn.Close()
		return nil, err
	}

	return &Vault{path: path, conn: conn, params: params, dek: dek, unlocked: true}, nil
}

// Open opens an existing vault in the locked state.
func Open(path string) (*Vault, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, ErrNotExist
	}
	conn, err := ehdb.Open(path)
	if err != nil {
		return nil, err
	}
	if err := ehdb.Migrate(conn, migrationsFS, "migrations"); err != nil {
		conn.Close()
		return nil, err
	}
	v := &Vault{path: path, conn: conn}
	if _, err := v.loadParams(); err != nil {
		conn.Close()
		return nil, err
	}
	return v, nil
}

// Path returns the vault file path.
func (v *Vault) Path() string { return v.path }

// IsUnlocked reports whether the DEK is currently in memory.
func (v *Vault) IsUnlocked() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.unlocked
}

// FormatVersion returns the vault format version recorded on disk.
func (v *Vault) FormatVersion() (int, error) {
	var fv int
	err := v.conn.QueryRow(`SELECT format_version FROM meta WHERE id = 1`).Scan(&fv)
	return fv, err
}

// loadParams reads the KDF header from disk into v.params.
func (v *Vault) loadParams() (KDFParams, error) {
	var p KDFParams
	var fv int
	err := v.conn.QueryRow(`SELECT format_version, kdf, kdf_time, kdf_memory,
		kdf_parallelism, key_len, kdf_salt FROM meta WHERE id = 1`).
		Scan(&fv, &p.Algorithm, &p.Time, &p.Memory, &p.Parallelism, &p.KeyLen, &p.Salt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotExist
	}
	if err != nil {
		return p, err
	}
	if fv != formatVersion {
		return p, fmt.Errorf("vault: unsupported format version %d", fv)
	}
	p.SaltLen = uint32(len(p.Salt))
	if err := p.validate(); err != nil {
		return p, err
	}
	v.params = p
	return p, nil
}

// Unlock derives the KEK from the master password and unwraps the DEK.
func (v *Vault) Unlock(masterPassword string) error {
	if masterPassword == "" {
		return ErrEmptyPassword
	}
	params, err := v.loadParams()
	if err != nil {
		return err
	}
	var kekNonce, dekCiphertext []byte
	if err := v.conn.QueryRow(`SELECT kek_nonce, dek_ciphertext FROM meta WHERE id = 1`).
		Scan(&kekNonce, &dekCiphertext); err != nil {
		return err
	}
	kek := params.derive(masterPassword)
	defer zero(kek)
	dek, err := open(kek, kekNonce, dekCiphertext, aadDEK)
	if err != nil {
		return ErrWrongPassword
	}
	v.mu.Lock()
	zero(v.dek)
	v.dek = dek
	v.unlocked = true
	v.mu.Unlock()
	return nil
}

// Lock wipes the DEK from memory.
func (v *Vault) Lock() {
	v.mu.Lock()
	zero(v.dek)
	v.dek = nil
	v.unlocked = false
	v.mu.Unlock()
}

// Close locks and closes the database.
func (v *Vault) Close() error {
	v.Lock()
	if v.conn == nil {
		return nil
	}
	err := v.conn.Close()
	v.conn = nil
	return err
}

// dekCopy returns a copy of the in-memory DEK. Callers must zero it.
func (v *Vault) dekCopy() ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if !v.unlocked || v.dek == nil {
		return nil, ErrLocked
	}
	cp := make([]byte, len(v.dek))
	copy(cp, v.dek)
	return cp, nil
}

// Put stores a new password entry and returns its id.
func (v *Vault) Put(label, password, note string) (string, error) {
	if label == "" {
		return "", ErrEmptyLabel
	}
	dek, err := v.dekCopy()
	if err != nil {
		return "", err
	}
	defer zero(dek)

	entryID := id.New()
	nonce, ciphertext, err := seal(dek, []byte(password), aadItem(entryID))
	if err != nil {
		return "", err
	}
	var noteNonce, noteCiphertext []byte
	if note != "" {
		noteNonce, noteCiphertext, err = seal(dek, []byte(note), aadNote(entryID))
		if err != nil {
			return "", err
		}
	}
	now := nowUTC()
	if _, err := v.conn.Exec(`INSERT INTO items
		(id, label, nonce, ciphertext, note_nonce, note_ciphertext, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		entryID, label, nonce, ciphertext, noteNonce, noteCiphertext, now, now); err != nil {
		return "", err
	}
	return entryID, nil
}

// Get returns a decrypted item by id. Its Password is only available when
// unlocked.
func (v *Vault) Get(entryID string) (Item, error) {
	var (
		label                 string
		nonce, ciphertext     []byte
		noteNonce, noteCipher []byte
		createdAt, updatedAt  string
	)
	err := v.conn.QueryRow(`SELECT label, nonce, ciphertext, note_nonce, note_ciphertext,
		created_at, updated_at FROM items WHERE id = ?`, entryID).
		Scan(&label, &nonce, &ciphertext, &noteNonce, &noteCipher, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}

	dek, err := v.dekCopy()
	if err != nil {
		return Item{}, err
	}
	defer zero(dek)
	password, err := open(dek, nonce, ciphertext, aadItem(entryID))
	if err != nil {
		return Item{}, fmt.Errorf("vault: decrypt item %s: %w", entryID, err)
	}
	item := Item{
		ID:        entryID,
		Label:     label,
		Password:  string(password),
		CreatedAt: parseUTC(createdAt),
		UpdatedAt: parseUTC(updatedAt),
	}
	if len(noteCipher) > 0 {
		note, err := open(dek, noteNonce, noteCipher, aadNote(entryID))
		if err != nil {
			return Item{}, fmt.Errorf("vault: decrypt note %s: %w", entryID, err)
		}
		item.Note = string(note)
	}
	return item, nil
}

// List returns metadata for all items (no secrets, works while locked).
func (v *Vault) List() ([]ItemMeta, error) {
	rows, err := v.conn.Query(`SELECT id, label, created_at, updated_at FROM items ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemMeta
	for rows.Next() {
		var m ItemMeta
		var created, updated string
		if err := rows.Scan(&m.ID, &m.Label, &created, &updated); err != nil {
			return nil, err
		}
		m.CreatedAt = parseUTC(created)
		m.UpdatedAt = parseUTC(updated)
		out = append(out, m)
	}
	return out, rows.Err()
}

// Update changes an item's label and password, re-encrypting the secret with
// the current data key. The note is preserved.
func (v *Vault) Update(entryID, label, password string) error {
	if label == "" {
		return ErrEmptyLabel
	}
	dek, err := v.dekCopy()
	if err != nil {
		return err
	}
	defer zero(dek)
	nonce, ciphertext, err := seal(dek, []byte(password), aadItem(entryID))
	if err != nil {
		return err
	}
	res, err := v.conn.Exec(`UPDATE items SET label = ?, nonce = ?, ciphertext = ?, updated_at = ?
		WHERE id = ?`, label, nonce, ciphertext, nowUTC(), entryID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes an item. It requires the vault to be unlocked.
func (v *Vault) Delete(entryID string) error {
	if _, err := v.dekCopy(); err != nil {
		return err
	}
	res, err := v.conn.Exec(`DELETE FROM items WHERE id = ?`, entryID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ChangeMasterPassword re-wraps the DEK under a new master password. It verifies
// oldPassword first and works whether or not the vault is currently unlocked.
func (v *Vault) ChangeMasterPassword(oldPassword, newPassword string) error {
	if newPassword == "" {
		return ErrEmptyPassword
	}
	params, err := v.loadParams()
	if err != nil {
		return err
	}
	var kekNonce, dekCiphertext []byte
	if err := v.conn.QueryRow(`SELECT kek_nonce, dek_ciphertext FROM meta WHERE id = 1`).
		Scan(&kekNonce, &dekCiphertext); err != nil {
		return err
	}
	oldKek := params.derive(oldPassword)
	dek, err := open(oldKek, kekNonce, dekCiphertext, aadDEK)
	zero(oldKek)
	if err != nil {
		return ErrWrongPassword
	}
	defer zero(dek)

	newSalt, err := randomBytes(int(params.SaltLen))
	if err != nil {
		return err
	}
	newParams := params
	newParams.Salt = newSalt
	newKek := newParams.derive(newPassword)
	newNonce, newCiphertext, err := seal(newKek, dek, aadDEK)
	zero(newKek)
	if err != nil {
		return err
	}
	if _, err := v.conn.Exec(`UPDATE meta SET kdf_salt = ?, kek_nonce = ?, dek_ciphertext = ?,
		updated_at = ? WHERE id = 1`, newSalt, newNonce, newCiphertext, nowUTC()); err != nil {
		return err
	}
	v.params = newParams
	// Keep the vault unlocked with the (unchanged) DEK.
	v.mu.Lock()
	zero(v.dek)
	v.dek = append([]byte(nil), dek...)
	v.unlocked = true
	v.mu.Unlock()
	return nil
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func parseUTC(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
