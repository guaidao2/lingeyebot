package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/guaidao2/lingeyebot/internal/store"
)

func reportTools(env Env) []Spec {
	return []Spec{reportRenderTool(env), reportExportTool(env)}
}

func reportRenderTool(env Env) Spec {
	return Spec{
		Name: "report_render", ReadOnly: true,
		Description: "Render the engagement as Markdown, including goals, exploration chain, facts, findings, " +
			"assets, dataflows, evidence, retests and belief gaps.",
		Parameters: obj(map[string]any{}),
		Fn: func(_ context.Context, _ anyArgs) (string, error) {
			return env.Store.Report(env.Run.ID)
		},
	}
}

func reportExportTool(env Env) Spec {
	return Spec{
		Name: "report_export",
		Description: "Write the delivery bundle into the workspace: report.md, findings.json, assets.json, " +
			"graph.json and coverage.json. Returns the output directory.",
		Parameters: obj(map[string]any{
			"dir": str("Optional output directory (default workspace/reports/<run>)."),
		}),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Dir string `json:"dir"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			dir := env.resolve(args.Dir)
			if args.Dir == "" {
				dir = filepath.Join(env.Workspace, "reports", env.Run.ID)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
			report, err := env.Store.Report(env.Run.ID)
			if err != nil {
				return "", err
			}
			snap, err := env.Store.Snapshot(env.Run.ID)
			if err != nil {
				return "", err
			}
			cov, err := env.Store.Coverage(env.Run.ID)
			if err != nil {
				return "", err
			}
			findings := []json.RawMessage{}
			for _, n := range snap.Nodes {
				if n.Kind == "finding" {
					findings = append(findings, n.Payload)
				}
			}
			files := map[string][]byte{
				"report.md":     []byte(report),
				"findings.json": mustMarshal(findings),
				"assets.json":   mustMarshal(snap.Assets),
				"graph.json":    mustMarshal(map[string]any{"nodes": snap.Nodes, "edges": snap.Edges, "anchors": snap.Anchors}),
				"coverage.json": mustMarshal(cov),
			}
			written := []string{}
			for name, data := range files {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, data, 0o644); err != nil {
					return "", err
				}
				written = append(written, path)
			}
			return toJSON(map[string]any{"dir": dir, "files": written})
		},
	}
}

func mustMarshal(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte("[]")
	}
	return b
}

var _ = store.Snapshot{}
