package llm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fastRetry(t *testing.T) {
	t.Helper()
	oldAttempts, oldBase := llmMaxAttempts, llmBaseDelay
	llmMaxAttempts, llmBaseDelay = 3, time.Millisecond
	t.Cleanup(func() { llmMaxAttempts, llmBaseDelay = oldAttempts, oldBase })
}

func TestOpenAIRetriesTransientStatuses(t *testing.T) {
	fastRetry(t)
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		switch attempts {
		case 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("rate limited"))
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
		}
	}))
	defer srv.Close()

	p := NewOpenAI(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}, srv.Client())
	resp, err := p.Chat(t.Context(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "ok" || attempts != 3 {
		t.Fatalf("content=%q attempts=%d", resp.Content, attempts)
	}
}

func TestOpenAIDoesNotRetryBadRequest(t *testing.T) {
	fastRetry(t)
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	p := NewOpenAI(Config{BaseURL: srv.URL, Model: "m"}, srv.Client())
	if _, err := p.Chat(t.Context(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}); err == nil {
		t.Fatal("expected 400 error")
	}
	if attempts != 1 {
		t.Fatalf("retried non-retryable status: %d", attempts)
	}
}

func TestOpenAIStreamRetriesBeforeFirstToken(t *testing.T) {
	fastRetry(t)
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := NewOpenAI(Config{BaseURL: srv.URL, Model: "m"}, srv.Client())
	var deltas strings.Builder
	resp, err := p.ChatStream(t.Context(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}, func(s string) {
		deltas.WriteString(s)
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || resp.Content != "Hello" || deltas.String() != "Hello" {
		t.Fatalf("attempts=%d content=%q deltas=%q", attempts, resp.Content, deltas.String())
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Now()
	if got := parseRetryAfter("2", now); got != 2*time.Second {
		t.Fatalf("seconds = %v", got)
	}
	when := now.Add(5 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(when, now); got <= 0 {
		t.Fatalf("date = %v", got)
	}
}
