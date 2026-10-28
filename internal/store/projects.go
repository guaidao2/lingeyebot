package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

const defaultProjectID = "proj-default"

// migrate upgrades databases created before the project model existed.
func (s *Store) migrate() error {
	if err := s.ensureColumn("runs", "project_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("runs", "title", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("messages", "reasoning", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("messages", "reasoning_signature", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("runs", "goal_mode", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.ensureColumn("runs", "pause_reason", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_runs_project ON runs(project_id, created_at DESC)`); err != nil {
		return err
	}
	project, err := s.EnsureDefaultProject()
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(`UPDATE runs SET project_id=? WHERE project_id=''`, project.ID); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`UPDATE runs SET title=objective WHERE title=''`); err != nil {
		return err
	}
	if err := s.migrateEdges(); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureColumn(table, column, ddl string) error {
	rows, err := s.DB.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			rows.Close()
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	_, err = s.DB.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, ddl))
	return err
}

// EnsureDefaultProject creates the migration project when missing.
func (s *Store) EnsureDefaultProject() (Project, error) {
	if project, err := s.GetProject(defaultProjectID); err == nil {
		return project, nil
	}
	now := nowStr()
	project := Project{
		ID: defaultProjectID, Name: "默认项目", NKey: "default",
		Description: "迁移自旧任务；可以重命名或新建项目。",
		Status:      "active", CreatedAt: now, UpdatedAt: now,
	}
	_, err := s.DB.Exec(`INSERT INTO projects (id, name, nkey, description, authorization, scope, status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		project.ID, project.Name, project.NKey, project.Description, "", "", project.Status, project.CreatedAt, project.UpdatedAt)
	if err != nil {
		// Another writer may have won the race; re-read.
		if existing, getErr := s.GetProject(defaultProjectID); getErr == nil {
			return existing, nil
		}
		return Project{}, err
	}
	return project, nil
}

// CreateProject inserts a project with a normalized unique key.
func (s *Store) CreateProject(p Project) (Project, error) {
	if p.ID == "" {
		p.ID = NewID("proj")
	}
	if strings.TrimSpace(p.Name) == "" {
		p.Name = "未命名项目"
	}
	if strings.TrimSpace(p.NKey) == "" {
		p.NKey = slug(p.Name) + "-" + idSuffix(p.ID)
	}
	p.NKey = slug(p.NKey)
	if p.NKey == "" {
		p.NKey = "project-" + idSuffix(p.ID)
	}
	if p.Status == "" {
		p.Status = "active"
	}
	now := nowStr()
	p.CreatedAt, p.UpdatedAt = now, now
	_, err := s.DB.Exec(`INSERT INTO projects (id, name, nkey, description, authorization, scope, status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.NKey, p.Description, p.Authorization, p.Scope, p.Status, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return p, fmt.Errorf("insert project: %w", err)
	}
	return p, nil
}

// GetProject reads one project.
func (s *Store) GetProject(id string) (Project, error) {
	var p Project
	err := s.DB.QueryRow(`SELECT id, name, nkey, description, authorization, scope, status, created_at, updated_at
		FROM projects WHERE id=?`, id).Scan(&p.ID, &p.Name, &p.NKey, &p.Description, &p.Authorization,
		&p.Scope, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// UpdateProject updates editable project fields.
func (s *Store) UpdateProject(p Project) (Project, error) {
	if strings.TrimSpace(p.Name) == "" {
		return p, fmt.Errorf("project name is required")
	}
	p.UpdatedAt = nowStr()
	_, err := s.DB.Exec(`UPDATE projects SET name=?, description=?, authorization=?, scope=?, status=?, updated_at=? WHERE id=?`,
		p.Name, p.Description, p.Authorization, p.Scope, p.Status, p.UpdatedAt, p.ID)
	if err != nil {
		return p, err
	}
	return s.GetProject(p.ID)
}

// ListProjects returns projects with live counts.
func (s *Store) ListProjects() ([]ProjectSummary, error) {
	rows, err := s.DB.Query(`SELECT id, name, nkey, description, authorization, scope, status, created_at, updated_at
		FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.NKey, &p.Description, &p.Authorization, &p.Scope,
			&p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	out := []ProjectSummary{}
	for _, p := range projects {
		summary := ProjectSummary{Project: p}
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM runs WHERE project_id=?`, p.ID).Scan(&summary.Conversations)
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM nodes n JOIN runs r ON r.id=n.run_id WHERE r.project_id=? AND n.kind='finding'`, p.ID).Scan(&summary.Findings)
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM goals g JOIN runs r ON r.id=g.run_id WHERE r.project_id=? AND g.status='open'`, p.ID).Scan(&summary.OpenGoals)
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM project_assets WHERE project_id=?`, p.ID).Scan(&summary.Assets)
		var last sql.NullString
		_ = s.DB.QueryRow(`SELECT MAX(updated_at) FROM runs WHERE project_id=?`, p.ID).Scan(&last)
		if last.Valid {
			summary.LastActivity = last.String
		} else {
			summary.LastActivity = p.UpdatedAt
		}
		out = append(out, summary)
	}
	return out, nil
}

// ProjectDetail assembles the project workspace.
func (s *Store) ProjectDetail(projectID string) (ProjectDetail, error) {
	project, err := s.GetProject(projectID)
	if err != nil {
		return ProjectDetail{}, err
	}
	conversations, err := s.ListRunsByProject(projectID, 100)
	if err != nil {
		return ProjectDetail{}, err
	}
	assets, err := s.ListProjectAssets(projectID)
	if err != nil {
		return ProjectDetail{}, err
	}
	board, err := s.ListProjectNodes(projectID, 200)
	if err != nil {
		return ProjectDetail{}, err
	}
	edges, err := s.ListProjectEdges(projectID)
	if err != nil {
		return ProjectDetail{}, err
	}
	memory, err := s.ListProjectMemory(projectID, 50)
	if err != nil {
		return ProjectDetail{}, err
	}
	findings, err := s.ListProjectFindings(projectID, 200)
	if err != nil {
		return ProjectDetail{}, err
	}
	coverage, err := s.ProjectCoverage(projectID)
	if err != nil {
		return ProjectDetail{}, err
	}
	return ProjectDetail{
		Project: project, Conversations: conversations, Assets: assets, Board: board,
		Edges: edges, Memory: memory, Findings: findings, Coverage: coverage,
	}, nil
}

// AddProjectNode appends one shared blackboard entry.
func (s *Store) AddProjectNode(n ProjectNode) (ProjectNode, error) {
	if n.ID == "" {
		n.ID = NewID("pn")
	}
	if n.Kind == "" {
		n.Kind = "note"
	}
	if n.State == "" {
		n.State = "active"
	}
	if n.Visibility == "" {
		n.Visibility = "project"
	}
	if len(n.Payload) == 0 {
		n.Payload = []byte("{}")
	}
	now := nowStr()
	n.CreatedAt, n.UpdatedAt = now, now
	_, err := s.DB.Exec(`INSERT INTO project_nodes
		(id, project_id, conversation_id, kind, payload, state, priority, pinned, visibility, origin, owner, source_node_id, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		n.ID, n.ProjectID, n.ConversationID, n.Kind, string(n.Payload), n.State, n.Priority,
		boolInt(n.Pinned), n.Visibility, n.Origin, n.Owner, n.SourceNodeID, n.CreatedAt, n.UpdatedAt)
	return n, err
}

// ListProjectNodes returns shared blackboard entries, pinned first.
func (s *Store) ListProjectNodes(projectID string, limit int) ([]ProjectNode, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.DB.Query(`SELECT id, project_id, conversation_id, kind, payload, state, priority, pinned,
		visibility, origin, owner, source_node_id, created_at, updated_at
		FROM project_nodes WHERE project_id=? ORDER BY pinned DESC, created_at DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectNode{}
	for rows.Next() {
		var n ProjectNode
		var payload string
		var pinned int
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.ConversationID, &n.Kind, &payload, &n.State,
			&n.Priority, &pinned, &n.Visibility, &n.Origin, &n.Owner, &n.SourceNodeID, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		n.Payload = []byte(payload)
		n.Pinned = pinned == 1
		out = append(out, n)
	}
	return out, rows.Err()
}

// AddProjectEdge links two blackboard nodes.
func (s *Store) AddProjectEdge(projectID, srcID, dstID, rel string) (string, error) {
	id := NewID("pe")
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO project_edges (id, project_id, src_id, dst_id, rel, created_at)
		VALUES (?,?,?,?,?,?)`, id, projectID, srcID, dstID, rel, nowStr())
	return id, err
}

// ListProjectEdges returns blackboard edges.
func (s *Store) ListProjectEdges(projectID string) ([]ProjectEdge, error) {
	rows, err := s.DB.Query(`SELECT id, project_id, src_id, dst_id, rel, created_at
		FROM project_edges WHERE project_id=? ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectEdge{}
	for rows.Next() {
		var e ProjectEdge
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.SrcID, &e.DstID, &e.Rel, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SetProjectMemory upserts compact shared knowledge.
func (s *Store) SetProjectMemory(projectID, key, value, kind, sourceNodeID, updatedBy string) (ProjectMemory, error) {
	if strings.TrimSpace(key) == "" {
		return ProjectMemory{}, fmt.Errorf("memory key is required")
	}
	if kind == "" {
		kind = "fact"
	}
	now := nowStr()
	m := ProjectMemory{
		ID: NewID("pm"), ProjectID: projectID, Key: key, Value: value, Kind: kind,
		SourceNodeID: sourceNodeID, UpdatedBy: updatedBy, UpdatedAt: now,
	}
	_, err := s.DB.Exec(`INSERT INTO project_memory (id, project_id, key, value, kind, source_node_id, updated_by, updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(project_id, key) DO UPDATE SET value=excluded.value, kind=excluded.kind,
		source_node_id=excluded.source_node_id, updated_by=excluded.updated_by, updated_at=excluded.updated_at`,
		m.ID, m.ProjectID, m.Key, m.Value, m.Kind, m.SourceNodeID, m.UpdatedBy, m.UpdatedAt)
	if err != nil {
		return m, err
	}
	row := s.DB.QueryRow(`SELECT id, project_id, key, value, kind, source_node_id, updated_by, updated_at
		FROM project_memory WHERE project_id=? AND key=?`, projectID, key)
	err = row.Scan(&m.ID, &m.ProjectID, &m.Key, &m.Value, &m.Kind, &m.SourceNodeID, &m.UpdatedBy, &m.UpdatedAt)
	return m, err
}

// ListProjectMemory returns compact shared knowledge.
func (s *Store) ListProjectMemory(projectID string, limit int) ([]ProjectMemory, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.DB.Query(`SELECT id, project_id, key, value, kind, source_node_id, updated_by, updated_at
		FROM project_memory WHERE project_id=? ORDER BY updated_at DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectMemory{}
	for rows.Next() {
		var m ProjectMemory
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Key, &m.Value, &m.Kind, &m.SourceNodeID, &m.UpdatedBy, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// LinkProjectAsset associates a global asset with a project.
func (s *Store) LinkProjectAsset(projectID, assetID, source string) error {
	if source == "" {
		source = "agent"
	}
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO project_assets (project_id, asset_id, source, created_at)
		VALUES (?,?,?,?)`, projectID, assetID, source, nowStr())
	return err
}

// ListProjectAssets returns assets explicitly linked or anchored by the project's conversations.
func (s *Store) ListProjectAssets(projectID string) ([]Asset, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT a.id, a.type, a.value, a.identifier, a.parent_id, a.company, a.meta,
		a.first_seen_run, a.last_seen, a.created_at
		FROM assets a
		LEFT JOIN project_assets pa ON pa.asset_id=a.id AND pa.project_id=?
		LEFT JOIN anchors an ON an.asset_id=a.id
		LEFT JOIN nodes n ON n.id=an.node_id
		LEFT JOIN runs r ON r.id=n.run_id AND r.project_id=?
		LEFT JOIN runs fr ON fr.id=a.first_seen_run AND fr.project_id=?
		WHERE pa.project_id IS NOT NULL OR r.project_id IS NOT NULL OR fr.project_id IS NOT NULL
		ORDER BY a.last_seen DESC`, projectID, projectID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		var a Asset
		var parent sql.NullString
		if err := rows.Scan(&a.ID, &a.Type, &a.Value, &a.Identifier, &parent, &a.Company, &a.Meta,
			&a.FirstSeenRun, &a.LastSeen, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.ParentID = parent.String
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListProjectFindings returns findings across the project's conversations.
func (s *Store) ListProjectFindings(projectID string, limit int) ([]ProjectFinding, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.DB.Query(`SELECT n.id, r.project_id, COALESCE(p.name,''), n.run_id,
		COALESCE(NULLIF(r.title,''), r.objective), n.payload, n.state, n.created_at
		FROM nodes n JOIN runs r ON r.id=n.run_id
		LEFT JOIN projects p ON p.id=r.project_id
		WHERE r.project_id=? AND n.kind='finding'
		ORDER BY n.created_at DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectFinding{}
	for rows.Next() {
		var f ProjectFinding
		var payload string
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.ProjectName, &f.ConversationID, &f.ConversationTitle,
			&payload, &f.State, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.Payload = []byte(payload)
		out = append(out, f)
	}
	return out, rows.Err()
}

// ProjectCoverage marks project assets tested by any conversation's evidenced facts/findings.
func (s *Store) ProjectCoverage(projectID string) (Coverage, error) {
	assets, err := s.ListProjectAssets(projectID)
	if err != nil {
		return Coverage{}, err
	}
	tested := map[string]bool{}
	conversations, err := s.ListRunsByProject(projectID, 500)
	if err == nil {
		for _, conversation := range conversations {
			cov, err := s.Coverage(conversation.ID)
			if err != nil {
				continue
			}
			for _, ac := range cov.Assets {
				if ac.Tested {
					tested[ac.ID] = true
				}
			}
		}
	}
	out := Coverage{Assets: []AssetCoverage{}}
	for _, a := range assets {
		ok := tested[a.ID]
		if ok {
			out.Tested++
		} else {
			out.Untested = append(out.Untested, a)
		}
		out.Assets = append(out.Assets, AssetCoverage{Asset: a, Tested: ok})
	}
	out.Total = len(assets)
	return out, nil
}

// Dashboard returns global counters for the landing page.
func (s *Store) Dashboard() (Dashboard, error) {
	d := Dashboard{
		Severity: map[string]int{}, StatusCounts: map[string]int{},
		ToolCounts: []ToolCount{}, RecentFacts: []ProjectFact{}, RecentEvents: []Event{},
	}
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&d.Projects)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&d.Conversations)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM runs WHERE status='active'`).Scan(&d.Running)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM nodes WHERE kind='finding'`).Scan(&d.Findings)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM goals WHERE status='open'`).Scan(&d.OpenGoals)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM assets`).Scan(&d.Assets)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE kind='tool_call'`).Scan(&d.ToolCalls)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM approvals WHERE status='pending'`).Scan(&d.PendingApprovals)

	rows, err := s.DB.Query(`SELECT state, payload FROM nodes WHERE kind='finding' ORDER BY created_at DESC`)
	if err == nil {
		for rows.Next() {
			var state, payload string
			if err := rows.Scan(&state, &payload); err != nil {
				continue
			}
			if state == "" {
				state = "open"
			}
			d.StatusCounts[state]++
			if state == "fixed" || state == "ignored" || state == "false_positive" {
				continue
			}
			var finding struct {
				Severity string `json:"severity"`
			}
			_ = json.Unmarshal([]byte(payload), &finding)
			if finding.Severity == "" {
				finding.Severity = "info"
			}
			d.Severity[finding.Severity]++
		}
		rows.Close()
	}

	toolRows, err := s.DB.Query(`SELECT COALESCE(NULLIF(tool,''),'unknown') AS tool, COUNT(*)
		FROM events WHERE kind='tool_call' GROUP BY tool ORDER BY 2 DESC LIMIT 8`)
	if err == nil {
		for toolRows.Next() {
			var item ToolCount
			if err := toolRows.Scan(&item.Tool, &item.Count); err == nil {
				d.ToolCounts = append(d.ToolCounts, item)
			}
		}
		toolRows.Close()
	}

	weights := map[string]int{"critical": 10, "high": 5, "medium": 2, "low": 1, "info": 0}
	for severity, count := range d.Severity {
		d.RiskScore += weights[severity] * count
	}

	d.RecentFinding, _ = s.ListAllFindings(8)
	d.RecentFacts, _ = s.ListRecentFacts(6)
	d.RecentEvents, _ = s.ListRecentEvents(10)
	return d, nil
}

// ListRecentFacts returns the newest facts across every conversation.
func (s *Store) ListRecentFacts(limit int) ([]ProjectFact, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.DB.Query(`SELECT n.id, r.project_id, COALESCE(p.name,''), n.run_id,
		COALESCE(NULLIF(r.title,''), r.objective), n.payload, n.state, n.created_at
		FROM nodes n JOIN runs r ON r.id=n.run_id
		LEFT JOIN projects p ON p.id=r.project_id
		WHERE n.kind='fact' ORDER BY n.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectFact{}
	for rows.Next() {
		var f ProjectFact
		var payload string
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.ProjectName, &f.ConversationID, &f.ConversationTitle,
			&payload, &f.State, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.Payload = []byte(payload)
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListAllFindings returns the newest findings across every conversation.
func (s *Store) ListAllFindings(limit int) ([]ProjectFinding, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.DB.Query(`SELECT n.id, r.project_id, COALESCE(p.name,''), n.run_id,
		COALESCE(NULLIF(r.title,''), r.objective), n.payload, n.state, n.created_at
		FROM nodes n JOIN runs r ON r.id=n.run_id
		LEFT JOIN projects p ON p.id=r.project_id
		WHERE n.kind='finding' ORDER BY n.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectFinding{}
	for rows.Next() {
		var f ProjectFinding
		var payload string
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.ProjectName, &f.ConversationID, &f.ConversationTitle,
			&payload, &f.State, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.Payload = []byte(payload)
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListRecentEvents returns the newest events across every run.
func (s *Store) ListRecentEvents(limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.DB.Query(`SELECT id, run_id, task_id, kind, tool, summary, detail, created_at
		FROM events ORDER BY created_at DESC LIMIT ?`, limit)
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

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.' || r == '/':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// DeleteProject permanently removes a project and every run-scoped artifact
// under it. Assets themselves are shared and are kept; only their project links
// are removed. The default migration project is protected.
func (s *Store) DeleteProject(id string) error {
	project, err := s.GetProject(id)
	if err != nil {
		return err
	}
	if project.ID == defaultProjectID {
		return fmt.Errorf("默认项目不能删除")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		`DELETE FROM events WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM audit WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM messages WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM summaries WHERE session_id IN (SELECT id FROM sessions WHERE run_id IN (SELECT id FROM runs WHERE project_id=?))`,
		`DELETE FROM sessions WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM edges WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM anchors WHERE node_id IN (SELECT id FROM nodes WHERE run_id IN (SELECT id FROM runs WHERE project_id=?))`,
		`DELETE FROM retests WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM flows WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM evidence WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM tasks WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM approvals WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM goals WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM constraints WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM nodes WHERE run_id IN (SELECT id FROM runs WHERE project_id=?)`,
		`DELETE FROM runs WHERE project_id=?`,
		`DELETE FROM project_edges WHERE project_id=?`,
		`DELETE FROM project_nodes WHERE project_id=?`,
		`DELETE FROM project_assets WHERE project_id=?`,
		`DELETE FROM project_memory WHERE project_id=?`,
		`DELETE FROM projects WHERE id=?`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement, id); err != nil {
			return fmt.Errorf("delete project %s: %w", id, err)
		}
	}
	return tx.Commit()
}

func idSuffix(id string) string {
	if len(id) <= 4 {
		return id
	}
	return id[len(id)-4:]
}
