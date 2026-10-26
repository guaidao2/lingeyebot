package store

import (
	"database/sql"
	"fmt"
)

// CreateApproval queues a tool call for human confirmation.
func (s *Store) CreateApproval(a Approval) (Approval, error) {
	if a.ID == "" {
		a.ID = NewID("appr")
	}
	if a.Status == "" {
		a.Status = "pending"
	}
	a.CreatedAt = nowStr()
	_, err := s.DB.Exec(`INSERT INTO approvals (id, run_id, task_id, tool, input, reason, status, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, a.ID, a.RunID, a.TaskID, a.Tool, a.Input, a.Reason, a.Status, a.CreatedAt)
	return a, err
}

// GetApproval reads one approval.
func (s *Store) GetApproval(id string) (Approval, error) {
	var a Approval
	var decided sql.NullString
	err := s.DB.QueryRow(`SELECT id, run_id, task_id, tool, input, reason, status, created_at, decided_at
		FROM approvals WHERE id=?`, id).Scan(&a.ID, &a.RunID, &a.TaskID, &a.Tool, &a.Input,
		&a.Reason, &a.Status, &a.CreatedAt, &decided)
	a.DecidedAt = decided.String
	return a, err
}

// ListApprovals returns approvals for a run, optionally filtered by status.
func (s *Store) ListApprovals(runID, status string, limit int) ([]Approval, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, run_id, task_id, tool, input, reason, status, created_at, decided_at
		FROM approvals WHERE 1=1`
	args := []any{}
	if runID != "" {
		query += ` AND run_id=?`
		args = append(args, runID)
	}
	if status != "" {
		query += ` AND status=?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Approval{}
	for rows.Next() {
		var a Approval
		var decided sql.NullString
		if err := rows.Scan(&a.ID, &a.RunID, &a.TaskID, &a.Tool, &a.Input, &a.Reason, &a.Status, &a.CreatedAt, &decided); err != nil {
			return nil, err
		}
		a.DecidedAt = decided.String
		out = append(out, a)
	}
	return out, rows.Err()
}

// DecideApproval approves or denies a pending approval.
func (s *Store) DecideApproval(id string, approved bool) (Approval, error) {
	status := "denied"
	if approved {
		status = "approved"
	}
	result, err := s.DB.Exec(`UPDATE approvals SET status=?, decided_at=? WHERE id=? AND status='pending'`,
		status, nowStr(), id)
	if err != nil {
		return Approval{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Approval{}, fmt.Errorf("approval %s is not pending", id)
	}
	return s.GetApproval(id)
}

// AddAudit records one gated tool call.
func (s *Store) AddAudit(a AuditEntry) error {
	if a.ID == "" {
		a.ID = NewID("audit")
	}
	if a.CreatedAt == "" {
		a.CreatedAt = nowStr()
	}
	_, err := s.DB.Exec(`INSERT INTO audit (id, run_id, task_id, tool, action, reason, command, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, a.ID, a.RunID, a.TaskID, a.Tool, a.Action, a.Reason, a.Command, a.CreatedAt)
	return err
}

// ListAudit returns the newest audit entries for a run.
func (s *Store) ListAudit(runID string, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.Query(`SELECT id, run_id, task_id, tool, action, reason, command, created_at
		FROM audit WHERE run_id=? ORDER BY created_at DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var a AuditEntry
		if err := rows.Scan(&a.ID, &a.RunID, &a.TaskID, &a.Tool, &a.Action, &a.Reason, &a.Command, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
