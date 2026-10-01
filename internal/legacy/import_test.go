package legacy

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/equals-chan/ElectricHamster/internal/store"
	"github.com/equals-chan/ElectricHamster/internal/vault"

	_ "modernc.org/sqlite"
)

func testVaultParams() vault.KDFParams {
	p := vault.DefaultKDFParams()
	p.Time = 1
	p.Memory = 8 * 1024
	p.Parallelism = 1
	return p
}

func TestParseConfigProperties(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.properties")
	content := "# comment\nfolderPath=D:/src\nzippedFilesPath=D:/out\nsqlPath=D:/db/\nifRandomPwd=1\nuseThisPwd=abc123\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseConfigProperties(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FolderPath != "D:/src" || cfg.ZippedFilesPath != "D:/out" {
		t.Errorf("unexpected config: %+v", cfg)
	}
	if cfg.PasswordMode() != "fixed" {
		t.Errorf("PasswordMode = %q, want fixed", cfg.PasswordMode())
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestImport(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	// Legacy config.
	cfgPath := filepath.Join(dir, "config.properties")
	cfg := "folderPath=" + src + "\nzippedFilesPath=" + out + "\nsqlPath=" + dir + "/\nifRandomPwd=0\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// Legacy database with the original schema.
	dbPath := filepath.Join(dir, "db.sqlite3")
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE t_pwd (id integer primary key, FolderName varchar(255), PassWord varchar(255), createTime varchar(255))`,
		`CREATE TABLE t_zipNO (id integer primary key, filesLastNO integer)`,
		`INSERT INTO t_zipNO VALUES (1, 10002)`,
		`INSERT INTO t_pwd VALUES (10000, 'alpha', 'pw-alpha', '2023-01-01')`,
		`INSERT INTO t_pwd VALUES (10001, 'beta', 'pw-beta', '2023-01-02')`,
	}
	for _, s := range stmts {
		if _, err := legacy.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	legacy.Close()

	// Fake a couple of archive files so sizes are picked up.
	os.WriteFile(filepath.Join(out, "10000.zip"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(out, "10001.zip"), []byte("bb"), 0o644)

	// Modern store + vault.
	appStore, err := store.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer appStore.Close()
	v, err := vault.Create(filepath.Join(dir, "vault.db"), "master", testVaultParams())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	res, err := Import(context.Background(), ImportOptions{ConfigPath: cfgPath, Store: appStore, Vault: v})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !res.TaskCreated || res.Archives != 2 || res.PasswordsIn != 2 {
		t.Fatalf("result = %+v, want task created, 2 archives, 2 passwords", res)
	}

	archives, err := appStore.ListArchives(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(archives) != 2 {
		t.Fatalf("archives = %d, want 2", len(archives))
	}
	// Password should be retrievable from the vault.
	item, err := v.Get(archives[0].PasswordID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Password != "pw-alpha" && item.Password != "pw-beta" {
		t.Errorf("unexpected imported password %q", item.Password)
	}
	if archives[0].ArchiveSize == 0 {
		t.Error("expected archive size to be populated")
	}

	// Re-importing must be idempotent (paths already recorded).
	res2, err := Import(context.Background(), ImportOptions{ConfigPath: cfgPath, Store: appStore, Vault: v})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Archives != 0 || res2.Skipped != 2 {
		t.Fatalf("second import = %+v, want 0 archives, 2 skipped", res2)
	}

	// The task's counter must be past the imported ids.
	seq, err := appStore.NextSeq(context.Background(), store.SeqCounterName(res.TaskID), 10000)
	if err != nil {
		t.Fatal(err)
	}
	if seq <= 10001 {
		t.Errorf("next seq = %d, want > 10001", seq)
	}
}
