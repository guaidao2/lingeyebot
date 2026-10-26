PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS projects (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    nkey          TEXT NOT NULL UNIQUE,
    description   TEXT NOT NULL DEFAULT '',
    authorization TEXT NOT NULL DEFAULT '',
    scope         TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_projects_created ON projects(created_at DESC);

CREATE TABLE IF NOT EXISTS runs (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL DEFAULT '',
    title         TEXT NOT NULL DEFAULT '',
    mode          TEXT NOT NULL,
    target        TEXT NOT NULL DEFAULT '',
    objective     TEXT NOT NULL DEFAULT '',
    authorization TEXT NOT NULL DEFAULT '',
    scope         TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active',
    provider_id   TEXT NOT NULL DEFAULT '',
    model         TEXT NOT NULL DEFAULT '',
    goal_mode     INTEGER NOT NULL DEFAULT 1,
    auto_turns    INTEGER NOT NULL DEFAULT 0,
    last_user_at  TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runs_created ON runs(created_at DESC);

CREATE TABLE IF NOT EXISTS goals (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    text        TEXT NOT NULL,
    acceptance  TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'open',
    evidence    TEXT NOT NULL DEFAULT '[]',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_goals_run ON goals(run_id);

CREATE TABLE IF NOT EXISTS constraints (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL DEFAULT 'deny',
    text       TEXT NOT NULL,
    origin     TEXT NOT NULL DEFAULT 'user',
    active     INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_constraints_run ON constraints(run_id);

CREATE TABLE IF NOT EXISTS nodes (
    id           TEXT PRIMARY KEY,
    run_id       TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    payload      TEXT NOT NULL DEFAULT '{}',
    state        TEXT NOT NULL DEFAULT 'open',
    priority     INTEGER NOT NULL DEFAULT 0,
    origin       TEXT NOT NULL DEFAULT '',
    owner        TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    completed_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_nodes_run_kind ON nodes(run_id, kind, created_at);
CREATE INDEX IF NOT EXISTS idx_nodes_run_state ON nodes(run_id, state);

CREATE TABLE IF NOT EXISTS edges (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    src_id     TEXT NOT NULL,
    dst_id     TEXT NOT NULL,
    rel        TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_edges_run ON edges(run_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_edges ON edges(run_id, src_id, dst_id, rel);

CREATE TABLE IF NOT EXISTS assets (
    id             TEXT PRIMARY KEY,
    type           TEXT NOT NULL,
    value          TEXT NOT NULL,
    identifier     TEXT NOT NULL,
    parent_id      TEXT,
    company        TEXT NOT NULL DEFAULT '',
    meta           TEXT NOT NULL DEFAULT '',
    first_seen_run TEXT NOT NULL DEFAULT '',
    last_seen      TEXT NOT NULL,
    created_at     TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_assets_type_identifier ON assets(type, identifier);
CREATE INDEX IF NOT EXISTS idx_assets_value ON assets(value);

CREATE TABLE IF NOT EXISTS anchors (
    node_id  TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    PRIMARY KEY (node_id, asset_id)
);
CREATE INDEX IF NOT EXISTS idx_anchors_asset ON anchors(asset_id);

CREATE TABLE IF NOT EXISTS evidence (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL DEFAULT 'text',
    label      TEXT NOT NULL,
    sha256     TEXT NOT NULL DEFAULT '',
    size       INTEGER NOT NULL DEFAULT 0,
    channel    TEXT NOT NULL DEFAULT '',
    layers     TEXT NOT NULL DEFAULT '[]',
    ref_path   TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_evidence_run ON evidence(run_id, created_at);

CREATE TABLE IF NOT EXISTS flows (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    source      TEXT NOT NULL,
    transform   TEXT NOT NULL DEFAULT '',
    sink        TEXT NOT NULL,
    confidence  REAL NOT NULL DEFAULT 0.6,
    evidence    TEXT NOT NULL DEFAULT '[]',
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_flows_run ON flows(run_id);

CREATE TABLE IF NOT EXISTS retests (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    finding_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'pending',
    verdict    TEXT NOT NULL DEFAULT '',
    notes      TEXT NOT NULL DEFAULT '',
    evidence   TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    finished_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_retests_run ON retests(run_id);

CREATE TABLE IF NOT EXISTS events (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL,
    task_id    TEXT NOT NULL DEFAULT '',
    kind       TEXT NOT NULL,
    tool       TEXT NOT NULL DEFAULT '',
    summary    TEXT NOT NULL DEFAULT '',
    detail     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_run ON events(run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_events_task ON events(run_id, task_id, created_at);

CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    parent_id   TEXT NOT NULL DEFAULT '',
    node_id     TEXT NOT NULL DEFAULT '',
    role        TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'queued',
    budget      INTEGER NOT NULL DEFAULT 0,
    inbox       TEXT NOT NULL DEFAULT '[]',
    result      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    started_at  TEXT,
    finished_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_tasks_run ON tasks(run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    generation INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_sessions_key ON sessions(key);

CREATE TABLE IF NOT EXISTS messages (
    id           TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    run_id       TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    role         TEXT NOT NULL,
    content      TEXT NOT NULL DEFAULT '',
    reasoning    TEXT NOT NULL DEFAULT '',
    tool_calls   TEXT NOT NULL DEFAULT '[]',
    tool_call_id TEXT NOT NULL DEFAULT '',
    name         TEXT NOT NULL DEFAULT '',
    visible      INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at);

CREATE TABLE IF NOT EXISTS summaries (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    cursor     INTEGER NOT NULL,
    content    TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS memory (
    id         TEXT PRIMARY KEY,
    scope      TEXT NOT NULL DEFAULT 'global',
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_memory_scope_key ON memory(scope, key);

CREATE TABLE IF NOT EXISTS approvals (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    task_id    TEXT NOT NULL DEFAULT '',
    tool       TEXT NOT NULL,
    input      TEXT NOT NULL DEFAULT '{}',
    reason     TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL,
    decided_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status, created_at);

CREATE TABLE IF NOT EXISTS audit (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL DEFAULT '',
    task_id    TEXT NOT NULL DEFAULT '',
    tool       TEXT NOT NULL,
    action     TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    command    TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_run ON audit(run_id, created_at);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_nodes (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL DEFAULT '',
    kind            TEXT NOT NULL DEFAULT 'note',
    payload         TEXT NOT NULL DEFAULT '{}',
    state           TEXT NOT NULL DEFAULT 'active',
    priority        INTEGER NOT NULL DEFAULT 0,
    pinned          INTEGER NOT NULL DEFAULT 0,
    visibility      TEXT NOT NULL DEFAULT 'project',
    origin          TEXT NOT NULL DEFAULT '',
    owner           TEXT NOT NULL DEFAULT '',
    source_node_id  TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_project_nodes_project ON project_nodes(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_project_nodes_pinned ON project_nodes(project_id, pinned, updated_at DESC);

CREATE TABLE IF NOT EXISTS project_edges (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    src_id     TEXT NOT NULL REFERENCES project_nodes(id) ON DELETE CASCADE,
    dst_id     TEXT NOT NULL REFERENCES project_nodes(id) ON DELETE CASCADE,
    rel        TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_project_edges_project ON project_edges(project_id);

CREATE TABLE IF NOT EXISTS project_assets (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    asset_id   TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    source     TEXT NOT NULL DEFAULT 'agent',
    created_at TEXT NOT NULL,
    PRIMARY KEY (project_id, asset_id)
);
CREATE INDEX IF NOT EXISTS idx_project_assets_asset ON project_assets(asset_id);

CREATE TABLE IF NOT EXISTS project_memory (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key            TEXT NOT NULL,
    value          TEXT NOT NULL,
    kind           TEXT NOT NULL DEFAULT 'fact',
    source_node_id TEXT NOT NULL DEFAULT '',
    updated_by     TEXT NOT NULL DEFAULT '',
    updated_at     TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_project_memory_key ON project_memory(project_id, key);
CREATE INDEX IF NOT EXISTS idx_project_memory_project ON project_memory(project_id, updated_at DESC);
