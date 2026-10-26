package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/store"
)

func boardTools(env Env) []Spec {
	return []Spec{boardWriteTool(env), boardReadTool(env)}
}

func boardWriteTool(env Env) Spec {
	return Spec{
		Name: "board_write",
		Description: "Write one shared project blackboard entry. scope=project is visible to every conversation; " +
			"pinned entries also go into project memory and every prompt. Use it for durable facts, decisions, " +
			"credentials, contacts or hints — not for task-local scratch work.",
		Parameters: obj(map[string]any{
			"kind":         strEnum("Entry kind.", "fact", "decision", "credential", "contact", "hint", "note"),
			"title":        str("Short title (also the memory key when pinned)."),
			"detail":       str("What future conversations must know, with evidence or provenance."),
			"pinned":       boolean("Also write project memory and inject it into every prompt."),
			"sourceNodeId": str("Optional conversation node this entry derives from."),
		}, "kind", "detail"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Kind         string `json:"kind"`
				Title        string `json:"title"`
				Detail       string `json:"detail"`
				Pinned       bool   `json:"pinned"`
				SourceNodeID string `json:"sourceNodeId"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			if strings.TrimSpace(args.Detail) == "" {
				return "", fmt.Errorf("detail is required")
			}
			projectID := env.Run.ProjectID
			if projectID == "" {
				return "", fmt.Errorf("this run is not attached to a project")
			}
			kind := args.Kind
			if kind == "" {
				kind = "note"
			}
			payload, _ := json.Marshal(map[string]string{"title": args.Title, "detail": args.Detail})
			node, err := env.Store.AddProjectNode(store.ProjectNode{
				ProjectID: projectID, ConversationID: env.Run.ID, Kind: kind, Payload: payload,
				Pinned: args.Pinned, Origin: env.Agent, Owner: env.Agent, SourceNodeID: args.SourceNodeID,
			})
			if err != nil {
				return "", err
			}
			var memory *store.ProjectMemory
			if args.Pinned {
				key := args.Title
				if strings.TrimSpace(key) == "" {
					key = kind + "-" + node.ID
				}
				saved, err := env.Store.SetProjectMemory(projectID, key, args.Detail, kind, node.ID, env.Agent)
				if err != nil {
					return "", err
				}
				memory = &saved
			}
			return toJSON(map[string]any{"node": node, "memory": memory})
		},
	}
}

func boardReadTool(env Env) Spec {
	return Spec{
		Name: "board_read", ReadOnly: true,
		Description: "Read the project blackboard: compact project memory plus recent shared nodes. Call this before " +
			"planning so you do not redo work another conversation already did.",
		Parameters: obj(map[string]any{
			"kind":  strEnum("Optional node kind filter.", "fact", "decision", "credential", "contact", "hint", "note"),
			"limit": integer("Maximum nodes (default 50)."),
		}),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Kind  string `json:"kind"`
				Limit int    `json:"limit"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			projectID := env.Run.ProjectID
			if projectID == "" {
				return "", fmt.Errorf("this run is not attached to a project")
			}
			limit := args.Limit
			if limit <= 0 {
				limit = 50
			}
			memory, err := env.Store.ListProjectMemory(projectID, 100)
			if err != nil {
				return "", err
			}
			nodes, err := env.Store.ListProjectNodes(projectID, limit)
			if err != nil {
				return "", err
			}
			filtered := make([]store.ProjectNode, 0, len(nodes))
			for _, node := range nodes {
				if args.Kind == "" || node.Kind == args.Kind {
					filtered = append(filtered, node)
				}
			}
			return toJSON(map[string]any{"memory": memory, "nodes": filtered})
		},
	}
}
