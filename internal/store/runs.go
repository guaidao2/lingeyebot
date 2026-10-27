package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

const runColumns = `id, project_id, title, mode, target, objective, authorization, scope, status, provider_id, model, goal_mode, auto_turns, last_user_at, created_at, updated_at`

// CreateRun inserts a conversation and, when goal mode is enabled, its first
// goal. A nil GoalMode preserves legacy behaviour (enabled).
func (s *Store) CreateRun(run Run) (Run, Goal, error) {
	if run.ID == "" {
		run.ID = NewID("run")
	}
	goalMode := 1
	if run.GoalMode != nil && !*run.GoalMode {
		goalMode = 0
	}
	if run.GoalMode == nil {
		enabled := true
		run.GoalMode = &enabled
	}
	if run.ProjectID == "" {
		project, err := s.EnsureDefaultProject()
		if err != nil {
			return run, Goal{}, fmt.Errorf("ensure default project: %w", err)
		}
		run.ProjectID = project.ID
	}
	if run.Title == "" {
		run.Title = strings.TrimSpace(run.Objective)
		if run.Title == "" {
			run.Title = strings.TrimSpace(run.Target)
		}
		if run.Title == "" {
			run.Title = "新任务"
		}
	}
	now := nowStr()
	run.CreatedAt, run.UpdatedAt = now, now
	if run.Status == "" {
		run.Status = "active"
	}
	_, err := s.DB.Exec(`INSERT INTO runs (`+runColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		run.ID, run.ProjectID, run.Title, run.Mode, run.Target, run.Objective, run.Authorization, run.Scope,
		run.Status, run.ProviderID, run.Model, goalMode, 0, "", run.CreatedAt, run.UpdatedAt)
	if err != nil {
		return run, Goal{}, fmt.Errorf("insert run: %w", err)
	}
	if goalMode == 0 {
		return run, Goal{}, nil
	}
	goal, err := s.AddGoal(Goal{RunID: run.ID, Text: run.Objective, Acceptance: ""})
	if err != nil {
		return run, goal, err
	}
	return run, goal, nil
}

// AddGoal appends one goal to a run.
func (s *Store) AddGoal(g Goal) (Goal, error) {
	if g.ID == "" {
		g.ID = NewID("goal")
	}
	now := nowStr()
	g.CreatedAt, g.UpdatedAt = now, now
	if g.Status == "" {
		g.Status = "open"
	}
	if g.Evidence == nil {
		g.Evidence = []string{}
	}
	_, err := s.DB.Exec(`INSERT INTO goals (id, run_id, text, acceptance, status, evidence, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`, g.ID, g.RunID, g.Text, g.Acceptance, g.Status, mustJSON(g.Evidence), g.CreatedAt, g.UpdatedAt)
	if err != nil {
		return g, fmt.Errorf("insert goal: %w", err)
	}
	return g, nil
}

