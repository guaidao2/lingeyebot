package llm

import (
	"fmt"
	"net/http"
	"strings"
)

// New builds a provider from adapter config.
func New(c Config, client *http.Client) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(c.Kind)) {
	case "", "openai":
		if strings.TrimSpace(c.BaseURL) == "" {
			c.BaseURL = "https://api.openai.com/v1"
		}
		return NewOpenAI(c, client), nil
	case "anthropic":
		if strings.TrimSpace(c.BaseURL) == "" {
			c.BaseURL = "https://api.anthropic.com"
		}
		return NewAnthropic(c, client), nil
	default:
		return nil, fmt.Errorf("unsupported provider kind %q", c.Kind)
	}
}
