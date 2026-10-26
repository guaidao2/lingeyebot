package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/model"
	"github.com/guaidao2/lingeyebot/internal/store"
)

type evidenceInput struct {
	Kind    string   `json:"kind"`
	Label   string   `json:"label"`
	Content string   `json:"content"`
	RefPath string   `json:"refPath"`
	Channel string   `json:"channel"`
	Layers  []string `json:"layers"`
}

func addEvidence(env Env, in evidenceInput) (store.Evidence, error) {
	content := []byte(in.Content)
	if len(content) == 0 && in.RefPath != "" {
		data, err := os.ReadFile(env.resolve(in.RefPath))
		if err != nil {
			return store.Evidence{}, err
		}
		if len(data) > 8<<20 {
			return store.Evidence{}, errors.New("evidence file exceeds 8MB")
		}
		content = data
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = in.Kind + " evidence"
	}
	kind := in.Kind
	if kind == "" {
		kind = "text"
	}
	return env.Store.AddEvidence(store.Evidence{
		RunID: env.Run.ID, Kind: kind, Label: label, Channel: in.Channel, Layers: in.Layers,
	}, content)
}

func runTools(env Env) []Spec {
	return []Spec{
		runStatusTool(env),
		goalAddTool(env),
		goalProveTool(env),
		constraintAddTool(env),
		hintAddTool(env),
		stateOverviewTool(env),
		nextActionsTool(env),
		coverageTool(env),
		reviewTool(env),
		nodeDetailTool(env),
		listNodesTool(env),
		assetAddTool(env),
		retestCreateTool(env),
		retestFinishTool(env),
		attackPathTool(env),
	}
}

func runStatusTool(env Env) Spec {
	return Spec{
		Name: "run_status", ReadOnly: true,
		Description: "Read the current run, its goals, active constraints and record counts.",
		Parameters:  obj(map[string]any{}),
		Fn: func(_ context.Context, _ anyArgs) (string, error) {
			snap, err := env.Store.Snapshot(env.Run.ID)
			if err != nil {
				return "", err
			}
			return toJSON(map[string]any{
				"run":         snap.Run,
				"goals":       snap.Goals,
				"constraints": snap.Constraints,
				"counts":      counts(snap),
			})
		},
	}
}

func goalAddTool(env Env) Spec {
	return Spec{
		Name: "goal_add",
		Description: "Add one independently verifiable goal. Quantified acceptance criteria are required for " +
			"targets like 'get N flags' or 'cover X%': the evaluator will check measured values.",
		Parameters: obj(map[string]any{
			"text":       str("Goal statement."),
			"acceptance": str("How completion is measured and checked."),
		}, "text"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Text       string `json:"text"`
				Acceptance string `json:"acceptance"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			g, err := env.Store.AddGoal(store.Goal{RunID: env.Run.ID, Text: strings.TrimSpace(args.Text), Acceptance: args.Acceptance})
			if err != nil {
				return "", err
			}
			return toJSON(g)
		},
	}
}

func goalProveTool(env Env) Spec {
	return Spec{
		Name: "goal_prove",
		Description: "Mark a goal met or abandoned. Only mark met when the acceptance criteria are actually met " +
			"and evidence exists; when the last open goal is met the run stops automatically.",
		Parameters: obj(map[string]any{
			"goalId":      str("Goal id."),
			"evidenceIds": arr("Evidence supporting the verdict.", str("Evidence id.")),
			"reason":      str("Why the goal is met or abandoned."),
			"status":      strEnum("Verdict (default met).", "met", "abandoned"),
		}, "goalId"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				GoalID      string   `json:"goalId"`
				EvidenceIDs []string `json:"evidenceIds"`
				Reason      string   `json:"reason"`
				Status      string   `json:"status"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			goal, err := env.Store.GetGoal(args.GoalID)
			if err != nil {
				return "", err
			}
			if goal.RunID != env.Run.ID {
				return "", fmt.Errorf("goal belongs to another run")
			}
			if args.Status != "abandoned" && env.ValidateGoal != nil {
				if err := env.ValidateGoal(goal); err != nil {
					return "", err
				}
			}
			g, err := env.Store.ProveGoal(args.GoalID, args.EvidenceIDs, args.Status)
			if err != nil {
				return "", err
			}
			env.emit("goal", "goal_prove", "目标状态更新："+g.Text, args.Reason)
			return toJSON(g)
		},
	}
}

