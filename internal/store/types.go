package store

import (
	"encoding/json"
	"time"
)

// Run is one conversation: a mode bound to a target, objective and scope.
type Run struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	Title         string `json:"title"`
	Mode          string `json:"mode"`
	Target        string `json:"target"`
	Objective     string `json:"objective"`
	Authorization string `json:"authorization"`
	Scope         string `json:"scope"`
	Status        string `json:"status"`
	ProviderID    string `json:"providerId"`
	Model         string `json:"model"`
	// GoalMode nil means a legacy run with goal mode enabled. New tasks
	// created via the API set this explicitly; the UI defaults it to off.
	GoalMode   *bool  `json:"goalMode,omitempty"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	AutoTurns  int    `json:"autoTurns"`
	LastUserAt string `json:"lastUserAt"`
}

// GoalsEnabled reports whether this run is goal-driven.
func (r Run) GoalsEnabled() bool {
	return r.GoalMode == nil || *r.GoalMode
}

// Goal is an independently provable deliverable.
type Goal struct {
	ID         string   `json:"id"`
	RunID      string   `json:"runId"`
	Text       string   `json:"text"`
	Acceptance string   `json:"acceptance"`
	Status     string   `json:"status"` // open|met|abandoned
	Evidence   []string `json:"evidenceIds"`
	CreatedAt  string   `json:"createdAt"`
	UpdatedAt  string   `json:"updatedAt"`
}

// Constraint bounds an engagement.
type Constraint struct {
	ID        string `json:"id"`
	RunID     string `json:"runId"`
	Kind      string `json:"kind"` // allow|deny
	Text      string `json:"text"`
	Origin    string `json:"origin"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"createdAt"`
}

// Node is one exploration-graph vertex.
type Node struct {
	ID          string          `json:"id"`
	RunID       string          `json:"runId"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload"`
	State       string          `json:"state"`
	Priority    int             `json:"priority"`
	Origin      string          `json:"origin"`
	Owner       string          `json:"owner"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt"`
	CompletedAt string          `json:"completedAt,omitempty"`
}

// Edge links exploration nodes.
type Edge struct {
	ID        string `json:"id"`
	RunID     string `json:"runId"`
	SrcID     string `json:"srcId"`
	DstID     string `json:"dstId"`
	Rel       string `json:"rel"`
	CreatedAt string `json:"createdAt"`
}

// Asset is a globally shared node in the asset graph.
type Asset struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Value        string `json:"value"`
	Identifier   string `json:"identifier"`
	ParentID     string `json:"parentId,omitempty"`
	Company      string `json:"company"`
	Meta         string `json:"meta"`
	FirstSeenRun string `json:"firstSeenRun"`
	LastSeen     string `json:"lastSeen"`
	CreatedAt    string `json:"createdAt"`
}

// Evidence is an immutable observation body addressed by sha256.
type Evidence struct {
	ID        string   `json:"id"`
	RunID     string   `json:"runId"`
	Kind      string   `json:"kind"`
	Label     string   `json:"label"`
	SHA256    string   `json:"sha256"`
	Size      int64    `json:"size"`
	Channel   string   `json:"channel"`
	Layers    []string `json:"layers"`
	RefPath   string   `json:"refPath"`
	CreatedAt string   `json:"createdAt"`
}

// Flow is a source -> transform -> sink concat point.
type Flow struct {
	ID         string   `json:"id"`
	RunID      string   `json:"runId"`
	Source     string   `json:"source"`
	Transform  string   `json:"transform"`
	Sink       string   `json:"sink"`
	Confidence float64  `json:"confidence"`
	Evidence   []string `json:"evidenceIds"`
	CreatedAt  string   `json:"createdAt"`
}

// Retest is an independent re-verification of a finding.
type Retest struct {
	ID         string   `json:"id"`
	RunID      string   `json:"runId"`
	FindingID  string   `json:"findingId"`
	Status     string   `json:"status"`
	Verdict    string   `json:"verdict"`
	Notes      string   `json:"notes"`
	Evidence   []string `json:"evidenceIds"`
	CreatedAt  string   `json:"createdAt"`
	FinishedAt string   `json:"finishedAt,omitempty"`
}

// Event is one execution or lifecycle record.
type Event struct {
	ID        string `json:"id"`
	RunID     string `json:"runId"`
	TaskID    string `json:"taskId"`
	Kind      string `json:"kind"`
	Tool      string `json:"tool"`
	Summary   string `json:"summary"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"createdAt"`
}

