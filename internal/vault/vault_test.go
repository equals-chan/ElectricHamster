package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testParams keeps Argon2id cheap so tests run fast while staying valid.
func testParams() KDFParams {
	p := DefaultKDFParams()
	p.Time = 1
	p.Memory = 8 * 1024
	p.Parallelism = 1
	return p
}

func newPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "vault.db")
}

func TestCreatePutGetPersist(t *testing.T) {
	path := newPath(t)
	v, err := Create(path, "master-pass", testParams())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !v.IsUnlocked() {
		t.Fatal("new vault should be unlocked")
	}
	id, err := v.Put("资料合集", "P@ssw0rd-123", "备注")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := v.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen in a fresh handle: simulates an application restart.
	v2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v2.Close()
	if v2.IsUnlocked() {
		t.Fatal("reopened vault should be locked")
	}
	if _, err := v2.Get(id); !errors.Is(err, ErrLocked) {
		t.Fatalf("Get while locked = %v, want ErrLocked", err)
	}
	if err := v2.Unlock("master-pass"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	got, err := v2.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Password != "P@ssw0rd-123" {
		t.Errorf("password = %q, want %q", got.Password, "P@ssw0rd-123")
	}
	if got.Note != "备注" {
		t.Errorf("note = %q, want %q", got.Note, "备注")
	}
	if got.Label != "资料合集" {
		t.Errorf("label = %q", got.Label)
	}
}

func TestWrongPassword(t *testing.T) {
	path := newPath(t)
	v, err := Create(path, "correct", testParams())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Put("a", "secret", ""); err != nil {
		t.Fatal(err)
	}
	v.Close()

	v2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer v2.Close()
	if err := v2.Unlock("wrong"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("Unlock(wrong) = %v, want ErrWrongPassword", err)
	}
	if v2.IsUnlocked() {
		t.Fatal("vault must stay locked after failed unlock")
	}
}

func TestChangeMasterPassword(t *testing.T) {
	path := newPath(t)
	v, err := Create(path, "old-pass", testParams())
	if err != nil {
		t.Fatal(err)
	}
	id, err := v.Put("folder", "the-password", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := v.ChangeMasterPassword("old-pass", "new-pass"); err != nil {
		t.Fatalf("ChangeMasterPassword: %v", err)
	}
	// Wrong old password should be rejected afterwards.
	if err := v.ChangeMasterPassword("old-pass", "x"); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("change with stale old password = %v, want ErrWrongPassword", err)
	}
	v.Close()

	v2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer v2.Close()
	if err := v2.Unlock("old-pass"); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("Unlock(old) = %v, want ErrWrongPassword", err)
	}
	if err := v2.Unlock("new-pass"); err != nil {
		t.Fatalf("Unlock(new): %v", err)
	}
	got, err := v2.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Password != "the-password" {
		t.Errorf("password after change = %q", got.Password)
	}
}

func TestDeleteAndList(t *testing.T) {
	path := newPath(t)
	v, err := Create(path, "m", testParams())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	id1, _ := v.Put("one", "p1", "")
	id2, _ := v.Put("two", "p2", "")

	list, err := v.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}

	if err := v.Delete(id1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := v.Delete(id1); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete twice = %v, want ErrNotFound", err)
	}
	if _, err := v.Get(id2); err != nil {
		t.Errorf("Get(id2) after delete: %v", err)
	}
	if _, err := v.Get(id1); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(deleted) = %v, want ErrNotFound", err)
	}
}

func TestCreateAndOpenErrors(t *testing.T) {
	path := newPath(t)
	if _, err := Open(path); !errors.Is(err, ErrNotExist) {
		t.Errorf("Open(missing) = %v, want ErrNotExist", err)
	}
	v, err := Create(path, "m", testParams())
	if err != nil {
		t.Fatal(err)
	}
	v.Close()
	if _, err := Create(path, "m", testParams()); !errors.Is(err, ErrExists) {
		t.Errorf("Create(existing) = %v, want ErrExists", err)
	}
	if _, err := Create(newPath(t), "", testParams()); !errors.Is(err, ErrEmptyPassword) {
		t.Errorf("Create(empty pwd) = %v, want ErrEmptyPassword", err)
	}
}

func TestSecretsAreNotStoredInPlaintext(t *testing.T) {
	path := newPath(t)
	v, err := Create(path, "master", testParams())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Put("folder", "TOP-SECRET-VALUE", "note-secret"); err != nil {
		t.Fatal(err)
	}
	v.Close()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// WAL mode means recent writes may still be in -wal; check both files.
	wal, _ := os.ReadFile(path + "-wal")
	haystack := string(raw) + string(wal)
	for _, secret := range []string{"TOP-SECRET-VALUE", "note-secret", "master"} {
		if strings.Contains(haystack, secret) {
			t.Errorf("plaintext %q found in database files", secret)
		}
	}
}

func TestUpdateEntry(t *testing.T) {
	path := newPath(t)
	v, err := Create(path, "m", testParams())
	if err != nil {
		t.Fatal(err)
	}
	id, err := v.Put("folder", "old-password", "keep-note")
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Update(id, "folder-renamed", "new-password"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	item, err := v.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if item.Password != "new-password" {
		t.Errorf("password = %q, want new-password", item.Password)
	}
	if item.Label != "folder-renamed" {
		t.Errorf("label = %q, want folder-renamed", item.Label)
	}
	if item.Note != "keep-note" {
		t.Errorf("note = %q, want preserved", item.Note)
	}
	if err := v.Update("missing", "x", "y"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update(missing) = %v, want ErrNotFound", err)
	}
	v.Close()

	// The change must persist across reopen.
	v2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer v2.Close()
	if err := v2.Unlock("m"); err != nil {
		t.Fatal(err)
	}
	got, err := v2.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Password != "new-password" {
		t.Errorf("persisted password = %q, want new-password", got.Password)
	}
}

func TestGeneratePassword(t *testing.T) {
	p, err := GeneratePassword(24)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 24 {
		t.Errorf("length = %d, want 24", len(p))
	}
	for _, c := range p {
		if !strings.ContainsRune(CharsetAlnum, c) {
			t.Errorf("unexpected character %q", c)
		}
	}
	if _, err := GeneratePassword(0); err == nil {
		t.Error("length 0 should error")
	}
	// Two draws must differ.
	q, _ := GeneratePassword(24)
	if p == q {
		t.Error("two generated passwords are identical")
	}
}

func TestKDFParamsStored(t *testing.T) {
	path := newPath(t)
	v, err := Create(path, "m", testParams())
	if err != nil {
		t.Fatal(err)
	}
	v.Close()
	v2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer v2.Close()
	if v2.params.Algorithm != "argon2id" {
		t.Errorf("algorithm = %q", v2.params.Algorithm)
	}
	if v2.params.Memory != 8*1024 {
		t.Errorf("memory = %d", v2.params.Memory)
	}
	fv, err := v2.FormatVersion()
	if err != nil || fv != formatVersion {
		t.Errorf("format version = %d (err %v), want %d", fv, err, formatVersion)
	}
}
