package mcp

import (
	"fmt"
	"os"
	"os/exec"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guaidao2/lingeyebot/internal/config"
)

// newStdioClient builds the official SDK's command transport.
func newStdioClient(cfg config.MCPServer, workspace string) (*sdkClient, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("stdio MCP server %q has no command", cfg.ID)
	}
	cmd := exec.Command(cfg.Command, cfg.Args...)
	if workspace != "" {
		cmd.Dir = workspace
	}
	cmd.Env = os.Environ()
	for key, value := range cfg.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	return &sdkClient{transport: &sdkmcp.CommandTransport{Command: cmd}}, nil
}
