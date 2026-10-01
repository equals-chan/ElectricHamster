// Package db provides SQLite connection setup and a small numbered-migration
// runner shared by the application store and the vault.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Open opens (creating if needed) a SQLite database at path with sensible
// pragmas. The driver is pure Go, so cross-compilation needs no cgo.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// A single connection keeps per-connection pragmas valid and avoids
	// SQLITE_BUSY for this low-concurrency desktop workload.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := conn.Exec(pragma); err != nil {
			conn.Close()
			return nil, fmt.Errorf("db: %s: %w", pragma, err)
		}
	}
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// splitStatements splits a SQL script into statements, ignoring semicolons
// inside single-quoted strings and `--` line comments.
func splitStatements(script string) []string {
	var out []string
	var b strings.Builder
	inSingle := false
	for i := 0; i < len(script); i++ {
		c := script[i]
		if inSingle {
			b.WriteByte(c)
			if c == '\'' {
				if i+1 < len(script) && script[i+1] == '\'' {
					b.WriteByte('\'')
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
			b.WriteByte(c)
		case '-':
			if i+1 < len(script) && script[i+1] == '-' {
				if j := strings.IndexByte(script[i:], '\n'); j >= 0 {
					b.WriteString(script[i : i+j+1])
					i += j
				} else {
					b.WriteString(script[i:])
					i = len(script)
				}
			} else {
				b.WriteByte(c)
			}
		case ';':
			if s := strings.TrimSpace(b.String()); s != "" {
				out = append(out, s)
			}
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		out = append(out, s)
	}
	return out
}
