// Package model defines the payload shapes stored in exploration nodes.
package model

// Node kinds.
const (
	KindGoal       = "goal"
	KindIntent     = "intent"
	KindFact       = "fact"
	KindFinding    = "finding"
	KindHint       = "hint"
	KindAttackPath = "attack_path"
	KindDigest     = "digest"
)

// Fact is a belief update, not a truth claim.
type Fact struct {
	Kind        string   `json:"kind,omitempty"` // port|service|vuln|http|source|credential|info
	Target      string   `json:"target,omitempty"`
	Detail      string   `json:"detail"`
	Basis       string   `json:"basis"` // observed|inferred|assumed|negative
	Confidence  float64  `json:"confidence"`
	Provenance  string   `json:"provenance,omitempty"`
	EvidenceIDs []string `json:"evidenceIds,omitempty"`
	AssetIDs    []string `json:"assetIds,omitempty"`
}

// Intent is an experiment: what to try and what observation would settle it.
type Intent struct {
	Title               string   `json:"title"`
	Detail              string   `json:"detail,omitempty"`
	ExpectedObservation string   `json:"expectedObservation,omitempty"`
	GoalID              string   `json:"goalId,omitempty"`
	DerivedFromFactIDs  []string `json:"derivedFromFactIds,omitempty"`
	AssetIDs            []string `json:"assetIds,omitempty"`
}

// Finding is a confirmed vulnerability with reproducible evidence.
type Finding struct {
	VulnClass      string   `json:"vulnclass,omitempty"`
	Title          string   `json:"title"`
	Severity       string   `json:"severity"` // critical|high|medium|low|info
	Summary        string   `json:"summary,omitempty"`
	Description    string   `json:"description,omitempty"`
	Impact         string   `json:"impact,omitempty"`
	TrustBoundary  string   `json:"trustBoundary,omitempty"`
	Steps          []string `json:"reproducibleSteps"`
	EvidenceIDs    []string `json:"evidenceIds,omitempty"`
	AssetIDs       []string `json:"assetIds,omitempty"`
	ReportMarkdown string   `json:"reportMarkdown,omitempty"`
}

// Hint is human or agent guidance consumed by later planning rounds.
type Hint struct {
	Text      string   `json:"text"`
	Source    string   `json:"source,omitempty"` // human|agent
	ParentIDs []string `json:"parentIds,omitempty"`
}

// AttackPath is a candidate or validated causal chain. It is not the exploration graph.
type AttackPath struct {
	Title   string     `json:"title"`
	Summary string     `json:"summary,omitempty"`
	Status  string     `json:"status"` // suspected|validated|blocked|closed
	Steps   []PathStep `json:"steps,omitempty"`
}

// PathStep is one hop in an attack path.
type PathStep struct {
	Seq    int    `json:"seq"`
	EdgeID string `json:"edgeId,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Goal is a deliverable that can be proven with evidence.
type Goal struct {
	Text        string   `json:"text"`
	Acceptance  string   `json:"acceptance,omitempty"`
	EvidenceIDs []string `json:"evidenceIds,omitempty"`
}

// Flow records a source -> transform -> sink concat point.
type Flow struct {
	Source      string   `json:"source"`
	Transform   string   `json:"transform,omitempty"`
	Sink        string   `json:"sink"`
	Confidence  float64  `json:"confidence"`
	EvidenceIDs []string `json:"evidenceIds,omitempty"`
}

// Constraint bounds an engagement.
type Constraint struct {
	Kind string `json:"kind"` // allow|deny
	Text string `json:"text"`
}
