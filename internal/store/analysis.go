package store

import (
	"encoding/json"

	"github.com/guaidao2/lingeyebot/internal/model"
)

// ParseFact decodes a fact node payload.
func ParseFact(n Node) (model.Fact, error) { return decode[model.Fact](n.Payload) }

// ParseIntent decodes an intent node payload.
func ParseIntent(n Node) (model.Intent, error) { return decode[model.Intent](n.Payload) }

// ParseFinding decodes a finding node payload.
func ParseFinding(n Node) (model.Finding, error) { return decode[model.Finding](n.Payload) }

// ParseHint decodes a hint node payload.
func ParseHint(n Node) (model.Hint, error) { return decode[model.Hint](n.Payload) }

// ParseAttackPath decodes an attack-path node payload.
func ParseAttackPath(n Node) (model.AttackPath, error) {
	return decode[model.AttackPath](n.Payload)
}

// Snapshot returns the complete engagement view.
func (s *Store) Snapshot(runID string) (Snapshot, error) {
	run, err := s.GetRun(runID)
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Run: &run}
	if snap.Goals, err = s.ListGoals(runID); err != nil {
		return snap, err
	}
	if snap.Constraints, err = s.ListConstraints(runID); err != nil {
		return snap, err
	}
	if snap.Nodes, err = s.ListNodes(runID, ""); err != nil {
		return snap, err
	}
	if snap.Edges, err = s.ListEdges(runID); err != nil {
		return snap, err
	}
	if snap.Assets, err = s.ListRunAssets(runID); err != nil {
		return snap, err
	}
	if snap.Anchors, err = s.ListAnchors(runID); err != nil {
		return snap, err
	}
	if snap.Evidence, err = s.ListEvidence(runID); err != nil {
		return snap, err
	}
	if snap.Flows, err = s.ListFlows(runID); err != nil {
		return snap, err
	}
	if snap.Retests, err = s.ListRetests(runID); err != nil {
		return snap, err
	}
	if snap.Events, err = s.ListEvents(runID, 100); err != nil {
		return snap, err
	}
	if snap.Tasks, err = s.ListTasks(runID); err != nil {
		return snap, err
	}
	return snap, nil
}

// Coverage computes per-asset tested state. A fact counts only when it carries
// evidence; a finding anchors its assets directly.
func (s *Store) Coverage(runID string) (Coverage, error) {
	snap, err := s.Snapshot(runID)
	if err != nil {
		return Coverage{}, err
	}
	tested := map[string]bool{}
	for _, n := range snap.Nodes {
		switch n.Kind {
		case model.KindFact:
			f, err := ParseFact(n)
			if err != nil || len(f.EvidenceIDs) == 0 {
				continue
			}
			for _, id := range f.AssetIDs {
				tested[id] = true
			}
			for _, a := range snap.Anchors {
				if a.NodeID == n.ID {
					tested[a.AssetID] = true
				}
			}
		case model.KindFinding:
			f, err := ParseFinding(n)
			if err != nil {
				continue
			}
			for _, id := range f.AssetIDs {
				tested[id] = true
			}
			for _, a := range snap.Anchors {
				if a.NodeID == n.ID {
					tested[a.AssetID] = true
				}
			}
		}
	}
	out := Coverage{Assets: []AssetCoverage{}}
	for _, a := range snap.Assets {
		ok := tested[a.ID]
		if ok {
			out.Tested++
		} else {
			out.Untested = append(out.Untested, a)
		}
		out.Assets = append(out.Assets, AssetCoverage{Asset: a, Tested: ok})
	}
	out.Total = len(snap.Assets)
	return out, nil
}

// Review builds the pre-report checklist.
func (s *Store) Review(runID string) (Review, error) {
	snap, err := s.Snapshot(runID)
	if err != nil {
		return Review{}, err
	}
	cov, err := s.Coverage(runID)
	if err != nil {
		return Review{}, err
	}
	out := Review{Items: []ReviewItem{}}
	for _, g := range snap.Goals {
		status := "incomplete"
		if g.Status == "met" {
			status = "satisfied"
		} else if g.Status == "abandoned" {
			status = "deferred"
		}
		out.Items = append(out.Items, ReviewItem{Kind: "goal", Ref: g.ID, Status: status, Text: g.Text})
	}
	for _, n := range snap.Nodes {
		if n.Kind != model.KindIntent || n.State != "blocked" {
			continue
		}
		in, _ := ParseIntent(n)
		out.Items = append(out.Items, ReviewItem{Kind: "intent", Ref: n.ID, Status: "blocked", Text: in.Title})
	}
	for _, a := range cov.Untested {
		out.Items = append(out.Items, ReviewItem{Kind: "asset", Ref: a.ID, Status: "incomplete", Text: a.Type + " " + a.Value + " 未被测试"})
	}
	for _, n := range snap.Nodes {
		switch n.Kind {
		case model.KindFinding:
			f, err := ParseFinding(n)
			if err != nil {
				continue
			}
			if len(f.Steps) == 0 {
				out.Items = append(out.Items, ReviewItem{Kind: "finding", Ref: n.ID, Status: "incomplete", Text: "漏洞缺少可复现步骤"})
			}
			if f.TrustBoundary == "" && (containsFold(f.VulnClass, "jwt") || containsFold(f.VulnClass, "token") || containsFold(f.VulnClass, "oauth")) {
				out.Items = append(out.Items, ReviewItem{Kind: "finding", Ref: n.ID, Status: "incomplete", Text: "token/断言类漏洞缺少信任边界说明"})
			}
		case model.KindFact:
			f, err := ParseFact(n)
			if err != nil {
				continue
			}
			if f.Basis == "assumed" {
				out.Items = append(out.Items, ReviewItem{Kind: "fact", Ref: n.ID, Status: "incomplete", Text: "仍是假设：" + f.Detail})
			}
		}
	}
	return out, nil
}

func containsFold(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && jsonContainsFold(s, sub))
}

func jsonContainsFold(s, sub string) bool {
	ls, lsub := lower(s), lower(sub)
	for i := 0; i+len(lsub) <= len(ls); i++ {
		if ls[i:i+len(lsub)] == lsub {
			return true
		}
	}
	return false
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

var _ = json.Valid