func constraintAddTool(env Env) Spec {
	return Spec{
		Name: "constraint_add",
		Description: "Record allow/deny constraints that bound this engagement. They are injected into every " +
			"prompt and checked by the policy gate.",
		Parameters: obj(map[string]any{
			"constraints": arr("Constraints.", obj(map[string]any{
				"kind": strEnum("Constraint kind (default deny).", "allow", "deny"),
				"text": str("Self-contained constraint with concrete targets."),
			}, "text")),
		}, "constraints"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Constraints []model.Constraint `json:"constraints"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			out := []store.Constraint{}
			for _, c := range args.Constraints {
				if strings.TrimSpace(c.Text) == "" {
					continue
				}
				created, err := env.Store.AddConstraint(store.Constraint{RunID: env.Run.ID, Kind: c.Kind, Text: c.Text, Origin: "agent"})
				if err != nil {
					return "", err
				}
				out = append(out, created)
			}
			return toJSON(out)
		},
	}
}

func hintAddTool(env Env) Spec {
	return Spec{
		Name:        "hint_add",
		Description: "Record strategic hints (human or agent judgment) that later planning rounds must consider.",
		Parameters: obj(map[string]any{
			"texts":  arr("Hint texts.", str("Hint.")),
			"source": strEnum("Hint source (default agent).", "human", "agent"),
		}, "texts"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Texts  []string `json:"texts"`
				Source string   `json:"source"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			source := args.Source
			if source != "human" {
				source = "agent"
			}
			out := []store.Node{}
			for _, text := range args.Texts {
				if strings.TrimSpace(text) == "" {
					continue
				}
				payload, _ := json.Marshal(model.Hint{Text: text, Source: source})
				node, err := env.Store.AddNode(store.Node{RunID: env.Run.ID, Kind: model.KindHint, Payload: payload, Owner: env.Agent})
				if err != nil {
					return "", err
				}
				out = append(out, node)
			}
			return toJSON(out)
		},
	}
}

func stateOverviewTool(env Env) Spec {
	return Spec{
		Name: "state_overview", ReadOnly: true,
		Description: "Read the full engagement state: goals, constraints, assets, intents, facts, findings, " +
			"evidence, tasks and belief gaps. Use it after subagent results or notable changes.",
		Parameters: obj(map[string]any{}),
		Fn: func(_ context.Context, _ anyArgs) (string, error) {
			snap, err := env.Store.Snapshot(env.Run.ID)
			if err != nil {
				return "", err
			}
			cov, err := env.Store.Coverage(env.Run.ID)
			if err != nil {
				return "", err
			}
			frontier, gaps := frontierOf(snap)
			var facts, findings []map[string]any
			for _, n := range snap.Nodes {
				switch n.Kind {
				case model.KindFact:
					if f, err := store.ParseFact(n); err == nil {
						facts = append(facts, map[string]any{"id": n.ID, "basis": f.Basis, "confidence": f.Confidence, "detail": f.Detail, "evidenceIds": f.EvidenceIDs})
					}
				case model.KindFinding:
					if f, err := store.ParseFinding(n); err == nil {
						findings = append(findings, map[string]any{"id": n.ID, "title": f.Title, "severity": f.Severity, "vulnclass": f.VulnClass})
					}
				}
			}
			return toJSON(map[string]any{
				"run": snap.Run, "goals": snap.Goals, "constraints": snap.Constraints,
				"counts": counts(snap), "coverage": cov,
				"frontier": frontier, "beliefGaps": gaps,
				"facts": facts, "findings": findings, "tasks": snap.Tasks,
			})
		},
	}
}

func nextActionsTool(env Env) Spec {
	return Spec{
		Name: "next_actions", ReadOnly: true,
		Description: "Read the ranked frontier and belief gaps. Pick the experiment most likely to change " +
			"the current judgment, not a checklist item.",
		Parameters: obj(map[string]any{}),
		Fn: func(_ context.Context, _ anyArgs) (string, error) {
			snap, err := env.Store.Snapshot(env.Run.ID)
			if err != nil {
				return "", err
			}
			frontier, gaps := frontierOf(snap)
			var hints []string
			for _, n := range snap.Nodes {
				if n.Kind == model.KindHint && n.State == "active" {
					if h, err := store.ParseHint(n); err == nil {
						hints = append(hints, h.Text)
					}
				}
			}
			return toJSON(map[string]any{"frontier": frontier, "beliefGaps": gaps, "hints": hints})
		},
	}
}

func coverageTool(env Env) Spec {
	return Spec{
		Name: "coverage", ReadOnly: true,
		Description: "Report asset coverage. A fact counts as tested only when it carries evidence; " +
			"a finding anchors its assets directly.",
		Parameters: obj(map[string]any{}),
		Fn: func(_ context.Context, _ anyArgs) (string, error) {
			cov, err := env.Store.Coverage(env.Run.ID)
			if err != nil {
				return "", err
			}
			return toJSON(cov)
		},
	}
}

func reviewTool(env Env) Spec {
	return Spec{
		Name: "review", ReadOnly: true,
		Description: "Pre-report checklist: open goals, blocked intents, untested assets, findings missing " +
			"reproducible steps or trust boundaries, and assumptions still unverified.",
		Parameters: obj(map[string]any{}),
		Fn: func(_ context.Context, _ anyArgs) (string, error) {
			r, err := env.Store.Review(env.Run.ID)
			if err != nil {
				return "", err
			}
			return toJSON(r)
		},
	}
}

