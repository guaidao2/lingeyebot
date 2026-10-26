package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/guaidao2/lingeyebot/internal/config"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSDKClientListsAndCallsTools(t *testing.T) {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	sdkmcp.AddTool(server, &sdkmcp.Tool{Name: "echo", Description: "Echo text"},
		func(ctx context.Context, req *sdkmcp.CallToolRequest, args map[string]any) (*sdkmcp.CallToolResult, any, error) {
			text, _ := args["text"].(string)
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "pong:" + text}},
			}, nil, nil
		})
	clientTransport, serverTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	client := &sdkClient{transport: clientTransport}
	if err := client.initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer client.close()

	tools, err := client.listTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" || len(tools[0].Schema) == 0 {
		t.Fatalf("tools = %+v", tools)
	}
	out, err := client.call(context.Background(), "echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "pong:hi") {
		t.Fatalf("call output = %q", out)
	}
}

func TestReconnectRebuildsClients(t *testing.T) {
	m := NewManager([]config.MCPServer{{ID: "bad", Transport: "http", URL: "http://127.0.0.1:1/mcp", Enabled: true}}, "")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := m.Reconnect(ctx); err == nil {
		t.Fatal("reconnect to unreachable server should report an error")
	}
	status := m.Status()
	if len(status) != 1 || status[0].State != "error" {
		t.Fatalf("status = %+v", status)
	}
}
