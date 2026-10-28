package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// EnsureSession returns the run's conversation session, creating it if needed.
func (s *Store) EnsureSession(runID string) (Session, error) {
	return s.EnsureSessionKey(runID, "run:"+runID)
}

// EnsureSessionKey returns a named session bound to a run.
func (s *Store) EnsureSessionKey(runID, key string) (Session, error) {
	var sess Session
	err := s.DB.QueryRow(`SELECT id, run_id, key, generation, created_at FROM sessions WHERE key=?`, key).
		Scan(&sess.ID, &sess.RunID, &sess.Key, &sess.Generation, &sess.CreatedAt)
	if err == nil {
		return sess, nil
	}
	if !IsNotFound(err) {
		return sess, err
	}
	sess = Session{ID: NewID("sess"), RunID: runID, Key: key, Generation: 1, CreatedAt: nowStr()}
	_, err = s.DB.Exec(`INSERT INTO sessions (id, run_id, key, generation, created_at) VALUES (?,?,?,?,?)`,
		sess.ID, sess.RunID, sess.Key, sess.Generation, sess.CreatedAt)
	if err != nil {
		return sess, fmt.Errorf("create session: %w", err)
	}
	return sess, nil
}

// AddMessage appends one conversation message.
func (s *Store) AddMessage(m Message) (Message, error) {
	if m.ID == "" {
		m.ID = NewID("msg")
	}
	if m.CreatedAt == "" {
		m.CreatedAt = nowStr()
	}
	if len(m.ToolCalls) == 0 {
		m.ToolCalls = json.RawMessage("[]")
	}
	_, err := s.DB.Exec(`INSERT INTO messages (id, session_id, run_id, role, content, reasoning, reasoning_signature, tool_calls, tool_call_id, name, visible, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, m.ID, m.SessionID, m.RunID, m.Role, m.Content, m.Reasoning, m.ReasoningSignature, string(m.ToolCalls),
		m.ToolCallID, m.Name, boolInt(m.Visible), m.CreatedAt)
	if err != nil {
		return m, fmt.Errorf("insert message: %w", err)
	}
	return m, nil
}

// ListMessages returns messages in chronological order.
func (s *Store) ListMessages(sessionID string, limit int) ([]Message, error) {
	query := `SELECT id, session_id, run_id, role, content, reasoning, reasoning_signature, tool_calls, tool_call_id, name, visible, created_at
		FROM messages WHERE session_id=? ORDER BY created_at, rowid`
	args := []any{sessionID}
	if limit > 0 {
		query = `SELECT id, session_id, run_id, role, content, reasoning, reasoning_signature, tool_calls, tool_call_id, name, visible, created_at
			FROM messages WHERE session_id=? ORDER BY created_at DESC, rowid DESC LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		var calls string
		var visible int
		if err := rows.Scan(&m.ID, &m.SessionID, &m.RunID, &m.Role, &m.Content, &m.Reasoning, &m.ReasoningSignature, &calls, &m.ToolCallID, &m.Name, &visible, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.ToolCalls = json.RawMessage(calls)
		m.Visible = visible == 1
		out = append(out, m)
	}
	if limit > 0 {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, rows.Err()
}

// SetMemory upserts a long-term memory value.
func (s *Store) SetMemory(scope, key, value string) error {
	if scope == "" {
		scope = "global"
	}
	_, err := s.DB.Exec(`INSERT INTO memory (id, scope, key, value, updated_at) VALUES (?,?,?,?,?)
		ON CONFLICT(scope, key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		NewID("mem"), scope, key, value, nowStr())
	return err
}

// ListMemory returns memory entries for a scope.
func (s *Store) ListMemory(scope string) (map[string]string, error) {
	rows, err := s.DB.Query(`SELECT key, value FROM memory WHERE scope=? ORDER BY updated_at DESC`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// AddSummary stores a compaction summary.
func (s *Store) AddSummary(sessionID string, cursor int, content string) error {
	_, err := s.DB.Exec(`INSERT INTO summaries (id, session_id, cursor, content, created_at) VALUES (?,?,?,?,?)`,
		NewID("sum"), sessionID, cursor, content, nowStr())
	return err
}

// LatestSummary returns the most recent compaction summary for a session.
func (s *Store) LatestSummary(sessionID string) (string, int, error) {
	var content string
	var cursor int
	err := s.DB.QueryRow(`SELECT content, cursor FROM summaries WHERE session_id=? ORDER BY created_at DESC LIMIT 1`, sessionID).
		Scan(&content, &cursor)
	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	return content, cursor, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