func nodeDetailTool(env Env) Spec {
	return Spec{
		Name: "node_detail", ReadOnly: true,
		Description: "Read one graph node with its payload, edges, anchored assets and evidence bodies.",
		Parameters: obj(map[string]any{
			"id": str("Node id."),
		}, "id"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				ID string `json:"id"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			n, err := env.Store.GetNode(args.ID)
			if err != nil {
				return "", err
			}
			if n.RunID != env.Run.ID {
				return "", fmt.Errorf("node belongs to another run")
			}
			snap, err := env.Store.Snapshot(env.Run.ID)
			if err != nil {
				return "", err
			}
			var edges []store.Edge
			assetsByID := map[string]store.Asset{}
			for _, a := range snap.Assets {
				assetsByID[a.ID] = a
			}
			for _, e := range snap.Edges {
				if e.SrcID == n.ID || e.DstID == n.ID {
					edges = append(edges, e)
				}
			}
			var anchors []store.Asset
			for _, a := range snap.Anchors {
				if a.NodeID == n.ID {
					if asset, ok := assetsByID[a.AssetID]; ok {
						anchors = append(anchors, asset)
					}
				}
			}
			evidence := []map[string]any{}
			ids := evidenceIDsOf(n)
			for _, id := range ids {
				e, err := env.Store.GetEvidence(id)
				if err != nil {
					continue
				}
				body, _ := env.Store.ReadEvidence(id)
				text := string(body)
				if len(text) > 4000 {
					text = text[:4000] + "\n...(truncated)"
				}
				evidence = append(evidence, map[string]any{"id": e.ID, "kind": e.Kind, "label": e.Label, "channel": e.Channel, "layers": e.Layers, "content": text})
			}
			return toJSON(map[string]any{
				"node": n, "payload": json.RawMessage(n.Payload), "edges": edges,
				"anchoredAssets": anchors, "evidence": evidence,
				"incomingEdges": filterEdges(edges, "", n.ID), "outgoingEdges": filterEdges(edges, n.ID, ""),
			})
		},
	}
}

func listNodesTool(env Env) Spec {
	return Spec{
		Name: "list_nodes", ReadOnly: true,
		Description: "List graph nodes, optionally filtered by kind or state.",
		Parameters: obj(map[string]any{
			"kind":  strEnum("Node kind.", "goal", "intent", "fact", "finding", "hint", "attack_path", "digest"),
			"state": str("State filter."),
			"limit": integer("Maximum nodes (default 50)."),
		}),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Kind  string `json:"kind"`
				State string `json:"state"`
				Limit int    `json:"limit"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			nodes, err := env.Store.ListNodes(env.Run.ID, args.Kind)
			if err != nil {
				return "", err
			}
			limit := args.Limit
			if limit <= 0 {
				limit = 50
			}
			out := []store.Node{}
			for _, n := range nodes {
				if args.State != "" && n.State != args.State {
					continue
				}
				out = append(out, n)
				if len(out) >= limit {
					break
				}
			}
			return toJSON(out)
		},
	}
}

func assetAddTool(env Env) Spec {
	return Spec{
		Name: "asset_add",
		Description: "Record one asset in the global asset graph (normalized and de-duplicated by type + value). " +
			"Record parents before children.",
		Parameters: obj(map[string]any{
			"type":     strEnum("Asset type.", "root-domain", "subdomain", "ip", "service", "app", "endpoint", "credential", "tech"),
			"value":    str("Asset value, e.g. example.com, 10.0.0.5:443, https://x/api."),
			"parentId": str("Existing parent asset id."),
			"company":  str("Owning company or organization."),
			"meta":     str("Optional metadata (JSON or text)."),
		}, "type", "value"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Type     string `json:"type"`
				Value    string `json:"value"`
				ParentID string `json:"parentId"`
				Company  string `json:"company"`
				Meta     string `json:"meta"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			asset, created, err := env.Store.UpsertAsset(store.Asset{
				Type: args.Type, Value: args.Value, ParentID: args.ParentID, Company: args.Company,
				Meta: args.Meta, FirstSeenRun: env.Run.ID,
			})
			if err != nil {
				return "", err
			}
			if env.Run.ProjectID != "" {
				_ = env.Store.LinkProjectAsset(env.Run.ProjectID, asset.ID, env.Agent)
			}
			return toJSON(map[string]any{"asset": asset, "created": created})
		},
	}
}

func retestCreateTool(env Env) Spec {
	return Spec{
		Name:        "retest_create",
		Description: "Create an independent retest record for one finding, then delegate verification to a subagent.",
		Parameters: obj(map[string]any{
			"findingId": str("Finding node id."),
			"notes":     str("Optional retest conditions."),
		}, "findingId"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				FindingID string `json:"findingId"`
				Notes     string `json:"notes"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			n, err := env.Store.GetNode(args.FindingID)
			if err != nil {
				return "", err
			}
			if n.RunID != env.Run.ID {
				return "", fmt.Errorf("finding belongs to another run")
			}
			if n.Kind != model.KindFinding {
				return "", fmt.Errorf("%s is not a finding", args.FindingID)
			}
			r, err := env.Store.AddRetest(store.Retest{RunID: env.Run.ID, FindingID: args.FindingID, Notes: args.Notes})
			if err != nil {
				return "", err
			}
			return toJSON(r)
		},
	}
}

