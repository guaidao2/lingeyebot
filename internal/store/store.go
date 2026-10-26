// Package store owns SQLite persistence for lingeyebot.
package store

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Store is the SQLite-backed persistence layer.
type Store struct {
	DB          *sql.DB
	DataDir     string
	EvidenceDir string

	mu sync.Mutex
}

// Open opens (and migrates) the database inside dataDir.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	evidenceDir := filepath.Join(dataDir, "evidence")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		return nil, fmt.Errorf("create evidence dir: %w", err)
	}
	dbPath := filepath.Join(dataDir, "lingeyebot.sqlite")
	dsn := "file:" + filepath.ToSlash(dbPath) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	s := &Store{DB: db, DataDir: dataDir, EvidenceDir: evidenceDir}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate data: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.DB.Close() }

// WithTx runs fn in a transaction, rolling back on error.
func (s *Store) WithTx(fn func(*sql.Tx) error) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// NewID returns a short, prefixed random id.
func NewID(prefix string) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return prefix + "-000000000000"
	}
	return prefix + "-" + hex.EncodeToString(b)
}

// IsNotFound reports whether err is sql.ErrNoRows.
func IsNotFound(err error) bool { return err == sql.ErrNoRows }

func likePattern(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return "%"
	}
	return "%" + q + "%"
}
