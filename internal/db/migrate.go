package db

import (
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

type migration struct {
	version int
	name    string
	body    string
}

// Migrate applies the numbered *.sql files in dir (an fs.FS) that have not been
// applied yet, recording each in a schema_migrations table. It is idempotent.
func Migrate(conn *sql.DB, fsys fs.FS, dir string) error {
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("db: create schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := conn.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	migrations, err := loadMigrations(fsys, dir)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		if err := apply(conn, m); err != nil {
			return fmt.Errorf("db: migration %d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
}

func loadMigrations(fsys fs.FS, dir string) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version, err := parseVersion(e.Name())
		if err != nil {
			return nil, err
		}
		body, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: version, name: e.Name(), body: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func parseVersion(name string) (int, error) {
	base := strings.TrimSuffix(name, ".sql")
	if i := strings.IndexByte(base, '_'); i >= 0 {
		base = base[:i]
	}
	v, err := strconv.Atoi(base)
	if err != nil {
		return 0, fmt.Errorf("db: migration %q: name must start with a number", name)
	}
	return v, nil
}

func apply(conn *sql.DB, m migration) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range splitStatements(m.body) {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("%w\nstatement: %s", err, stmt)
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return err
	}
	return tx.Commit()
}
