package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// sdkClient adapts the official MCP Go SDK to the manager's small interface.
type sdkClient struct {
	transport sdkmcp.Transport
	session   *sdkmcp.ClientSession
}

func (c *sdkClient) initialize(ctx context.Context) error {
	client := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name: "lingeyebot", Version: "0.2.0",
	}, nil)
	session, err := client.Connect(ctx, c.transport, nil)
	if err != nil {
		return err
	}
	c.session = session
	return nil
}

func (c *sdkClient) listTools(ctx context.Context) ([]Tool, error) {
	if c.session == nil {
		return nil, fmt.Errorf("MCP session is not initialized")
	}
	result, err := c.session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		return nil, err
	}
	out := make([]Tool, 0, len(result.Tools))
	for _, tool := range result.Tools {
		schema := json.RawMessage(`{"type":"object","properties":{}}`)
		if tool.InputSchema != nil {
			if encoded, err := json.Marshal(tool.InputSchema); err == nil {
				schema = encoded
			}
		}
		out = append(out, Tool{Name: tool.Name, Description: tool.Description, Schema: schema})
	}
	return out, nil
}

func (c *sdkClient) call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	if c.session == nil {
		return "", fmt.Errorf("MCP session is not initialized")
	}
	arguments := map[string]any{}
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return "", fmt.Errorf("MCP arguments must be a JSON object: %w", err)
		}
	}
	result, err := c.session.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, content := range result.Content {
		switch item := content.(type) {
		case *sdkmcp.TextContent:
			b.WriteString(item.Text)
			b.WriteString("\n")
		default:
			if encoded, err := json.Marshal(item); err == nil {
				b.WriteString(string(encoded))
				b.WriteString("\n")
			}
		}
	}
	if result.StructuredContent != nil {
		if encoded, err := json.MarshalIndent(result.StructuredContent, "", "  "); err == nil {
			b.Write(encoded)
			b.WriteString("\n")
		}
	}
	text := b.String()
	if result.IsError {
		return "", fmt.Errorf("%s", text)
	}
	if text == "" {
		return "ok", nil
	}
	return text, nil
}

func (c *sdkClient) close() error {
	if c.session == nil {
		if transport, ok := c.transport.(*sdkmcp.CommandTransport); ok && transport.Command != nil && transport.Command.Process != nil {
			_ = transport.Command.Process.Kill()
		}
		return nil
	}
	return c.session.Close()
}
