package store

import (
	"fmt"
	"sort"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/model"
)

// Report renders the engagement as Markdown, including belief gaps.
func (s *Store) Report(runID string) (string, error) {
	snap, err := s.Snapshot(runID)
	if err != nil {
		return "", err
	}
	cov, err := s.Coverage(runID)
	if err != nil {
		return "", err
	}
	run := snap.Run
	var b strings.Builder
	b.WriteString("# lingeyebot 安全测试报告\n\n")
	if run == nil {
		return b.String() + "（尚未建立任务）\n", nil
	}
	auth := run.Authorization
	if auth == "" {
		auth = "（未声明）"
	}
	scope := run.Scope
	if scope == "" {
		scope = "（未声明）"
	}
	fmt.Fprintf(&b, "- 模式：%s\n- 目标：%s\n- 目的：%s\n- 授权：%s\n- 范围：%s\n- 状态：%s\n- 创建时间：%s\n\n",
		run.Mode, run.Target, run.Objective, auth, scope, run.Status, run.CreatedAt)

	evidenceByID := map[string]Evidence{}
	for _, e := range snap.Evidence {
		evidenceByID[e.ID] = e
	}
	var facts, intents, findings []Node
	var hints []Node
	for _, n := range snap.Nodes {
		switch n.Kind {
		case model.KindFact:
			facts = append(facts, n)
		case model.KindIntent:
			intents = append(intents, n)
		case model.KindFinding:
			findings = append(findings, n)
		case model.KindHint:
			hints = append(hints, n)
		}
	}

	fmt.Fprintf(&b, "## 摘要\n\n")
	fmt.Fprintf(&b, "- 目标 %d，意图 %d，事实 %d，漏洞 %d\n", len(snap.Goals), len(intents), len(facts), len(findings))
	fmt.Fprintf(&b, "- 资产 %d（已测 %d，未测 %d），证据 %d，数据流 %d\n", cov.Total, cov.Tested, len(cov.Untested), len(snap.Evidence), len(snap.Flows))
	fmt.Fprintf(&b, "- 复测 %d\n\n", len(snap.Retests))

	b.WriteString("## 目标与判定\n\n")
	if len(snap.Goals) == 0 {
		b.WriteString("（无）\n\n")
	}
	for _, g := range snap.Goals {
		fmt.Fprintf(&b, "- [%s] %s", g.Status, g.Text)
		if g.Acceptance != "" {
			fmt.Fprintf(&b, "（验收：%s）", g.Acceptance)
		}
		if len(g.Evidence) > 0 {
			fmt.Fprintf(&b, " 证据：%s", strings.Join(g.Evidence, ", "))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## 探索链路\n\n")
	if len(intents) == 0 {
		b.WriteString("（无）\n\n")
	}
	for _, n := range intents {
		in, err := ParseIntent(n)
		if err != nil {
			continue
		}
		parent := in.GoalID
		if len(in.DerivedFromFactIDs) > 0 {
			parent = strings.Join(in.DerivedFromFactIDs, ", ")
		}
		if parent == "" {
			parent = "无锚点"
		}
		fmt.Fprintf(&b, "- [%s] %s「%s」← %s", n.State, n.ID, in.Title, parent)
		if in.ExpectedObservation != "" {
			fmt.Fprintf(&b, "；预期观测：%s", in.ExpectedObservation)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## 事实\n\n")
	if len(facts) == 0 {
		b.WriteString("（无）\n\n")
	}
	for _, n := range facts {
		f, err := ParseFact(n)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "- [%s/%.2f] %s", f.Basis, f.Confidence, f.Detail)
		if f.Target != "" {
			fmt.Fprintf(&b, "（目标 %s）", f.Target)
		}
		if len(f.EvidenceIDs) > 0 {
			fmt.Fprintf(&b, " 证据：%s", strings.Join(f.EvidenceIDs, ", "))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## 漏洞\n\n")
	if len(findings) == 0 {
		b.WriteString("（无）\n\n")
	}
	for _, n := range findings {
		f, err := ParseFinding(n)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "### %s [%s] %s\n\n", n.ID, f.Severity, f.Title)
		if f.VulnClass != "" {
			fmt.Fprintf(&b, "- 分类：%s\n", f.VulnClass)
		}
		if f.Summary != "" {
			fmt.Fprintf(&b, "- 摘要：%s\n", f.Summary)
		}
		if f.Description != "" {
			fmt.Fprintf(&b, "- 描述：%s\n", f.Description)
		}
		if f.Impact != "" {
			fmt.Fprintf(&b, "- 影响：%s\n", f.Impact)
		}
		if f.TrustBoundary != "" {
			fmt.Fprintf(&b, "- 信任边界：%s\n", f.TrustBoundary)
		}
		b.WriteString("- 可复现步骤：\n")
		for i, step := range f.Steps {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, step)
		}
		if len(f.EvidenceIDs) > 0 {
			var refs []string
			for _, id := range f.EvidenceIDs {
				if e, ok := evidenceByID[id]; ok {
					refs = append(refs, fmt.Sprintf("%s(%s:%s)", id, e.Kind, e.Label))
				} else {
					refs = append(refs, id)
				}
			}
			fmt.Fprintf(&b, "- 证据：%s\n", strings.Join(refs, "；"))
		}
		if f.ReportMarkdown != "" {
			b.WriteString("\n" + f.ReportMarkdown + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## 资产\n\n")
	assetsByID := map[string]Asset{}
	for _, a := range snap.Assets {
		assetsByID[a.ID] = a
	}
	tested := map[string]bool{}
	for _, ac := range cov.Assets {
		tested[ac.ID] = ac.Tested
	}
	if len(snap.Assets) == 0 {
		b.WriteString("（无）\n\n")
	}
	for _, a := range snap.Assets {
		state := "未测"
		if tested[a.ID] {
			state = "已测"
		}
		parent := ""
		if a.ParentID != "" {
			if p, ok := assetsByID[a.ParentID]; ok {
				parent = " ← " + p.Value
			}
		}
		fmt.Fprintf(&b, "- [%s] %s（%s，%s）%s\n", a.Type, a.Value, a.FirstSeenRun, state, parent)
	}

	b.WriteString("\n## 数据流与拼接点\n\n")
	if len(snap.Flows) == 0 {
		b.WriteString("（无）\n\n")
	}
	for _, f := range snap.Flows {
		transform := f.Transform
		if transform == "" {
			transform = "（无变换）"
		}
		fmt.Fprintf(&b, "- %s → %s → %s（置信度 %.2f）\n", f.Source, transform, f.Sink, f.Confidence)
	}

	b.WriteString("\n## 证据\n\n")
	if len(snap.Evidence) == 0 {
		b.WriteString("（无）\n\n")
	}
	for _, e := range snap.Evidence {
		fmt.Fprintf(&b, "- %s [%s] %s", e.ID, e.Kind, e.Label)
		if e.Channel != "" {
			fmt.Fprintf(&b, " 信道=%s", e.Channel)
		}
		if len(e.Layers) > 0 {
			fmt.Fprintf(&b, " 层=%s", strings.Join(e.Layers, " / "))
		}
		if e.Size > 0 {
			fmt.Fprintf(&b, " 正文=%d 字节 sha256=%s", e.Size, e.SHA256[:12])
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## 复测\n\n")
	if len(snap.Retests) == 0 {
		b.WriteString("（无）\n\n")
	}
	for _, r := range snap.Retests {
		fmt.Fprintf(&b, "- %s 漏洞 %s：%s", r.ID, r.FindingID, r.Status)
		if r.Verdict != "" {
			fmt.Fprintf(&b, " / %s", r.Verdict)
		}
		if r.Notes != "" {
			fmt.Fprintf(&b, "；%s", r.Notes)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## 信念缺口\n\n")
	missing := false
	for _, n := range facts {
		f, err := ParseFact(n)
		if err != nil {
			continue
		}
		if f.Basis == "assumed" || f.Basis == "negative" {
			missing = true
			fmt.Fprintf(&b, "- [%s] %s\n", f.Basis, f.Detail)
		}
	}
	for _, a := range cov.Untested {
		missing = true
		fmt.Fprintf(&b, "- [未测资产] %s %s\n", a.Type, a.Value)
	}
	for _, g := range snap.Goals {
		if g.Status == "open" {
			missing = true
			fmt.Fprintf(&b, "- [未完成目标] %s\n", g.Text)
		}
	}
	if !missing {
		b.WriteString("（当前无已知缺口）\n")
	}
	b.WriteString("\n## 活跃提示\n\n")
	if len(hints) == 0 {
		b.WriteString("（无）\n")
	}
	for _, n := range hints {
		h, err := ParseHint(n)
		if err != nil || n.State != "active" {
			continue
		}
		fmt.Fprintf(&b, "- %s\n", h.Text)
	}
	return b.String(), nil
}

// SortedAssets returns assets in a stable order for display and export.
func SortedAssets(assets []Asset) []Asset {
	out := append([]Asset(nil), assets...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Value < out[j].Value
	})
	return out
}
