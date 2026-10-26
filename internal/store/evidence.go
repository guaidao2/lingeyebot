package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AddEvidence stores an immutable evidence body and its index row.
func (s *Store) AddEvidence(e Evidence, content []byte) (Evidence, error) {
	if e.ID == "" {
		e.ID = NewID("ev")
	}
	if e.Kind == "" {
		e.Kind = "text"
	}
	if e.CreatedAt == "" {
		e.CreatedAt = nowStr()
	}
	if e.Layers == nil {
		e.Layers = []string{}
	}
	sum := sha256.Sum256(content)
	e.SHA256 = hex.EncodeToString(sum[:])
	e.Size = int64(len(content))
	if len(content) > 0 {
		rel := filepath.Join("blobs", e.SHA256[:2], e.SHA256+".bin")
		full := filepath.Join(s.EvidenceDir, rel)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return e, fmt.Errorf("create evidence dir: %w", err)
			}
			tmp := full + ".tmp"
			if err := os.WriteFile(tmp, content, 0o600); err != nil {
				return e, fmt.Errorf("write evidence: %w", err)
			}
			if err := os.Rename(tmp, full); err != nil {
				return e, fmt.Errorf("commit evidence: %w", err)
			}
		}
		e.RefPath = rel
	}
	_, err := s.DB.Exec(`INSERT INTO evidence (id, run_id, kind, label, sha256, size, channel, layers, ref_path, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`, e.ID, e.RunID, e.Kind, e.Label, e.SHA256, e.Size,
		e.Channel, mustJSON(e.Layers), e.RefPath, e.CreatedAt)
	if err != nil {
		return e, fmt.Errorf("insert evidence: %w", err)
	}
	return e, nil
}

// ReadEvidence returns the immutable body of an evidence record.
func (s *Store) ReadEvidence(id string) ([]byte, error) {
	e, err := s.GetEvidence(id)
	if err != nil {
		return nil, err
	}
	if e.RefPath == "" {
		return nil, nil
	}
	return os.ReadFile(filepath.Join(s.EvidenceDir, e.RefPath))
}

// GetEvidence reads one evidence row.
func (s *Store) GetEvidence(id string) (Evidence, error) {
	var e Evidence
	var layers string
	err := s.DB.QueryRow(`SELECT id, run_id, kind, label, sha256, size, channel, layers, ref_path, created_at
		FROM evidence WHERE id=?`, id).Scan(&e.ID, &e.RunID, &e.Kind, &e.Label, &e.SHA256,
		&e.Size, &e.Channel, &layers, &e.RefPath, &e.CreatedAt)
	if err != nil {
		return e, err
	}
	_ = json.Unmarshal([]byte(layers), &e.Layers)
	return e, nil
}

// ListEvidence returns evidence for a run.
func (s *Store) ListEvidence(runID string) ([]Evidence, error) {
	rows, err := s.DB.Query(`SELECT id, run_id, kind, label, sha256, size, channel, layers, ref_path, created_at
		FROM evidence WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Evidence{}
	for rows.Next() {
		var e Evidence
		var layers string
		if err := rows.Scan(&e.ID, &e.RunID, &e.Kind, &e.Label, &e.SHA256, &e.Size, &e.Channel, &layers, &e.RefPath, &e.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(layers), &e.Layers)
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddFlow records a concat point.
func (s *Store) AddFlow(f Flow) (Flow, error) {
	if f.ID == "" {
		f.ID = NewID("flow")
	}
	if f.CreatedAt == "" {
		f.CreatedAt = nowStr()
	}
	if f.Confidence <= 0 {
		f.Confidence = 0.6
	}
	if f.Evidence == nil {
		f.Evidence = []string{}
	}
	_, err := s.DB.Exec(`INSERT INTO flows (id, run_id, source, transform, sink, confidence, evidence, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, f.ID, f.RunID, f.Source, f.Transform, f.Sink, f.Confidence, mustJSON(f.Evidence), f.CreatedAt)
	return f, err
}

// ListFlows returns dataflows for a run.
func (s *Store) ListFlows(runID string) ([]Flow, error) {
	rows, err := s.DB.Query(`SELECT id, run_id, source, transform, sink, confidence, evidence, created_at
		FROM flows WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Flow{}
	for rows.Next() {
		var f Flow
		var evidence string
		if err := rows.Scan(&f.ID, &f.RunID, &f.Source, &f.Transform, &f.Sink, &f.Confidence, &evidence, &f.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(evidence), &f.Evidence)
		out = append(out, f)
	}
	return out, rows.Err()
}

// AddRetest creates an independent retest record.
func (s *Store) AddRetest(r Retest) (Retest, error) {
	if r.ID == "" {
		r.ID = NewID("retest")
	}
	if r.Status == "" {
		r.Status = "pending"
	}
	if r.Evidence == nil {
		r.Evidence = []string{}
	}
	r.CreatedAt = nowStr()
	_, err := s.DB.Exec(`INSERT INTO retests (id, run_id, finding_id, status, verdict, notes, evidence, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, r.ID, r.RunID, r.FindingID, r.Status, r.Verdict, r.Notes, mustJSON(r.Evidence), r.CreatedAt)
	return r, err
}

// FinishRetest records a retest verdict.
func (s *Store) FinishRetest(id, verdict, notes string, evidence []string) (Retest, error) {
	if verdict != "reproduced" && verdict != "fixed" && verdict != "inconclusive" {
		return Retest{}, fmt.Errorf("invalid verdict %q", verdict)
	}
	now := nowStr()
	_, err := s.DB.Exec(`UPDATE retests SET status='completed', verdict=?, notes=?, evidence=?, finished_at=? WHERE id=?`,
		verdict, notes, mustJSON(evidence), now, id)
	if err != nil {
		return Retest{}, err
	}
	var r Retest
	var ev string
	var finished sql.NullString
	err = s.DB.QueryRow(`SELECT id, run_id, finding_id, status, verdict, notes, evidence, created_at, finished_at
		FROM retests WHERE id=?`, id).Scan(&r.ID, &r.RunID, &r.FindingID, &r.Status, &r.Verdict, &r.Notes, &ev, &r.CreatedAt, &finished)
	r.FinishedAt = finished.String
	_ = json.Unmarshal([]byte(ev), &r.Evidence)
	return r, err
}

// ListRetests returns retests for a run.
func (s *Store) ListRetests(runID string) ([]Retest, error) {
	rows, err := s.DB.Query(`SELECT id, run_id, finding_id, status, verdict, notes, evidence, created_at, finished_at
		FROM retests WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Retest{}
	for rows.Next() {
		var r Retest
		var ev string
		var finished sql.NullString
		if err := rows.Scan(&r.ID, &r.RunID, &r.FindingID, &r.Status, &r.Verdict, &r.Notes, &ev, &r.CreatedAt, &finished); err != nil {
			return nil, err
		}
		r.FinishedAt = finished.String
		_ = json.Unmarshal([]byte(ev), &r.Evidence)
		out = append(out, r)
	}
	return out, rows.Err()
}
