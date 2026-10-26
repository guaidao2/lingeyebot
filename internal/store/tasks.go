package store

import (
	"database/sql"
	"encoding/json"
)

// CreateTask inserts one subagent task.
func (s *Store) CreateTask(t Task) (Task, error) {
	if t.ID == "" {
		t.ID = NewID("task")
	}
	if t.Status == "" {
		t.Status = "queued"
	}
	if t.Inbox == nil {
		t.Inbox = []InboxMessage{}
	}
	t.CreatedAt = nowStr()
	_, err := s.DB.Exec(`INSERT INTO tasks (id, run_id, parent_id, node_id, role, title, status, budget, inbox, result, error, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, t.ID, t.RunID, t.ParentID, t.NodeID, t.Role, t.Title,
		t.Status, t.Budget, mustJSON(t.Inbox), t.Result, t.Error, t.CreatedAt)
	return t, err
}

const selectTaskSQL = `SELECT id, run_id, parent_id, node_id, role, title, status, budget, inbox, result, error, created_at, started_at, finished_at
	FROM tasks WHERE id=?`

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(row taskScanner) (Task, error) {
	var t Task
	var inbox, started, finished sql.NullString
	err := row.Scan(&t.ID, &t.RunID, &t.ParentID, &t.NodeID, &t.Role, &t.Title,
		&t.Status, &t.Budget, &inbox, &t.Result, &t.Error, &t.CreatedAt, &started, &finished)
	if err != nil {
		return t, err
	}
	t.StartedAt, t.FinishedAt = started.String, finished.String
	_ = json.Unmarshal([]byte(inbox.String), &t.Inbox)
	if t.Inbox == nil {
		t.Inbox = []InboxMessage{}
	}
	return t, nil
}

// GetTask reads one task.
func (s *Store) GetTask(id string) (Task, error) {
	return scanTask(s.DB.QueryRow(selectTaskSQL, id))
}

// ListTasks returns tasks for a run.
func (s *Store) ListTasks(runID string) ([]Task, error) {
	rows, err := s.DB.Query(`SELECT id, run_id, parent_id, node_id, role, title, status, budget, inbox, result, error, created_at, started_at, finished_at
		FROM tasks WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		var inbox, started, finished sql.NullString
		if err := rows.Scan(&t.ID, &t.RunID, &t.ParentID, &t.NodeID, &t.Role, &t.Title, &t.Status,
			&t.Budget, &inbox, &t.Result, &t.Error, &t.CreatedAt, &started, &finished); err != nil {
			return nil, err
		}
		t.StartedAt, t.FinishedAt = started.String, finished.String
		_ = json.Unmarshal([]byte(inbox.String), &t.Inbox)
		if t.Inbox == nil {
			t.Inbox = []InboxMessage{}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateTaskStatus changes a task's status and result.
func (s *Store) UpdateTaskStatus(id, status, result, errText string) error {
	now := nowStr()
	var started, finished any
	switch status {
	case "running":
		started = now
	case "done", "failed", "cancelled", "blocked", "exhausted":
		finished = now
	}
	_, err := s.DB.Exec(`UPDATE tasks SET status=?, result=CASE WHEN ?<>'' THEN ? ELSE result END,
		error=CASE WHEN ?<>'' THEN ? ELSE error END,
		started_at=COALESCE(?, started_at), finished_at=COALESCE(?, finished_at)
		WHERE id=? AND (status NOT IN ('done','failed','cancelled') OR ?='cancelled')`,
		status, result, result, errText, errText, started, finished, id, status)
	return err
}

// AppendInbox queues one mid-run instruction for a task.
func (s *Store) AppendInbox(id, text string) error {
	return s.WithTx(func(tx *sql.Tx) error {
		t, err := scanTask(tx.QueryRow(selectTaskSQL, id))
		if err != nil {
			return err
		}
		t.Inbox = append(t.Inbox, InboxMessage{Text: text, At: nowStr()})
		_, err = tx.Exec(`UPDATE tasks SET inbox=? WHERE id=?`, mustJSON(t.Inbox), id)
		return err
	})
}

// TakeInbox returns and clears a task's queued instructions atomically.
func (s *Store) TakeInbox(id string) ([]InboxMessage, error) {
	var out []InboxMessage
	err := s.WithTx(func(tx *sql.Tx) error {
		t, err := scanTask(tx.QueryRow(selectTaskSQL, id))
		if err != nil {
			return err
		}
		if len(t.Inbox) == 0 {
			return nil
		}
		out = t.Inbox
		_, err = tx.Exec(`UPDATE tasks SET inbox='[]' WHERE id=?`, id)
		return err
	})
	return out, err
}

// ActiveTaskCount counts running subagent tasks for a run.
func (s *Store) ActiveTaskCount(runID string) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM tasks WHERE run_id=? AND status IN ('queued','running')`, runID).Scan(&n)
	return n, err
}

// CancelActiveTasks marks all running tasks of a run as cancelled.
func (s *Store) CancelActiveTasks(runID string) error {
	_, err := s.DB.Exec(`UPDATE tasks SET status='cancelled', finished_at=? WHERE run_id=? AND status IN ('queued','running')`, nowStr(), runID)
	return err
}

// ListAllTasks returns recent tasks across all conversations.
func (s *Store) ListAllTasks(limit int) ([]Task, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.DB.Query(`SELECT id, run_id, parent_id, node_id, role, title, status, budget, inbox, result, error, created_at, started_at, finished_at
		FROM tasks ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