func retestFinishTool(env Env) Spec {
	return Spec{
		Name:        "retest_finish",
		Description: "Record a retest verdict: reproduced, fixed or inconclusive.",
		Parameters: obj(map[string]any{
			"retestId":    str("Retest id."),
			"verdict":     strEnum("Verdict.", "reproduced", "fixed", "inconclusive"),
			"notes":       str("Evidence-based notes."),
			"evidenceIds": arr("New evidence ids.", str("Evidence id.")),
		}, "retestId", "verdict"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				RetestID    string   `json:"retestId"`
				Verdict     string   `json:"verdict"`
				Notes       string   `json:"notes"`
				EvidenceIDs []string `json:"evidenceIds"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			retests, err := env.Store.ListRetests(env.Run.ID)
			if err != nil {
				return "", err
			}
			owned := false
			for _, retest := range retests {
				if retest.ID == args.RetestID {
					owned = true
					break
				}
			}
			if !owned {
				return "", fmt.Errorf("retest %s is not in this run", args.RetestID)
			}
			r, err := env.Store.FinishRetest(args.RetestID, args.Verdict, args.Notes, args.EvidenceIDs)
			if err != nil {
				return "", err
			}
			return toJSON(r)
		},
	}
}

func attackPathTool(env Env) Spec {
	return Spec{
		Name: "attack_path_add",
		Description: "Record a candidate or validated attack path. Attack paths are delivery artifacts and stay " +
			"separate from the exploration graph.",
		Parameters: obj(map[string]any{
			"title":   str("Path title."),
			"summary": str("One-paragraph summary."),
			"status":  strEnum("Status (default suspected).", "suspected", "validated", "blocked", "closed"),
			"steps": arr("Ordered steps.", obj(map[string]any{
				"seq":    integer("Step number."),
				"edgeId": str("Graph edge id if any."),
				"note":   str("What this step does."),
			})),
		}, "title"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args model.AttackPath
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			if args.Status == "" {
				args.Status = "suspected"
			}
			payload, _ := json.Marshal(args)
			n, err := env.Store.AddNode(store.Node{RunID: env.Run.ID, Kind: model.KindAttackPath, Payload: payload, State: args.Status, Owner: env.Agent})
			if err != nil {
				return "", err
			}
			return toJSON(n)
		},
	}
}

// graphTools are the write-side exploration tools.
func attachToParent(env Env, nodeID, intentID, intentRel, goalRel string) {
	if intentID != "" {
		intent, err := env.Store.GetNode(intentID)
		if err == nil && intent.RunID == env.Run.ID && intent.Kind == model.KindIntent {
			_, _ = env.Store.AddEdge(env.Run.ID, intentID, nodeID, intentRel)
			if intent.State == "open" {
				_ = env.Store.UpdateNodeState(intentID, "running", intent.Priority)
			}
			return
		}
		// A stale/cross-run intent id must not create an edge in this run;
		// fall through to the first open intent or the goal.
	}
	intents, err := env.Store.ListNodes(env.Run.ID, model.KindIntent)
	if err == nil {
		for _, intent := range intents {
			if intent.State == "open" || intent.State == "running" {
				_, _ = env.Store.AddEdge(env.Run.ID, intent.ID, nodeID, intentRel)
				if intent.State == "open" {
					_ = env.Store.UpdateNodeState(intent.ID, "running", intent.Priority)
				}
				return
			}
		}
	}
	goals, err := env.Store.ListGoals(env.Run.ID)
	if err != nil {
		return
	}
	for _, goal := range goals {
		if goal.Status == "open" {
			_, _ = env.Store.AddEdge(env.Run.ID, goal.ID, nodeID, goalRel)
			return
		}
	}
}

func graphTools(env Env) []Spec {
	return []Spec{
		intentAddTool(env),
		intentUpdateTool(env),
		factAddTool(env),
		findingAddTool(env),
		evidenceAddTool(env),
		flowAddTool(env),
	}
}

func intentAddTool(env Env) Spec {
	return Spec{
		Name: "intent_add",
		Description: "Add up to four exploration intents in one batch. Each intent is an experiment: it says what " +
			"to try and what observation would confirm or refute it. Anchor it to a goal or to facts.",
		Parameters: obj(map[string]any{
			"intents": arr("Intents (maximum four).", obj(map[string]any{
				"title":               str("Short intent title."),
				"detail":              str("Scope, methods and hypothesis."),
				"expectedObservation": str("Observation that would settle this direction."),
				"goalId":              str("Anchor goal id."),
				"derivedFromFactIds":  arr("Anchor fact ids.", str("Fact id.")),
				"assetIds":            arr("Assets this intent tests.", str("Asset id.")),
				"priority":            integer("Higher first."),
			}, "title")),
		}, "intents"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Intents []struct {
					model.Intent
					Priority int `json:"priority"`
				} `json:"intents"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			existing, err := env.Store.ListNodes(env.Run.ID, model.KindIntent)
			if err != nil {
				return "", err
			}
			seen := map[string]bool{}
			for _, n := range existing {
				if n.State == "open" || n.State == "running" {
					if in, err := store.ParseIntent(n); err == nil {
						seen[normalizeTitle(in.Title)] = true
					}
				}
			}
			type pendingIntent struct {
				Intent   model.Intent
				Priority int
			}
			pending := []pendingIntent{}
			skipped := []string{}
			for i, in := range args.Intents {
				if i >= 4 {
					break
				}
				if strings.TrimSpace(in.Title) == "" {
					continue
				}
				key := normalizeTitle(in.Title)
				if seen[key] {
					skipped = append(skipped, in.Title)
					continue
				}
				if in.GoalID == "" && len(in.DerivedFromFactIDs) == 0 {
					return "", fmt.Errorf("intent %q needs goalId or derivedFromFactIds", in.Title)
				}
				if in.GoalID != "" {
					goal, err := env.Store.GetGoal(in.GoalID)
					if err != nil {
						return "", err
					}
					if goal.RunID != env.Run.ID {
						return "", fmt.Errorf("goal %s belongs to another run", in.GoalID)
					}
				}
				for _, factID := range in.DerivedFromFactIDs {
					fact, err := env.Store.GetNode(factID)
					if err != nil {
						return "", err
					}
					if fact.RunID != env.Run.ID || fact.Kind != model.KindFact {
						return "", fmt.Errorf("derived fact %s is not in this run", factID)
					}
				}
				for _, assetID := range in.AssetIDs {
					if _, err := env.Store.GetAsset(assetID); err != nil {
						return "", fmt.Errorf("asset %s is not a known asset", assetID)
					}
				}
				seen[key] = true
				pending = append(pending, pendingIntent{Intent: in.Intent, Priority: in.Priority})
			}
			created := []store.Node{}
			for _, item := range pending {
				in := item.Intent
				payload, _ := json.Marshal(in)
				n, err := env.Store.AddNode(store.Node{RunID: env.Run.ID, Kind: model.KindIntent, Payload: payload, Priority: item.Priority, Owner: env.Agent})
				if err != nil {
					return "", err
				}
				if in.GoalID != "" {
					_, _ = env.Store.AddEdge(env.Run.ID, in.GoalID, n.ID, "spawns")
				}
				for _, factID := range in.DerivedFromFactIDs {
					_, _ = env.Store.AddEdge(env.Run.ID, factID, n.ID, "derived_from")
				}
				for _, assetID := range in.AssetIDs {
					_ = env.Store.Anchor(n.ID, assetID)
				}
				created = append(created, n)
			}
			return toJSON(map[string]any{"created": created, "skippedDuplicate": skipped})
		},
	}
}

