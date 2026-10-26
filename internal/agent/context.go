package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/llm"
	"github.com/guaidao2/lingeyebot/internal/model"
	"github.com/guaidao2/lingeyebot/internal/store"
)

// MessagesFromStore converts persisted messages into provider-valid messages.
//
// A window over the tail of a conversation can start with an orphan tool result,
// and an interrupted turn can leave an assistant tool_call without a matching
// result. Both make OpenAI/Anthropic reject the next request, so repair them:
// drop orphan tool rows and synthesize a result for every dangling call.
func MessagesFromStore(msgs []store.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	pending := map[string]bool{}
	order := []string{}
	recoveredID := 0

	flush := func() {
		for _, id := range order {
			if pending[id] {
				out = append(out, llm.Message{Role: "tool", ToolCallID: id, Content: "missing tool result (recovered)"})
			}
		}
		pending = map[string]bool{}
		order = nil
	}

	for _, m := range msgs {
		switch m.Role {
		case "assistant":
			flush()
			lm := llm.Message{Role: "assistant", Content: m.Content}
			if len(m.ToolCalls) > 0 && string(m.ToolCalls) != "null" {
				_ = json.Unmarshal(m.ToolCalls, &lm.ToolCalls)
			}
			if lm.Content == "" && len(lm.ToolCalls) == 0 {
				continue
			}
			for i := range lm.ToolCalls {
				if lm.ToolCalls[i].ID == "" {
					lm.ToolCalls[i].ID = fmt.Sprintf("recovered-%d", recoveredID)
					recoveredID++
				}
				id := lm.ToolCalls[i].ID
				pending[id] = true
				order = append(order, id)
			}
			out = append(out, lm)
		case "tool":
			if m.ToolCallID == "" || !pending[m.ToolCallID] {
				continue // orphan result: its assistant turn is outside the window
			}
			delete(pending, m.ToolCallID)
			out = append(out, llm.Message{
				Role: "tool", Content: m.Content, ToolCallID: m.ToolCallID, Name: m.Name,
			})
		case "user":
			flush()
			if m.Content == "" {
				continue
			}
			out = append(out, llm.Message{Role: "user", Content: m.Content})
		}
	}
	flush()
	return out
}

// Overview renders a compact, model-facing situation summary.
func Overview(run store.Run, snap store.Snapshot, cov store.Coverage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【任务】模式=%s 目标=%s 目的=%s\n", run.Mode, run.Target, run.Objective)
	if run.Authorization != "" {
		fmt.Fprintf(&b, "【授权】%s\n", run.Authorization)
	}
	if run.Scope != "" {
		fmt.Fprintf(&b, "【范围】%s\n", run.Scope)
	}
	if len(snap.Constraints) > 0 {
		b.WriteString("【约束】\n")
		for _, c := range snap.Constraints {
			fmt.Fprintf(&b, "- [%s] %s\n", c.Kind, c.Text)
		}
	}
	b.WriteString("【目标状态】\n")
	if len(snap.Goals) == 0 {
		b.WriteString("- （无目标）\n")
	}
	for _, g := range snap.Goals {
		fmt.Fprintf(&b, "- %s [%s]", g.ID, g.Status)
		if g.Acceptance != "" {
			fmt.Fprintf(&b, "（验收：%s）", g.Acceptance)
		}
		fmt.Fprintf(&b, " %s\n", g.Text)
	}

	intents, facts, findings := 0, 0, 0
	var openIntents []store.Node
	var recentFacts []store.Node
	var assumed, negative []string
	for _, n := range snap.Nodes {
		switch n.Kind {
		case model.KindIntent:
			intents++
			if n.State == "open" || n.State == "running" {
				openIntents = append(openIntents, n)
			}
		case model.KindFact:
			facts++
			f, err := store.ParseFact(n)
			if err == nil {
				recentFacts = append(recentFacts, n)
				if f.Basis == "assumed" {
					assumed = append(assumed, f.Detail)
				}
				if f.Basis == "negative" {
					negative = append(negative, f.Detail)
				}
			}
		case model.KindFinding:
			findings++
		}
	}
	fmt.Fprintf(&b, "【态势】资产 %d（已测 %d，未测 %d），意图 %d（进行中 %d），事实 %d，漏洞 %d，证据 %d\n",
		cov.Total, cov.Tested, len(cov.Untested), intents, len(openIntents), facts, findings, len(snap.Evidence))

	if len(openIntents) > 0 {
		b.WriteString("【前沿意图】\n")
		for i, n := range openIntents {
			if i >= 8 {
				break
			}
			in, _ := store.ParseIntent(n)
			fmt.Fprintf(&b, "- %s [%s] %s", n.ID, n.State, in.Title)
			if in.ExpectedObservation != "" {
				fmt.Fprintf(&b, "（预期观测：%s）", in.ExpectedObservation)
			}
			b.WriteString("\n")
		}
	}
	if len(recentFacts) > 0 {
		b.WriteString("【最近事实】\n")
		start := 0
		if len(recentFacts) > 8 {
			start = len(recentFacts) - 8
		}
		for _, n := range recentFacts[start:] {
			f, _ := store.ParseFact(n)
			fmt.Fprintf(&b, "- %s [%s/%.2f] %s\n", n.ID, f.Basis, f.Confidence, Short(f.Detail, 200))
		}
	}
	if len(assumed)+len(negative) > 0 {
		b.WriteString("【信念缺口】\n")
		for _, s := range assumed {
			fmt.Fprintf(&b, "- 假设：%s\n", Short(s, 160))
		}
		for _, s := range negative {
			fmt.Fprintf(&b, "- 否定：%s\n", Short(s, 160))
		}
	}
	if len(cov.Untested) > 0 {
		b.WriteString("【未测资产】\n")
		for i, a := range cov.Untested {
			if i >= 8 {
				break
			}
			fmt.Fprintf(&b, "- %s %s\n", a.Type, a.Value)
		}
	}
	return b.String()
}

// GoalBlock renders goals for prompts that need a shorter view.
func GoalBlock(snap store.Snapshot) string {
	var b strings.Builder
	for _, g := range snap.Goals {
		fmt.Fprintf(&b, "- [%s] %s\n", g.Status, g.Text)
	}
	return b.String()
}