// ListGoals returns goals for a run.
func (s *Store) ListGoals(runID string) ([]Goal, error) {
	rows, err := s.DB.Query(`SELECT id, run_id, text, acceptance, status, evidence, created_at, updated_at
		FROM goals WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Goal{}
	for rows.Next() {
		var g Goal
		var evidence string
		if err := rows.Scan(&g.ID, &g.RunID, &g.Text, &g.Acceptance, &g.Status, &evidence, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(evidence), &g.Evidence)
		if g.Evidence == nil {
			g.Evidence = []string{}
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ProveGoal marks a goal met or abandoned atomically.
func (s *Store) ProveGoal(goalID string, evidenceIDs []string, status string) (Goal, error) {
	if status != "abandoned" {
		status = "met"
	}
	var g Goal
	err := s.WithTx(func(tx *sql.Tx) error {
		read, err := scanGoal(tx.QueryRow(`SELECT id, run_id, text, acceptance, status, evidence, created_at, updated_at
			FROM goals WHERE id=?`, goalID))
		if err != nil {
			return err
		}
		g = read
		g.Status = status
		g.UpdatedAt = nowStr()
		seen := map[string]bool{}
		for _, id := range g.Evidence {
			seen[id] = true
		}
		for _, id := range evidenceIDs {
			if id != "" && !seen[id] {
				g.Evidence = append(g.Evidence, id)
				seen[id] = true
			}
		}
		_, err = tx.Exec(`UPDATE goals SET status=?, evidence=?, updated_at=? WHERE id=?`,
			g.Status, mustJSON(g.Evidence), g.UpdatedAt, g.ID)
		return err
	})
	return g, err
}

// GetGoal reads one goal.
func (s *Store) GetGoal(id string) (Goal, error) {
	return scanGoal(s.DB.QueryRow(`SELECT id, run_id, text, acceptance, status, evidence, created_at, updated_at
		FROM goals WHERE id=?`, id))
}

type goalScanner interface {
	Scan(dest ...any) error
}

func scanGoal(row goalScanner) (Goal, error) {
	var g Goal
	var evidence string
	err := row.Scan(&g.ID, &g.RunID, &g.Text, &g.Acceptance, &g.Status, &evidence, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return g, err
	}
	_ = json.Unmarshal([]byte(evidence), &g.Evidence)
	if g.Evidence == nil {
		g.Evidence = []string{}
	}
	return g, nil
}

// ListRunsByProject returns conversations for one project, newest first.
func (s *Store) ListRunsByProject(projectID string, limit int) ([]Run, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.Query(`SELECT `+runColumns+` FROM runs WHERE project_id=? ORDER BY created_at DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRuns returns recent runs, newest first.
func (s *Store) ListRuns(limit int) ([]Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.DB.Query(`SELECT `+runColumns+` FROM runs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRun reads one run.
func (s *Store) GetRun(id string) (Run, error) {
	row := s.DB.QueryRow(`SELECT `+runColumns+` FROM runs WHERE id=?`, id)
	return scanRun(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRun(row rowScanner) (Run, error) {
	var r Run
	var goalMode int
	err := row.Scan(&r.ID, &r.ProjectID, &r.Title, &r.Mode, &r.Target, &r.Objective, &r.Authorization, &r.Scope,
		&r.Status, &r.ProviderID, &r.Model, &goalMode, &r.AutoTurns, &r.LastUserAt, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, err
	}
	enabled := goalMode != 0
	r.GoalMode = &enabled
	return r, nil
}

// UpdateRunStatus changes a run's status.
func (s *Store) UpdateRunStatus(id, status string) error {
	_, err := s.DB.Exec(`UPDATE runs SET status=?, updated_at=? WHERE id=?`, status, nowStr(), id)
	return err
}

// MarkRunAchievedIfActive sets achieved only if no Stop/pause won the race.
func (s *Store) MarkRunAchievedIfActive(id string) (bool, error) {
	res, err := s.DB.Exec(`UPDATE runs SET status='achieved', updated_at=? WHERE id=? AND status='active'`, nowStr(), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RecoverInterrupted closes tasks and pauses runs left by a previous process.
// It is called once at server startup, not from every Store.Open.
func (s *Store) RecoverInterrupted() error {
	if err := s.RecoverInterruptedTasks(); err != nil {
		return err
	}
	return s.RecoverInterruptedRuns()
}

// RecoverInterruptedRuns pauses runs left active by a previous process. There
// is no live turn goroutine after restart, so leaving them active would make the
// UI show a phantom running task forever. A control event makes the pause
// observable instead of silently changing status.
func (s *Store) RecoverInterruptedRuns() error {
	rows, err := s.DB.Query(`SELECT id FROM runs WHERE status='active'`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		if err := s.UpdateRunStatus(id, "paused"); err != nil {
			return err
		}
		_, _ = s.AddEvent(Event{
			RunID: id, Kind: "control", Summary: "进程重启，任务已暂停",
			Detail: "恢复后请检查子代理/继续任务",
		})
	}
	return nil
}

// ActivateRunIfAchieved reactivates a completed run when a subagent report
// arrives, without overwriting a user pause/stall.
func (s *Store) ActivateRunIfAchieved(id string) (bool, error) {
	res, err := s.DB.Exec(`UPDATE runs SET status='active', updated_at=? WHERE id=? AND status='achieved'`, nowStr(), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SetRunModel records which provider/model the run uses.
func (s *Store) SetRunModel(id, providerID, model string) error {
	_, err := s.DB.Exec(`UPDATE runs SET provider_id=?, model=?, updated_at=? WHERE id=?`, providerID, model, nowStr(), id)
	return err
}

// AddConstraint appends one allow/deny constraint.
func (s *Store) AddConstraint(c Constraint) (Constraint, error) {
	if c.ID == "" {
		c.ID = NewID("con")
	}
	if c.Kind != "allow" {
		c.Kind = "deny"
	}
	if c.Origin == "" {
		c.Origin = "user"
	}
	c.Active = true
	c.CreatedAt = nowStr()
	_, err := s.DB.Exec(`INSERT INTO constraints (id, run_id, kind, text, origin, active, created_at)
		VALUES (?,?,?,?,?,1,?)`, c.ID, c.RunID, c.Kind, c.Text, c.Origin, c.CreatedAt)
	return c, err
}

// ListConstraints returns active constraints for a run.
func (s *Store) ListConstraints(runID string) ([]Constraint, error) {
	rows, err := s.DB.Query(`SELECT id, run_id, kind, text, origin, active, created_at
		FROM constraints WHERE run_id=? AND active=1 ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Constraint
	for rows.Next() {
		var c Constraint
		var active int
		if err := rows.Scan(&c.ID, &c.RunID, &c.Kind, &c.Text, &c.Origin, &active, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Active = active == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// GoalsSettled reports whether every goal is met or abandoned.
func (s *Store) GoalsSettled(runID string) (bool, error) {
	goals, err := s.ListGoals(runID)
	if err != nil {
		return false, err
	}
	if len(goals) == 0 {
		return false, nil
	}
	for _, g := range goals {
		if g.Status == "open" {
			return false, nil
		}
	}
	return true, nil
}

// HasOpenGoals reports whether at least one goal is still open.
func (s *Store) HasOpenGoals(runID string) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM goals WHERE run_id=? AND status='open'`, runID).Scan(&n)
	return n > 0, err
}

var _ = sql.ErrNoRows

// TouchRun records the last user interaction.
func (s *Store) TouchRun(id string) error {
	_, err := s.DB.Exec(`UPDATE runs SET last_user_at=?, updated_at=? WHERE id=?`, nowStr(), nowStr(), id)
	return err
}

// IncrementAutoTurns counts one supervisor-initiated continuation.
func (s *Store) IncrementAutoTurns(id string) (int, error) {
	if _, err := s.DB.Exec(`UPDATE runs SET auto_turns=auto_turns+1, updated_at=? WHERE id=?`, nowStr(), id); err != nil {
		return 0, err
	}
	var n int
	err := s.DB.QueryRow(`SELECT auto_turns FROM runs WHERE id=?`, id).Scan(&n)
	return n, err
}

// ResetAutoTurns clears the continuation counter after user input.
func (s *Store) ResetAutoTurns(id string) error {
	_, err := s.DB.Exec(`UPDATE runs SET auto_turns=0, updated_at=? WHERE id=?`, nowStr(), id)
	return err
}

// SetGoalMode toggles a run's goal-driven behaviour.
func (s *Store) SetGoalMode(id string, enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	_, err := s.DB.Exec(`UPDATE runs SET goal_mode=?, updated_at=? WHERE id=?`, value, nowStr(), id)
	return err
}
