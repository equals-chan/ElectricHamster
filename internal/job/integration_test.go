package job

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/equals-chan/ElectricHamster/internal/archive"
	"github.com/equals-chan/ElectricHamster/internal/id"
	"github.com/equals-chan/ElectricHamster/internal/store"
	"github.com/equals-chan/ElectricHamster/internal/vault"
)

// TestEngineWithRealArchiver exercises the full pipeline with an actual 7-Zip
// binary. It is skipped when none is available.
func TestEngineWithRealArchiver(t *testing.T) {
	z, err := archive.NewSevenZip(context.Background())
	if err != nil {
		if errors.Is(err, archive.ErrToolNotFound) {
			t.Skipf("no 7-Zip binary: %v", err)
		}
		t.Fatal(err)
	}
	info, _ := z.Info(context.Background())
	codec := "zstd"
	if !info.HasZstd {
		codec = "lzma2"
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	writeTree(t, src, map[string][]string{
		"资料": {"a.txt", "b.txt"},
		"其它": {"c.txt"},
	})

	st, err := store.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	v, err := vault.Create(filepath.Join(dir, "vault.db"), "master", testVaultParams())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	task := store.Task{
		ID: id.New(), Name: "real", SourceDir: src, DestDir: dst,
		Format: "7z", Codec: codec, Level: 3, Encrypt: true, HeaderEncrypt: true,
		Password:  store.PasswordPolicy{Mode: "random", Length: 20},
		DedupRule: "content_sig", Enabled: true,
	}
	if err := st.CreateTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	engine := &Engine{Store: st, Vault: v, Archiver: z, Workers: 2, Verify: true}
	stats, err := engine.Run(context.Background(), task, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Done != 2 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want done=2 failed=0", stats)
	}

	archives, err := st.ListArchives(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range archives {
		if _, err := os.Stat(a.ArchivePath); err != nil {
			t.Errorf("archive missing: %v", err)
		}
		item, err := v.Get(a.PasswordID)
		if err != nil {
			t.Fatalf("vault.Get: %v", err)
		}
		if err := z.Verify(context.Background(), a.ArchivePath, item.Password); err != nil {
			t.Errorf("verify %s: %v", a.ArchivePath, err)
		}
	}
}
