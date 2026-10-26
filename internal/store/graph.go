package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// AddNode inserts one exploration node.
func (s *Store) AddNode(n Node) (Node, error) {
	if n.ID == "" {
		n.ID = NewID("n")
	}
	now := nowStr()
	if n.CreatedAt == "" {
		n.CreatedAt = now
	}
	n.UpdatedAt = now
	if n.State == "" {
		n.State = defaultState(n.Kind)
	}
	if len(n.Payload) == 0 {
		n.Payload = json.RawMessage("{}")
	}
	_, err := s.DB.Exec(`INSERT INTO nodes (id, run_id, kind, payload, state, priority, origin, owner, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		n.ID, n.RunID, n.Kind, string(n.Payload), n.State, n.Priority, n.Origin, n.Owner, n.CreatedAt, n.UpdatedAt)
	if err != nil {
		return n, fmt.Errorf("insert node: %w", err)
	}
	return n, nil
}

func defaultState(kind string) string {
	switch kind {
	case "fact", "finding":
		return "confirmed"
	case "hint", "digest":
		return "active"
	case "attack_path":
		return "suspected"
	default:
		return "open"
	}
}

// GetNode reads one node.
func (s *Store) GetNode(id string) (Node, error) {
	var n Node
	var payload string
	var completed sql.NullString
	err := s.DB.QueryRow(`SELECT id, run_id, kind, payload, state, priority, origin, owner, created_at, updated_at, completed_at
		FROM nodes WHERE id=?`, id).Scan(&n.ID, &n.RunID, &n.Kind, &payload, &n.State, &n.Priority,
		&n.Origin, &n.Owner, &n.CreatedAt, &n.UpdatedAt, &completed)
	if err != nil {
		return n, err
	}
	n.Payload = json.RawMessage(payload)
	n.CompletedAt = completed.String
	return n, nil
}

// ListNodes returns nodes for a run, optionally filtered by kind.
func (s *Store) ListNodes(runID, kind string) ([]Node, error) {
	query := `SELECT id, run_id, kind, payload, state, priority, origin, owner, created_at, updated_at, completed_at
		FROM nodes WHERE run_id=?`
	args := []any{runID}
	if kind != "" {
		query += ` AND kind=?`
		args = append(args, kind)
	}
	query += ` ORDER BY created_at`
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Node{}
	for rows.Next() {
		var n Node
		var payload string
		var completed sql.NullString
		if err := rows.Scan(&n.ID, &n.RunID, &n.Kind, &payload, &n.State, &n.Priority,
			&n.Origin, &n.Owner, &n.CreatedAt, &n.UpdatedAt, &completed); err != nil {
			return nil, err
		}
		n.Payload = json.RawMessage(payload)
		n.CompletedAt = completed.String
		out = append(out, n)
	}
	return out, rows.Err()
}

// UpdateNodeState changes state, priority and optionally payload.
func (s *Store) UpdateNodeState(id, state string, priority int) error {
	now := nowStr()
	completed := ""
	switch state {
	case "done", "blocked", "exhausted", "stopped", "met", "abandoned", "deleted":
		completed = now
	}
	_, err := s.DB.Exec(`UPDATE nodes SET state=?, priority=?, updated_at=?, completed_at=CASE WHEN ?<>'' THEN ? ELSE completed_at END
		WHERE id=?`, state, priority, now, completed, completed, id)
	return err
}

// SetNodeState updates a node's state without changing its priority.
func (s *Store) SetNodeState(id, state string) error {
	now := nowStr()
	completed := ""
	switch state {
	case "fixed", "ignored", "false_positive", "done", "blocked", "met", "abandoned":
		completed = now
	}
	result, err := s.DB.Exec(`UPDATE nodes SET state=?, updated_at=?, completed_at=CASE WHEN ?<>'' THEN ? ELSE NULL END
		WHERE id=?`, state, now, completed, completed, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateNodePayload replaces a node's payload.
func (s *Store) UpdateNodePayload(id string, payload any) error {
	_, err := s.DB.Exec(`UPDATE nodes SET payload=?, updated_at=? WHERE id=?`, mustJSON(payload), nowStr(), id)
	return err
}

// AddEdge inserts one graph edge, ignoring duplicates.
func (s *Store) AddEdge(runID, srcID, dstID, rel string) (string, error) {
	id := NewID("e")
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO edges (id, run_id, src_id, dst_id, rel, created_at)
		VALUES (?,?,?,?,?,?)`, id, runID, srcID, dstID, rel, nowStr())
	return id, err
}

// ListEdges returns edges for a run.
func (s *Store) ListEdges(runID string) ([]Edge, error) {
	rows, err := s.DB.Query(`SELECT id, run_id, src_id, dst_id, rel, created_at FROM edges WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Edge{}
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.ID, &e.RunID, &e.SrcID, &e.DstID, &e.Rel, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Anchor links a node to an asset.
func (s *Store) Anchor(nodeID, assetID string) error {
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO anchors (node_id, asset_id) VALUES (?,?)`, nodeID, assetID)
	return err
}

// ListAnchors returns anchors for a run.
func (s *Store) ListAnchors(runID string) ([]Anchor, error) {
	rows, err := s.DB.Query(`SELECT a.node_id, a.asset_id FROM anchors a
		JOIN nodes n ON n.id = a.node_id WHERE n.run_id=?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Anchor{}
	for rows.Next() {
		var a Anchor
		if err := rows.Scan(&a.NodeID, &a.AssetID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// NodesByIDs loads nodes by id, preserving no particular order.
func (s *Store) NodesByIDs(ids []string) (map[string]Node, error) {
	out := map[string]Node{}
	for _, id := range ids {
		if id == "" {
			continue
		}
		n, err := s.GetNode(id)
		if err != nil {
			continue
		}
		out[id] = n
	}
	return out, nil
}
