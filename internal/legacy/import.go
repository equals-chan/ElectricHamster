package legacy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/equals-chan/ElectricHamster/internal/id"
	"github.com/equals-chan/ElectricHamster/internal/store"
	"github.com/equals-chan/ElectricHamster/internal/vault"

	_ "modernc.org/sqlite"
)

// ImportResult summarises an import.
type ImportResult struct {
	TaskID      string
	TaskCreated bool
	Archives    int
	Skipped     int
	PasswordsIn int
}

// ImportOptions controls Import.
type ImportOptions struct {
	ConfigPath string
	// AppDB and Vault are the modern store/vault to write into.
	Store *store.Store
	Vault *vault.Vault
}

// Import reads the legacy config + database and creates a task plus archive
// ledger entries. Passwords are moved into the encrypted vault.
func Import(ctx context.Context, opt ImportOptions) (ImportResult, error) {
	var res ImportResult
	if opt.Store == nil {
		return res, errors.New("legacy: no store provided")
	}
	if opt.Vault == nil || !opt.Vault.IsUnlocked() {
		return res, errors.New("legacy: vault must be unlocked to import passwords")
	}

	cfg, err := ParseConfigProperties(opt.ConfigPath)
	if err != nil {
		return res, err
	}
	if err := cfg.Validate(); err != nil {
		return res, err
	}

	base := cfg.BaseDir(opt.ConfigPath)
	dbPath := cfg.SqlPath + "db.sqlite3"
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(base, dbPath)
	}
	if _, err := os.Stat(dbPath); err != nil {
		return res, fmt.Errorf("legacy: cannot find legacy database at %s: %w", dbPath, err)
	}

	src := cfg.FolderPath
	dst := cfg.ZippedFilesPath

	// Reuse an existing task with the same source/dest, otherwise create one.
	task, created, err := findOrCreateTask(ctx, opt.Store, src, dst, cfg)
	if err != nil {
		return res, err
	}
	res.TaskID = task.ID
	res.TaskCreated = created
	if _, gerr := opt.Store.GetTask(ctx, task.ID); gerr != nil {
		return res, fmt.Errorf("legacy: task %q not persisted before insert: %w", task.ID, gerr)
	}

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return res, err
	}
	defer legacyDB.Close()

	rows, err := legacyDB.Query(`SELECT id, FolderName, PassWord, createTime FROM t_pwd ORDER BY id`)
	if err != nil {
		return res, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			oldID      int
			folderName string
			password   string
			createdAt  string
		)
		if err := rows.Scan(&oldID, &folderName, &password, &createdAt); err != nil {
			return res, err
		}
		archivePath := filepath.Join(dst, fmt.Sprintf("%d.zip", oldID))
		if _, found, err := opt.Store.ArchiveByPath(ctx, archivePath); err != nil {
			return res, err
		} else if found {
			res.Skipped++
			continue
		}

		pwdID := ""
		if password != "" {
			pwdID, err = opt.Vault.Put(folderName, password, "imported from legacy db")
			if err != nil {
				return res, err
			}
			res.PasswordsIn++
		}

		var size int64
		if fi, statErr := os.Stat(archivePath); statErr == nil {
			size = fi.Size()
		}

		rec := store.Archive{
			ID:          id.New(),
			TaskID:      task.ID,
			Seq:         int64(oldID),
			FolderName:  folderName,
			SourcePath:  filepath.Join(src, folderName),
			ArchivePath: archivePath,
			ArchiveSize: size,
			ContentSig:  "",
			PasswordID:  pwdID,
			Status:      store.ArchiveOK,
		}
		if err := opt.Store.InsertArchive(ctx, rec); err != nil {
			return res, fmt.Errorf("legacy: insert archive task=%q seq=%d path=%q: %w", task.ID, rec.Seq, rec.ArchivePath, err)
		}
		res.Archives++
	}
	if err := rows.Err(); err != nil {
		return res, err
	}

	// Advance the task's sequence counter past the largest imported id so new
	// archives do not collide.
	if res.Archives > 0 {
		var maxSeq int64
		_ = legacyDB.QueryRow(`SELECT MAX(id) FROM t_pwd`).Scan(&maxSeq)
		if maxSeq > 0 {
			if err := opt.Store.EnsureSeqAtLeast(ctx, store.SeqCounterName(task.ID), maxSeq); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

func findOrCreateTask(ctx context.Context, st *store.Store, src, dst string, cfg Config) (store.Task, bool, error) {
	tasks, err := st.ListTasks(ctx)
	if err != nil {
		return store.Task{}, false, err
	}
	for _, t := range tasks {
		if t.SourceDir == src && t.DestDir == dst {
			return t, false, nil
		}
	}
	task := store.Task{
		ID:            id.New(),
		Name:          filepath.Base(src),
		SourceDir:     src,
		DestDir:       dst,
		Format:        "7z",
		Codec:         "zstd",
		Level:         15,
		Encrypt:       true,
		HeaderEncrypt: true,
		DedupRule:     "content_sig",
		Enabled:       true,
		Password: store.PasswordPolicy{
			Mode:   cfg.PasswordMode(),
			Length: 20,
			Fixed:  cfg.UseThisPwd,
		},
	}
	if err := st.CreateTask(ctx, task); err != nil {
		return store.Task{}, false, err
	}
	return task, true, nil
}
