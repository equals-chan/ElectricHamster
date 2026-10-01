// Package store owns the application database: tasks, runs, archives and
// counters. It never stores plaintext passwords.
package store

import (
	"database/sql"
	"embed"
	"fmt"

	ehdb "github.com/equals-chan/ElectricHamster/internal/db"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps the application SQLite database.
type Store struct {
	conn *sql.DB
}

// SetMaxOpenConns is exposed for tests that need concurrent readers.
func (s *Store) SetMaxOpenConns(n int) { s.conn.SetMaxOpenConns(n) }

// Open opens (creating if needed) the application database and applies
// outstanding migrations.
func Open(path string) (*Store, error) {
	conn, err := ehdb.Open(path)
	if err != nil {
		return nil, err
	}
	if err := ehdb.Migrate(conn, migrationsFS, "migrations"); err != nil {
		conn.Close()
		return nil, err
	}
	return &Store{conn: conn}, nil
}

// DB exposes the underlying connection for future repository code (M3).
func (s *Store) DB() *sql.DB { return s.conn }

// Close closes the database.
func (s *Store) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

// SchemaVersion returns the highest applied migration version.
func (s *Store) SchemaVersion() (int, error) {
	var v sql.NullInt64
	if err := s.conn.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, fmt.Errorf("store: schema version: %w", err)
	}
	return int(v.Int64), nil
}
