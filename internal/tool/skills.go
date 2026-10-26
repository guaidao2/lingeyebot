package tool

import (
	"context"
	"errors"
)

func skillTools(env Env) []Spec {
	return []Spec{skillListTool(env), skillReadTool(env)}
}

func skillListTool(env Env) Spec {
	return Spec{
		Name: "skill_list", ReadOnly: true,
		Description: "List available skills (name, description, path). Skills are progressive: read one only " +
			"when its description matches the current task.",
		Parameters: obj(map[string]any{}),
		Fn: func(_ context.Context, _ anyArgs) (string, error) {
			items := []map[string]string{}
			for _, s := range env.Skills.List() {
				items = append(items, map[string]string{"name": s.Name, "description": s.Description, "path": s.Path})
			}
			return toJSON(items)
		},
	}
}

func skillReadTool(env Env) Spec {
	return Spec{
		Name: "skill_read", ReadOnly: true,
		Description: "Read one skill's full SKILL.md body. Follow it as a playbook for the current task.",
		Parameters: obj(map[string]any{
			"name": str("Skill name from skill_list."),
		}, "name"),
		Fn: func(_ context.Context, raw anyArgs) (string, error) {
			var args struct {
				Name string `json:"name"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			s, ok := env.Skills.Get(args.Name)
			if !ok {
				return "", errors.New("unknown skill " + args.Name)
			}
			return s.Body, nil
		},
	}
}