func intentUpdateTool(env Env) Spec {
	return Spec{
		Name: "intent_update",
		Description: "Change one intent state: open, running, paused, done, blocked, exhausted, stopped, deleted. " +
			"Give a reason when blocking or stopping.",
		Parameters: obj(map[string]any{
			"intentId": str("Intent id."),
			"state":    strEnum("New state.", "open", "running", "paused", "done", "blocked", "exhausted", "stopped", "deleted"),
			"reason":   str("Why the state changed."),
			"priority": integer("Optional new priority."),
		}, "intentId", "state"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				IntentID string `json:"intentId"`
				State    string `json:"state"`
				Reason   string `json:"reason"`
				Priority int    `json:"priority"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			n, err := env.Store.GetNode(args.IntentID)
			if err != nil {
				return "", err
			}
			if n.Kind != model.KindIntent || n.RunID != env.Run.ID {
				return "", fmt.Errorf("%s is not an intent of this run", args.IntentID)
			}
			priority := args.Priority
			if priority == 0 {
				priority = n.Priority
			}
			if err := env.Store.UpdateNodeState(args.IntentID, args.State, priority); err != nil {
				return "", err
			}
			env.emit("intent", "intent_update", "意图状态："+args.State, args.Reason)
			return toJSON(map[string]any{"id": args.IntentID, "state": args.State, "reason": args.Reason})
		},
	}
}

func factAddTool(env Env) Spec {
	return Spec{
		Name: "fact_add",
		Description: "Record one belief update on the exploration graph. basis=observed requires evidence; " +
			"inferred means derived; assumed means unverified; negative records a checked-and-blocked path. " +
			"Prefer one consolidated fact per intent over many fragments.",
		Parameters: obj(map[string]any{
			"intentId":    str("Intent this fact came from."),
			"kind":        strEnum("Fact kind.", "port", "service", "vuln", "http", "source", "credential", "info"),
			"target":      str("Host/URL/service this fact refers to."),
			"detail":      str("The observation and what it means."),
			"basis":       strEnum("Belief basis (default observed).", "observed", "inferred", "assumed", "negative"),
			"confidence":  num("Belief strength 0..1."),
			"provenance":  str("Where the observation came from."),
			"evidenceIds": arr("Existing evidence ids.", str("Evidence id.")),
			"assetIds":    arr("Assets this fact anchors.", str("Asset id.")),
			"evidence": arr("Inline evidence to create first.", obj(map[string]any{
				"kind":    strEnum("Evidence kind.", "http", "command", "file", "text", "image"),
				"label":   str("Short label."),
				"content": str("Evidence body."),
				"refPath": str("Read body from this file instead."),
				"channel": str("Observation channel (error/boolean/time/oob/http)."),
				"layers":  arr("Per-layer interpretations.", str("Layer.")),
			})),
		}, "detail"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				IntentID    string          `json:"intentId"`
				Kind        string          `json:"kind"`
				Target      string          `json:"target"`
				Detail      string          `json:"detail"`
				Basis       string          `json:"basis"`
				Confidence  float64         `json:"confidence"`
				Provenance  string          `json:"provenance"`
				EvidenceIDs []string        `json:"evidenceIds"`
				AssetIDs    []string        `json:"assetIds"`
				Evidence    []evidenceInput `json:"evidence"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			detail := strings.TrimSpace(args.Detail)
			if detail == "" {
				return "", errors.New("detail is required")
			}
			basis := args.Basis
			if basis == "" {
				basis = "observed"
			}
			switch basis {
			case "observed", "inferred", "assumed", "negative":
			default:
				return "", fmt.Errorf("invalid basis %q", basis)
			}
			for _, in := range args.Evidence {
				e, err := addEvidence(env, in)
				if err != nil {
					return "", err
				}
				args.EvidenceIDs = append(args.EvidenceIDs, e.ID)
			}
			if basis == "observed" && len(args.EvidenceIDs) == 0 {
				return "", errors.New("an observed fact needs at least one evidence record; use basis=assumed when unverified")
			}
			confidence := args.Confidence
			if confidence <= 0 {
				confidence = 0.8
				if basis == "assumed" {
					confidence = 0.4
				}
			}
			payload, _ := json.Marshal(model.Fact{
				Kind: args.Kind, Target: args.Target, Detail: detail, Basis: basis,
				Confidence: confidence, Provenance: args.Provenance,
				EvidenceIDs: args.EvidenceIDs, AssetIDs: args.AssetIDs,
			})
			n, err := env.Store.AddNode(store.Node{RunID: env.Run.ID, Kind: model.KindFact, Payload: payload, Owner: env.Agent})
			if err != nil {
				return "", err
			}
			attachToParent(env, n.ID, args.IntentID, "yields", "leads_to")
			for _, assetID := range args.AssetIDs {
				_ = env.Store.Anchor(n.ID, assetID)
				if env.Run.ProjectID != "" {
					_ = env.Store.LinkProjectAsset(env.Run.ProjectID, assetID, env.Agent)
				}
			}
			return toJSON(map[string]any{"id": n.ID, "basis": basis, "evidenceIds": args.EvidenceIDs, "covered": basis == "observed" || basis == "inferred"})
		},
	}
}

