package server

import (
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

func TestPersonalAccountFlow(t *testing.T) {
	_, _, ts := newTestServer(t, true)

	status, body := request(t, ts, "GET", "/api/auth/status", nil)
	if status != http.StatusOK {
		t.Fatalf("auth status: %d %s", status, body)
	}

	status, body = request(t, ts, "POST", "/api/auth/password", map[string]any{
		"username": "bot", "newPassword": "secret123",
	})
	if status != http.StatusOK {
		t.Fatalf("set password: %d %s", status, body)
	}

	status, _ = request(t, ts, "GET", "/api/projects", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("protected endpoint should require login, got %d", status)
	}

	status, _ = request(t, ts, "POST", "/api/auth/login", map[string]any{"password": "wrong"})
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong password should fail, got %d", status)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	loginReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/login", strings.NewReader(`{"password":"secret123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := client.Do(loginReq)
	if err != nil {
		t.Fatal(err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login failed: %d", loginResp.StatusCode)
	}

	projectsReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/projects", nil)
	projectsResp, err := client.Do(projectsReq)
	if err != nil {
		t.Fatal(err)
	}
	projectsResp.Body.Close()
	if projectsResp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated request failed: %d", projectsResp.StatusCode)
	}
}
