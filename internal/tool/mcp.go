package tool

import (
	"context"
	"encoding/json"
	"strings"
)

func mcpTools(env Env) []Spec {
	out := []Spec{}
	for _, t := range env.MCP.Tools() {
		item := t
		name := "mcp__" + sanitize(item.Server) + "__" + sanitize(item.Name)
		schema := item.Schema
		if len(schema) == 0 {
			schema = obj(map[string]any{})
		}
		desc := item.Description
		if desc == "" {
			desc = "MCP tool " + item.Name + " from server " + item.Server
		}
		out = append(out, Spec{
			Name:        name,
			Description: "[MCP:" + item.Server + "] " + desc,
			Parameters:  schema,
			Fn: func(ctx context.Context, raw anyArgs) (string, error) {
				return env.MCP.Call(ctx, item.Server, item.Name, json.RawMessage(raw))
			},
		})
	}
	return out
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
