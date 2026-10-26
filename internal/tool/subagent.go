package tool

import (
	"context"
	"errors"
	"fmt"
)

func subagentTools(env Env) []Spec {
	return []Spec{subagentTool(env)}
}

func subagentTool(env Env) Spec {
	return Spec{
		Name: "subagent",
		Description: "Manage delegated tasks. Roles: intel-collector (OSINT and company intel), enumerator " +
			"(active enumeration), threat-modeler (attack/fragility modeling), pre-pentester (get a foothold), " +
			"post-pentester (internal weaknesses), evasion (cover tracks in red-team actions), worker (free-form, " +
			"define the task yourself). create runs in the background and returns immediately: end your turn after " +
			"create and wait for the completion report; do not poll. send queues an instruction; cancel is idempotent; " +
			"check/list only when the user asks for progress or you must cancel.",
		Parameters: obj(map[string]any{
			"action":         strEnum("Action.", "create", "send", "cancel", "check", "list"),
			"role":           strEnum("Role for create.", "intel-collector", "enumerator", "threat-modeler", "pre-pentester", "post-pentester", "evasion", "worker"),
			"task":           str("Required for create: the concrete task for the subagent."),
			"scope":          str("Required context for create: authorized scope and boundaries."),
			"context":        str("Required context for create: current facts, failed paths and relevant node ids."),
			"expectedOutput": str("Required context for create: what evidence/result the subagent must return."),
			"label":          str("Short display label."),
			"nodeId":         str("Optional intent node this task serves."),
			"budget":         integer("Optional iteration budget; 0 / omitted = unlimited."),
			"task_id":        str("Task id for send/cancel/check."),
			"message":        str("Instruction for send."),
		}, "action"),
		Fn: func(ctx context.Context, raw anyArgs) (string, error) {
			var args struct {
				Action         string `json:"action"`
				Role           string `json:"role"`
				Task           string `json:"task"`
				Scope          string `json:"scope"`
				Context        string `json:"context"`
				ExpectedOutput string `json:"expectedOutput"`
				Label          string `json:"label"`
				NodeID         string `json:"nodeId"`
				Budget         int    `json:"budget"`
				TaskID         string `json:"task_id"`
				Message        string `json:"message"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			switch args.Action {
			case "create":
				if args.Task == "" {
					return "", errors.New("create requires task")
				}
				t, err := env.Subagents.Spawn(ctx, env.Run.ID, SpawnInput{
					Role: args.Role, Task: args.Task, Scope: args.Scope, Context: args.Context,
					ExpectedOutput: args.ExpectedOutput, Label: args.Label, NodeID: args.NodeID, Budget: args.Budget,
				})
				if err != nil {
					return "", err
				}
				return toJSON(t)
			case "send":
				if args.TaskID == "" || args.Message == "" {
					return "", errors.New("send requires task_id and message")
				}
				if err := env.Subagents.Send(ctx, env.Run.ID, args.TaskID, args.Message); err != nil {
					return "", err
				}
				return fmt.Sprintf("queued for %s (accepted does not mean executed yet)", args.TaskID), nil
			case "cancel":
				if args.TaskID == "" {
					return "", errors.New("cancel requires task_id")
				}
				if err := env.Subagents.Cancel(ctx, env.Run.ID, args.TaskID); err != nil {
					return "", err
				}
				return "cancelled " + args.TaskID, nil
			case "check":
				if args.TaskID == "" {
					tasks, err := env.Subagents.List(ctx, env.Run.ID)
					if err != nil {
						return "", err
					}
					return toJSON(tasks)
				}
				t, err := env.Subagents.Check(ctx, env.Run.ID, args.TaskID)
				if err != nil {
					return "", err
				}
				return toJSON(t)
			case "list":
				tasks, err := env.Subagents.List(ctx, env.Run.ID)
				if err != nil {
					return "", err
				}
				return toJSON(tasks)
			default:
				return "", fmt.Errorf("unknown action %q", args.Action)
			}
		},
	}
}
