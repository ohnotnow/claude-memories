package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type Memory struct {
	ID        int64  `json:"id"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type Store struct {
	db *sql.DB
}

func defaultDBPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "claude-memories", "memories.db"), nil
}

func NewStore(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	ctx := context.Background()
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return nil, fmt.Errorf("pragma %q: %w", pragma, err)
		}
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

type migration struct {
	version     int
	description string
	apply       func(ctx context.Context, tx *sql.Tx) error
}

var migrations = []migration{
	{1, "baseline memories table", func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE memories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`)
		return err
	}},
	{2, "add updated_at to memories", func(ctx context.Context, tx *sql.Tx) error {
		// Nullable, no default: NULL means "never rewritten since creation".
		_, err := tx.ExecContext(ctx, "ALTER TABLE memories ADD COLUMN updated_at TEXT")
		return err
	}},
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		version INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO schema_version (id, version) VALUES (1, 0)"); err != nil {
		return fmt.Errorf("seed schema_version: %w", err)
	}

	var current int
	if err := s.db.QueryRowContext(ctx, "SELECT version FROM schema_version WHERE id = 1").Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.version, err)
		}
		if err := m.apply(ctx, tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.description, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE schema_version SET version = ? WHERE id = 1", m.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("bump schema version: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}
	return nil
}

func (s *Store) Add(ctx context.Context, content string) (*Memory, error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO memories (content) VALUES (?)", content)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *Store) Get(ctx context.Context, id int64) (*Memory, error) {
	m := &Memory{}
	err := s.db.QueryRowContext(ctx,
		"SELECT id, content, created_at, COALESCE(updated_at, '') FROM memories WHERE id = ?", id,
	).Scan(&m.ID, &m.Content, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// Update rewrites a memory's content in place, preserving its id and
// created_at and stamping updated_at. Returns nil when the id doesn't exist.
func (s *Store) Update(ctx context.Context, id int64, content string) (*Memory, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE memories SET content = ?, updated_at = datetime('now') WHERE id = ?", content, id)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return s.Get(ctx, id)
}

// Search returns memories containing every whitespace-separated token of the
// query, in any order — agents naturally search with several descriptive
// words that rarely appear verbatim. A single token stays a plain substring
// match, and LIKE keeps its ASCII case-insensitivity.
func (s *Store) Search(ctx context.Context, query string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		tokens = []string{query}
	}
	conds := make([]string, len(tokens))
	args := make([]any, 0, len(tokens)+1)
	for i, tok := range tokens {
		conds[i] = "content LIKE ?"
		args = append(args, "%"+tok+"%")
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, content, created_at, COALESCE(updated_at, '') FROM memories WHERE "+
			strings.Join(conds, " AND ")+" ORDER BY id DESC LIMIT ?",
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect(rows)
}

func (s *Store) List(ctx context.Context, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, content, created_at, COALESCE(updated_at, '') FROM memories ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect(rows)
}

// Count returns the total number of memories in the store, regardless of any
// limit. index() uses it to tell whether older memories fell below its cap.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func collect(rows *sql.Rows) ([]Memory, error) {
	// Non-nil so an empty result marshals as [] rather than null.
	out := []Memory{}
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.Content, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Delete(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM memories WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