// Task is a subagent job.
type Task struct {
	ID         string         `json:"id"`
	RunID      string         `json:"runId"`
	ParentID   string         `json:"parentId"`
	NodeID     string         `json:"nodeId"`
	Role       string         `json:"role"`
	Title      string         `json:"title"`
	Status     string         `json:"status"`
	Budget     int            `json:"budget"`
	Inbox      []InboxMessage `json:"inbox"`
	Result     string         `json:"result"`
	Error      string         `json:"error"`
	CreatedAt  string         `json:"createdAt"`
	StartedAt  string         `json:"startedAt,omitempty"`
	FinishedAt string         `json:"finishedAt,omitempty"`
}

// InboxMessage is one mid-run instruction queued for a task.
type InboxMessage struct {
	Text string `json:"text"`
	At   string `json:"at"`
}

// Message is one persisted conversation turn.
type Message struct {
	ID         string          `json:"id"`
	SessionID  string          `json:"sessionId"`
	RunID      string          `json:"runId"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	Reasoning  string          `json:"reasoning,omitempty"`
	ToolCalls  json.RawMessage `json:"toolCalls,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"`
	Name       string          `json:"name,omitempty"`
	Visible    bool            `json:"visible"`
	CreatedAt  string          `json:"createdAt"`
}

// Approval is a pending tool-call confirmation.
type Approval struct {
	ID        string `json:"id"`
	RunID     string `json:"runId"`
	TaskID    string `json:"taskId"`
	Tool      string `json:"tool"`
	Input     string `json:"input"`
	Reason    string `json:"reason"`
	Status    string `json:"status"` // pending|approved|denied|timeout
	CreatedAt string `json:"createdAt"`
	DecidedAt string `json:"decidedAt,omitempty"`
}

// AuditEntry records one gated tool call.
type AuditEntry struct {
	ID        string `json:"id"`
	RunID     string `json:"runId"`
	TaskID    string `json:"taskId"`
	Tool      string `json:"tool"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	Command   string `json:"command"`
	CreatedAt string `json:"createdAt"`
}

// Session is a conversation bound to one run.
type Session struct {
	ID         string `json:"id"`
	RunID      string `json:"runId"`
	Key        string `json:"key"`
	Generation int    `json:"generation"`
	CreatedAt  string `json:"createdAt"`
}

// Snapshot is the whole engagement view used by tools, API and reports.
type Snapshot struct {
	Run         *Run         `json:"run"`
	Goals       []Goal       `json:"goals"`
	Constraints []Constraint `json:"constraints"`
	Nodes       []Node       `json:"nodes"`
	Edges       []Edge       `json:"edges"`
	Assets      []Asset      `json:"assets"`
	Anchors     []Anchor     `json:"anchors"`
	Evidence    []Evidence   `json:"evidence"`
	Flows       []Flow       `json:"flows"`
	Retests     []Retest     `json:"retests"`
	Events      []Event      `json:"events"`
	Tasks       []Task       `json:"tasks"`
}

// Anchor links an exploration node to an asset.
type Anchor struct {
	NodeID  string `json:"nodeId"`
	AssetID string `json:"assetId"`
}

// Coverage summarizes tested assets. A fact counts only when it carries evidence.
type Coverage struct {
	Total    int             `json:"total"`
	Tested   int             `json:"tested"`
	Untested []Asset         `json:"untested"`
	Assets   []AssetCoverage `json:"assets"`
}

// AssetCoverage is the per-asset tested flag.
type AssetCoverage struct {
	Asset
	Tested bool `json:"tested"`
}

// Review is the pre-report checklist.
type Review struct {
	Items []ReviewItem `json:"items"`
}

// ReviewItem is one open or incomplete item.
type ReviewItem struct {
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	Status string `json:"status"` // satisfied|incomplete|blocked|deferred
	Text   string `json:"text"`
}

// Project groups conversations and shares scope, assets, findings and a blackboard.
type Project struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	NKey          string `json:"nkey"`
	Description   string `json:"description"`
	Authorization string `json:"authorization"`
	Scope         string `json:"scope"`
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// ProjectSummary is a project row plus its live counts.
type ProjectSummary struct {
	Project
	Conversations int    `json:"conversations"`
	Findings      int    `json:"findings"`
	OpenGoals     int    `json:"openGoals"`
	Assets        int    `json:"assets"`
	LastActivity  string `json:"lastActivity"`
}

