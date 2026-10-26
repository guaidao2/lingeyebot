package store

import "fmt"

// migrateEdges rebuilds the legacy edges table whose src/dst foreign keys
// pointed only at nodes. Goals live in their own table, so goal→node edges
// were silently rejected. New schemas have no FK on edge endpoints; the
// application validates both nodes and goals in AddEdge.
func (s *Store) migrateEdges() error {
	rows, err := s.DB.Query(`PRAGMA foreign_key_list(edges)`)
	if err != nil {
		return err
	}
	hasFK := rows.Next()
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !hasFK {
		return nil
	}
	if _, err := s.DB.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer s.DB.Exec(`PRAGMA foreign_keys=ON`)

	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`ALTER TABLE edges RENAME TO edges_legacy`,
		`CREATE TABLE edges (
			id         TEXT PRIMARY KEY,
			run_id     TEXT NOT NULL,
			src_id     TEXT NOT NULL,
			dst_id     TEXT NOT NULL,
			rel        TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`INSERT INTO edges (id, run_id, src_id, dst_id, rel, created_at)
		 SELECT id, run_id, src_id, dst_id, rel, created_at FROM edges_legacy`,
		`DROP TABLE edges_legacy`,
		`CREATE INDEX IF NOT EXISTS idx_edges_run ON edges(run_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_edges ON edges(run_id, src_id, dst_id, rel)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("migrate edges: %w", err)
		}
	}
	return tx.Commit()
}