func findingAddTool(env Env) Spec {
	return Spec{
		Name: "finding_add",
		Description: "Record a confirmed vulnerability. Requires reproducible steps and evidence; never report " +
			"version/CVE matches or 'looks injectable' as confirmed. Token/assertion issues need a trust boundary.",
		Parameters: obj(map[string]any{
			"intentId":          str("Intent that produced this finding."),
			"vulnclass":         str("Vulnerability class."),
			"title":             str("Short title."),
			"severity":          strEnum("Severity.", "critical", "high", "medium", "low", "info"),
			"summary":           str("One-paragraph summary."),
			"description":       str("Root cause and details."),
			"impact":            str("Impact."),
			"trustBoundary":     str("For token/assertion issues: who asserts, who verifies, what is trusted."),
			"reproducibleSteps": arr("Ordered reproduction steps.", str("Step.")),
			"evidenceIds":       arr("Existing evidence ids.", str("Evidence id.")),
			"assetIds":          arr("Affected asset ids.", str("Asset id.")),
			"evidence": arr("Inline evidence to create first.", obj(map[string]any{
				"kind":    strEnum("Evidence kind.", "http", "command", "file", "text", "image"),
				"label":   str("Short label."),
				"content": str("Evidence body."),
				"refPath": str("Read body from this file instead."),
				"channel": str("Observation channel."),
				"layers":  arr("Per-layer interpretations.", str("Layer.")),
			})),
			"reportMarkdown": str("Optional detailed Markdown report."),
		}, "title", "severity", "reproducibleSteps"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				IntentID       string          `json:"intentId"`
				VulnClass      string          `json:"vulnclass"`
				Title          string          `json:"title"`
				Severity       string          `json:"severity"`
				Summary        string          `json:"summary"`
				Description    string          `json:"description"`
				Impact         string          `json:"impact"`
				TrustBoundary  string          `json:"trustBoundary"`
				Steps          []string        `json:"reproducibleSteps"`
				EvidenceIDs    []string        `json:"evidenceIds"`
				AssetIDs       []string        `json:"assetIds"`
				Evidence       []evidenceInput `json:"evidence"`
				ReportMarkdown string          `json:"reportMarkdown"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			steps := []string{}
			for _, s := range args.Steps {
				if strings.TrimSpace(s) != "" {
					steps = append(steps, s)
				}
			}
			if len(steps) == 0 {
				return "", errors.New("a finding needs at least one reproducible step")
			}
			switch args.Severity {
			case "critical", "high", "medium", "low", "info":
			default:
				return "", fmt.Errorf("invalid severity %q", args.Severity)
			}
			for _, in := range args.Evidence {
				e, err := addEvidence(env, in)
				if err != nil {
					return "", err
				}
				args.EvidenceIDs = append(args.EvidenceIDs, e.ID)
			}
			if len(args.EvidenceIDs) == 0 {
				return "", errors.New("a finding needs at least one evidence record")
			}
			payload, _ := json.Marshal(model.Finding{
				VulnClass: args.VulnClass, Title: args.Title, Severity: args.Severity,
				Summary: args.Summary, Description: args.Description, Impact: args.Impact,
				TrustBoundary: args.TrustBoundary, Steps: steps, EvidenceIDs: args.EvidenceIDs,
				AssetIDs: args.AssetIDs, ReportMarkdown: args.ReportMarkdown,
			})
			n, err := env.Store.AddNode(store.Node{RunID: env.Run.ID, Kind: model.KindFinding, Payload: payload, Owner: env.Agent})
			if err != nil {
				return "", err
			}
			attachToParent(env, n.ID, args.IntentID, "proves", "proves")
			for _, assetID := range args.AssetIDs {
				_ = env.Store.Anchor(n.ID, assetID)
				if env.Run.ProjectID != "" {
					_ = env.Store.LinkProjectAsset(env.Run.ProjectID, assetID, env.Agent)
				}
			}
			env.emit("finding", "finding_add", "确认漏洞："+args.Title, args.Summary)
			return toJSON(map[string]any{"id": n.ID, "title": args.Title, "severity": args.Severity, "evidenceIds": args.EvidenceIDs})
		},
	}
}

func evidenceAddTool(env Env) Spec {
	return Spec{
		Name: "evidence_add",
		Description: "Store one immutable evidence record (sha256 content-addressed). Use it before adding facts; " +
			"facts without evidence do not count toward coverage.",
		Parameters: obj(map[string]any{
			"kind":    strEnum("Evidence kind.", "http", "command", "file", "text", "image"),
			"label":   str("Short label."),
			"content": str("Evidence body (request/response, command output, excerpt)."),
			"refPath": str("Read the body from this file instead."),
			"channel": str("Observation channel (error/boolean/time/oob/http)."),
			"layers":  arr("Per-layer interpretations of the same input.", str("Layer.")),
		}, "label"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var in evidenceInput
			if err := decodeInto(raw, &in); err != nil {
				return "", err
			}
			if in.Content == "" && in.RefPath == "" {
				return "", errors.New("content or refPath is required")
			}
			e, err := addEvidence(env, in)
			if err != nil {
				return "", err
			}
			return toJSON(e)
		},
	}
}

func flowAddTool(env Env) Spec {
	return Spec{
		Name: "flow_add",
		Description: "Record one dataflow / concat point (source -> transform -> sink). Link the finding to the " +
			"sink, not the entry point.",
		Parameters: obj(map[string]any{
			"source":      str("Input source (parameter, header, file, token field)."),
			"transform":   str("Transforms along the path (decode, normalize, truncate, concat)."),
			"sink":        str("Interpreter where the value is used (SQL, shell, template, decoder)."),
			"confidence":  num("Confidence 0..1."),
			"evidenceIds": arr("Supporting evidence ids.", str("Evidence id.")),
		}, "source", "sink"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Source      string   `json:"source"`
				Transform   string   `json:"transform"`
				Sink        string   `json:"sink"`
				Confidence  float64  `json:"confidence"`
				EvidenceIDs []string `json:"evidenceIds"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			f, err := env.Store.AddFlow(store.Flow{
				RunID: env.Run.ID, Source: args.Source, Transform: args.Transform, Sink: args.Sink,
				Confidence: args.Confidence, Evidence: args.EvidenceIDs,
			})
			if err != nil {
				return "", err
			}
			return toJSON(f)
		},
	}
}

