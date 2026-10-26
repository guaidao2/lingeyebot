package mcp

import (
	"fmt"
	"net/http"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guaidao2/lingeyebot/internal/config"
)

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for key, value := range t.headers {
		clone.Header.Set(key, value)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

// newHTTPClient builds the official SDK's streamable HTTP transport.
func newHTTPClient(cfg config.MCPServer) (*sdkClient, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("http MCP server %q has no url", cfg.ID)
	}
	client := &http.Client{Transport: headerTransport{base: http.DefaultTransport, headers: cfg.Headers}}
	return &sdkClient{transport: &sdkmcp.StreamableClientTransport{
		Endpoint:             cfg.URL,
		HTTPClient:           client,
		DisableStandaloneSSE: true,
	}}, nil
}
