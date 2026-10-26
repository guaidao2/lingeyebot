package tool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func httpTools(env Env) []Spec {
	return []Spec{httpRequestTool(env)}
}

func httpRequestTool(env Env) Spec {
	client := &http.Client{Timeout: 0}
	return Spec{
		Name: "http_request",
		Description: "Send one HTTP request and return status, headers and body. " +
			"Only http/https URLs are allowed. Large bodies are truncated.",
		Parameters: obj(map[string]any{
			"method":         strEnum("HTTP method (default GET).", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"),
			"url":            str("Absolute http(s) URL."),
			"headers":        map[string]any{"type": "object", "description": "Request headers.", "additionalProperties": map[string]any{"type": "string"}},
			"body":           str("Request body."),
			"timeoutSeconds": integer("Timeout in seconds (default 30, max 600)."),
		}, "url"),
		Fn: func(ctx context.Context, raw anyArgs) (string, error) {
			var args struct {
				Method         string            `json:"method"`
				URL            string            `json:"url"`
				Headers        map[string]string `json:"headers"`
				Body           string            `json:"body"`
				TimeoutSeconds int               `json:"timeoutSeconds"`
			}
			if err := decodeInto(raw, &args); err != nil {
				return "", err
			}
			u, err := url.Parse(strings.TrimSpace(args.URL))
			if err != nil {
				return "", err
			}
			if u.Scheme != "http" && u.Scheme != "https" {
				return "", errors.New("only http and https URLs are supported")
			}
			timeout := args.TimeoutSeconds
			if timeout <= 0 {
				timeout = 30
			}
			if timeout > 600 {
				timeout = 600
			}
			reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
			defer cancel()
			method := strings.ToUpper(strings.TrimSpace(args.Method))
			if method == "" {
				method = http.MethodGet
			}
			var bodyReader io.Reader
			if args.Body != "" {
				bodyReader = strings.NewReader(args.Body)
			}
			req, err := http.NewRequestWithContext(reqCtx, method, u.String(), bodyReader)
			if err != nil {
				return "", err
			}
			for k, v := range args.Headers {
				req.Header.Set(k, v)
			}
			resp, err := client.Do(req)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			if err != nil {
				return "", err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%s %s\n%s\n", resp.Proto, resp.Status, u.String())
			for k, values := range resp.Header {
				fmt.Fprintf(&b, "%s: %s\n", k, strings.Join(values, ", "))
			}
			b.WriteString("\n")
			text := string(body)
			if len(body) >= 2<<20 {
				text += "\n...(body truncated at 2MB)"
			}
			b.WriteString(text)
			_ = env
			return b.String(), nil
		},
	}
}
