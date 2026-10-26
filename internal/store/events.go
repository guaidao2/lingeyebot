package store

import (
	"database/sql"
)

// AddEvent appends one execution record.
func (s *Store) AddEvent(e Event) (Event, error) {
	if e.ID == "" {
		e.ID = NewID("evt")
	}
	if e.CreatedAt == "" {
		e.CreatedAt = nowStr()
	}
	_, err := s.DB.Exec(`INSERT INTO events (id, run_id, task_id, kind, tool, summary, detail, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, e.ID, e.RunID, e.TaskID, e.Kind, e.Tool, e.Summary, e.Detail, e.CreatedAt)
	return e, err
}

// ListEvents returns the newest events for a run, oldest first within the window.
func (s *Store) ListEvents(runID string, limit int) ([]Event, error) {
	if limit <= 0 || limit > 2000 {
		limit = 200
	}
	rows, err := s.DB.Query(`SELECT id, run_id, task_id, kind, tool, summary, detail, created_at
		FROM events WHERE run_id=? ORDER BY created_at DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.RunID, &e.TaskID, &e.Kind, &e.Tool, &e.Summary, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	reverse(out)
	return out, rows.Err()
}

// ListTaskEvents returns events for one subagent task.
func (s *Store) ListTaskEvents(runID, taskID string, limit int) ([]Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.Query(`SELECT id, run_id, task_id, kind, tool, summary, detail, created_at
		FROM events WHERE run_id=? AND task_id=? ORDER BY created_at DESC LIMIT ?`, runID, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.RunID, &e.TaskID, &e.Kind, &e.Tool, &e.Summary, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	reverse(out)
	return out, rows.Err()
}

// SearchTaskEvents searches execution notes across tasks in a run.
func (s *Store) SearchTaskEvents(runID, query string, limit int) ([]Event, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	pat := likePattern(query)
	rows, err := s.DB.Query(`SELECT id, run_id, task_id, kind, tool, summary, detail, created_at
		FROM events WHERE run_id=? AND (summary LIKE ? OR detail LIKE ?)
		ORDER BY created_at DESC LIMIT ?`, runID, pat, pat, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.RunID, &e.TaskID, &e.Kind, &e.Tool, &e.Summary, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func reverse[T any](xs []T) {
	for i, j := 0, len(xs)-1; i < j; i, j = i+1, j-1 {
		xs[i], xs[j] = xs[j], xs[i]
	}
}

var _ = sql.ErrNoRows