func frontierOf(snap store.Snapshot) ([]map[string]any, map[string]any) {
	factIDs := map[string]bool{}
	assetTested := map[string]bool{}
	for _, n := range snap.Nodes {
		switch n.Kind {
		case model.KindFact:
			factIDs[n.ID] = true
			if f, err := store.ParseFact(n); err == nil && len(f.EvidenceIDs) > 0 {
				for _, id := range f.AssetIDs {
					assetTested[id] = true
				}
			}
		case model.KindFinding:
			if f, err := store.ParseFinding(n); err == nil {
				for _, id := range f.AssetIDs {
					assetTested[id] = true
				}
			}
		}
	}
	type item struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		State    string   `json:"state"`
		Priority int      `json:"priority"`
		DepsMet  bool     `json:"depsMet"`
		Missing  []string `json:"missingDeps"`
		AssetIDs []string `json:"assetIds"`
		Score    int      `json:"score"`
	}
	items := []item{}
	for _, n := range snap.Nodes {
		if n.Kind != model.KindIntent || (n.State != "open" && n.State != "running") {
			continue
		}
		in, err := store.ParseIntent(n)
		if err != nil {
			continue
		}
		missing := []string{}
		for _, factID := range in.DerivedFromFactIDs {
			if !factIDs[factID] {
				missing = append(missing, factID)
			}
		}
		depsMet := len(missing) == 0
		score := n.Priority*10 + 20 + len(in.AssetIDs)
		if !depsMet {
			score -= 50
		}
		if n.State == "running" {
			score += 5
		}
		items = append(items, item{ID: n.ID, Title: in.Title, State: n.State, Priority: n.Priority, DepsMet: depsMet, Missing: missing, AssetIDs: in.AssetIDs, Score: score})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	var assumed, negative, openGoals, blocked []string
	for _, n := range snap.Nodes {
		switch n.Kind {
		case model.KindFact:
			if f, err := store.ParseFact(n); err == nil {
				if f.Basis == "assumed" {
					assumed = append(assumed, f.Detail)
				}
				if f.Basis == "negative" {
					negative = append(negative, f.Detail)
				}
			}
		case model.KindIntent:
			if n.State == "blocked" {
				if in, err := store.ParseIntent(n); err == nil {
					blocked = append(blocked, in.Title)
				}
			}
		}
	}
	for _, g := range snap.Goals {
		if g.Status == "open" {
			openGoals = append(openGoals, g.Text)
		}
	}
	untested := []map[string]string{}
	for _, a := range snap.Assets {
		if !assetTested[a.ID] {
			untested = append(untested, map[string]string{"id": a.ID, "type": a.Type, "value": a.Value})
		}
	}
	frontier := []map[string]any{}
	for _, it := range items {
		if len(frontier) >= 12 {
			break
		}
		frontier = append(frontier, map[string]any{
			"id": it.ID, "title": it.Title, "state": it.State, "priority": it.Priority,
			"depsMet": it.DepsMet, "missingDeps": it.Missing, "assetIds": it.AssetIDs, "score": it.Score,
		})
	}
	return frontier, map[string]any{
		"assumedFacts": assumed, "negativeFacts": negative, "openGoals": openGoals,
		"blockedIntents": blocked, "untestedAssets": untested,
	}
}

func counts(snap store.Snapshot) map[string]int {
	out := map[string]int{"assets": len(snap.Assets), "evidence": len(snap.Evidence), "nodes": len(snap.Nodes)}
	for _, n := range snap.Nodes {
		out[n.Kind]++
	}
	return out
}

func normalizeTitle(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func evidenceIDsOf(n store.Node) []string {
	switch n.Kind {
	case model.KindFact:
		if f, err := store.ParseFact(n); err == nil {
			return f.EvidenceIDs
		}
	case model.KindFinding:
		if f, err := store.ParseFinding(n); err == nil {
			return f.EvidenceIDs
		}
	}
	return nil
}

func filterEdges(edges []store.Edge, src, dst string) []store.Edge {
	out := []store.Edge{}
	for _, e := range edges {
		if src != "" && e.SrcID != src {
			continue
		}
		if dst != "" && e.DstID != dst {
			continue
		}
		out = append(out, e)
	}
	return out
}

func toJSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