// ProjectNode is one shared blackboard entry.
type ProjectNode struct {
	ID             string          `json:"id"`
	ProjectID      string          `json:"projectId"`
	ConversationID string          `json:"conversationId"`
	Kind           string          `json:"kind"`
	Payload        json.RawMessage `json:"payload"`
	State          string          `json:"state"`
	Priority       int             `json:"priority"`
	Pinned         bool            `json:"pinned"`
	Visibility     string          `json:"visibility"`
	Origin         string          `json:"origin"`
	Owner          string          `json:"owner"`
	SourceNodeID   string          `json:"sourceNodeId"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
}

// ProjectEdge links shared blackboard entries.
type ProjectEdge struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	SrcID     string `json:"srcId"`
	DstID     string `json:"dstId"`
	Rel       string `json:"rel"`
	CreatedAt string `json:"createdAt"`
}

// ProjectMemory is compact shared knowledge injected into prompts.
type ProjectMemory struct {
	ID           string `json:"id"`
	ProjectID    string `json:"projectId"`
	Key          string `json:"key"`
	Value        string `json:"value"`
	Kind         string `json:"kind"`
	SourceNodeID string `json:"sourceNodeId"`
	UpdatedBy    string `json:"updatedBy"`
	UpdatedAt    string `json:"updatedAt"`
}

// ProjectFinding is a finding surfaced at project scope.
type ProjectFinding struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"projectId"`
	ProjectName       string          `json:"projectName"`
	ConversationID    string          `json:"conversationId"`
	ConversationTitle string          `json:"conversationTitle"`
	Payload           json.RawMessage `json:"payload"`
	State             string          `json:"state"`
	CreatedAt         string          `json:"createdAt"`
}

// ProjectDetail is the project workspace view.
type ProjectDetail struct {
	Project       Project          `json:"project"`
	Conversations []Run            `json:"conversations"`
	Assets        []Asset          `json:"assets"`
	Board         []ProjectNode    `json:"board"`
	Edges         []ProjectEdge    `json:"edges"`
	Memory        []ProjectMemory  `json:"memory"`
	Findings      []ProjectFinding `json:"findings"`
	Coverage      Coverage         `json:"coverage"`
}

// ToolCount is one entry in the tool-usage ranking.
type ToolCount struct {
	Tool  string `json:"tool"`
	Count int    `json:"count"`
}

// ProjectFact is a confirmed/inferred fact surfaced across projects.
type ProjectFact struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"projectId"`
	ProjectName       string          `json:"projectName"`
	ConversationID    string          `json:"conversationId"`
	ConversationTitle string          `json:"conversationTitle"`
	Payload           json.RawMessage `json:"payload"`
	State             string          `json:"state"`
	CreatedAt         string          `json:"createdAt"`
}

// Capabilities summarizes the tools available to the agent.
type Capabilities struct {
	MCPTools  int `json:"mcpTools"`
	MCPOnline int `json:"mcpOnline"`
	Skills    int `json:"skills"`
	Agents    int `json:"agents"`
	Modes     int `json:"modes"`
}

// Dashboard is the global overview.
type Dashboard struct {
	Projects         int              `json:"projects"`
	Conversations    int              `json:"conversations"`
	Running          int              `json:"running"`
	Findings         int              `json:"findings"`
	OpenGoals        int              `json:"openGoals"`
	Assets           int              `json:"assets"`
	ToolCalls        int              `json:"toolCalls"`
	PendingApprovals int              `json:"pendingApprovals"`
	RiskScore        int              `json:"riskScore"`
	Severity         map[string]int   `json:"severity"`
	StatusCounts     map[string]int   `json:"statusCounts"`
	ToolCounts       []ToolCount      `json:"toolCounts"`
	RecentFinding    []ProjectFinding `json:"recentFindings"`
	RecentFacts      []ProjectFact    `json:"recentFacts"`
	RecentEvents     []Event          `json:"recentEvents"`
	Capabilities     Capabilities     `json:"capabilities"`
}

func decode[T any](raw []byte) (T, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	err := json.Unmarshal(raw, &v)
	return v, err
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

const timestampLayout = "2006-01-02T15:04:05.000000000Z07:00"

func nowStr() string { return time.Now().UTC().Format(timestampLayout) }
