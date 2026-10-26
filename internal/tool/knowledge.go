package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/knowledge"
)

const knowledgeHint = " Only store durable methodology; never logs, secrets, credentials or one-off command output."

func knowledgeTools(env Env) []Spec {
	role := env.Role
	if role == "" {
		role = "main"
	}
	load := func(name string) (knowledge.Doc, error) {
		if env.Knowledge == nil {
			return knowledge.Doc{}, fmt.Errorf("knowledge store unavailable")
		}
		return env.Knowledge.Get(role, name)
	}
	return []Spec{
		{
			Name: "knowledge_find", ReadOnly: true,
			Description: "Search your own role's knowledge files by keyword and return matching lines." + knowledgeHint,
			Parameters: obj(map[string]any{
				"keyword": str("Keyword to search for."),
				"limit":   integer("Maximum matches (default 20)."),
			}, "keyword"),
			Fn: func(_ context.Context, raw anyArgs) (string, error) {
				var args struct {
					Keyword string `json:"keyword"`
					Limit   int    `json:"limit"`
				}
				if err := decodeInto(raw, &args); err != nil {
					return "", err
				}
				if env.Knowledge == nil {
					return "", fmt.Errorf("knowledge store unavailable")
				}
				keyword := strings.TrimSpace(args.Keyword)
				if keyword == "" {
					return "", fmt.Errorf("keyword required")
				}
				limit := args.Limit
				if limit <= 0 || limit > 50 {
					limit = 20
				}
				docs, err := env.Knowledge.List(role)
				if err != nil {
					return "", err
				}
				type match struct {
					Name string `json:"name"`
					Line int    `json:"line"`
					Text string `json:"text"`
				}
				out := []match{}
				for _, meta := range docs {
					doc, err := env.Knowledge.Get(role, meta.Name)
					if err != nil {
						continue
					}
					for i, line := range strings.Split(doc.Body, "\n") {
						if strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
							out = append(out, match{Name: doc.Name, Line: i + 1, Text: strings.TrimSpace(line)})
							if len(out) >= limit {
								return toJSON(map[string]any{"matches": out})
							}
						}
					}
				}
				return toJSON(map[string]any{"matches": out})
			},
		},
		{
			Name: "knowledge_load", ReadOnly: true,
			Description: "Load one of your own role's knowledge documents by name." + knowledgeHint,
			Parameters:  obj(map[string]any{"name": str("Knowledge name without .md.")}, "name"),
			Fn: func(_ context.Context, raw anyArgs) (string, error) {
				var args struct {
					Name string `json:"name"`
				}
				if err := decodeInto(raw, &args); err != nil {
					return "", err
				}
				doc, err := load(args.Name)
				if err != nil {
					return "", err
				}
				return toJSON(doc)
			},
		},
		{
			Name:        "knowledge_create",
			Description: "Create a durable methodology note under your own role. Fails if it already exists." + knowledgeHint,
			Parameters: obj(map[string]any{
				"name":        str("Knowledge name without .md."),
				"description": str("Short metadata description."),
				"body":        str("Markdown methodology body."),
			}, "name", "description", "body"),
			Fn: func(_ context.Context, raw anyArgs) (string, error) {
				var args struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Body        string `json:"body"`
				}
				if err := decodeInto(raw, &args); err != nil {
					return "", err
				}
				if env.Knowledge == nil {
					return "", fmt.Errorf("knowledge store unavailable")
				}
				doc, err := env.Knowledge.Create(role, args.Name, args.Description, args.Body)
				if err != nil {
					return "", err
				}
				return toJSON(doc)
			},
		},
		{
			Name:        "knowledge_update",
			Description: "Replace one of your own role's knowledge documents." + knowledgeHint,
			Parameters: obj(map[string]any{
				"name":        str("Knowledge name without .md."),
				"description": str("New metadata description."),
				"body":        str("New Markdown methodology body."),
			}, "name", "description", "body"),
			Fn: func(_ context.Context, raw anyArgs) (string, error) {
				var args struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Body        string `json:"body"`
				}
				if err := decodeInto(raw, &args); err != nil {
					return "", err
				}
				if env.Knowledge == nil {
					return "", fmt.Errorf("knowledge store unavailable")
				}
				doc, err := env.Knowledge.Update(role, args.Name, args.Description, args.Body)
				if err != nil {
					return "", err
				}
				return toJSON(doc)
			},
		},
	}
}
