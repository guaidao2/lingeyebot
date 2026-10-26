package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func execTools(env Env) []Spec {
	return []Spec{shellTool(env)}
}

func shellTool(env Env) Spec {
	return Spec{
		Name:      "shell",
		Dangerous: true,
		Description: "Execute a shell command in the workspace. On Linux uses bash -lc, on Windows uses PowerShell. " +
			"Commands are authorized by the policy gate; destructive commands may require user approval. " +
			"Prefer non-interactive commands with a bounded runtime.",
		Parameters: obj(map[string]any{
			"command":        str("Command line to execute."),
			"cwd":            str("Working directory; defaults to the run workspace."),
			"timeoutSeconds": integer("Hard timeout in seconds (default 120, max 1800)."),
		}, "command"),
		Fn: func(ctx context.Context, raw anyArgs) (string, error) {
			var args struct {
				Command        string `json:"command"`
				Cwd            string `json:"cwd"`
				TimeoutSeconds int    `json:"timeoutSeconds"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			if strings.TrimSpace(args.Command) == "" {
				return "", errors.New("command is required")
			}
			timeout := args.TimeoutSeconds
			if timeout <= 0 {
				timeout = 120
			}
			if timeout > 1800 {
				timeout = 1800
			}
			runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
			defer cancel()

			dir := env.resolve(args.Cwd)
			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				shell := "powershell"
				if _, err := exec.LookPath("pwsh"); err == nil {
					shell = "pwsh"
				}
				cmd = exec.CommandContext(runCtx, shell, "-NoProfile", "-NonInteractive", "-Command", args.Command)
			} else {
				shell := "/bin/bash"
				if _, err := exec.LookPath("bash"); err != nil {
					shell = "/bin/sh"
				}
				cmd = exec.CommandContext(runCtx, shell, "-lc", args.Command)
			}
			cmd.Dir = dir
			cmd.WaitDelay = 2 * time.Second
			buf := &cappedBuffer{limit: 200_000}
			cmd.Stdout = buf
			cmd.Stderr = buf
			err := cmd.Run()
			if errors.Is(err, exec.ErrWaitDelay) {
				// A background child exited 0 while holding inherited pipes;
				// the command itself succeeded.
				err = nil
			}
			body := buf.String()
			if buf.truncated {
				body += "\n...(output truncated)"
			}
			exit := 0
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					exit = exitErr.ExitCode()
				} else if runCtx.Err() == context.DeadlineExceeded {
					exit = -1
					body += fmt.Sprintf("\n[timeout after %ds]", timeout)
				} else {
					exit = -1
					body += "\n[exec error] " + err.Error()
				}
			}
			return fmt.Sprintf("exit=%d\n%s", exit, body), nil
		},
	}
}

type anyArgs = []byte

func decodeInto(raw anyArgs, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return jsonUnmarshal(raw, v)
}

// cappedBuffer keeps child process output bounded while reporting a full write
// so the child never blocks on a full pipe.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.limit <= 0 {
		c.limit = 200_000
	}
	remaining := c.limit - c.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = c.buf.Write(p[:remaining])
			c.truncated = true
		} else {
			_, _ = c.buf.Write(p)
		}
	} else {
		c.truncated = true
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }
