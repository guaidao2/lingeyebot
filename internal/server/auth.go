package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "lingeyebot_session"

type authStatus struct {
	Enabled       bool   `json:"enabled"`
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.Config()
	status := authStatus{Enabled: cfg.Auth.Enabled, Username: cfg.Auth.Username}
	status.Authenticated = !cfg.Auth.Enabled || s.validSession(r)
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg := s.Config()
	status := authStatus{Enabled: cfg.Auth.Enabled, Username: cfg.Auth.Username}
	if cfg.Auth.Enabled {
		if bcrypt.CompareHashAndPassword([]byte(cfg.Auth.PasswordHash), []byte(req.Password)) != nil {
			writeErr(w, http.StatusUnauthorized, fmt.Errorf("密码不正确"))
			return
		}
		s.setSession(w, r, cfg.Auth.Username, cfg.Auth.Secret, cfg.Auth.SessionDays)
		status.Authenticated = true
	} else {
		status.Authenticated = true
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleLogout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username        string `json:"username"`
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if len(req.NewPassword) < 8 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("新密码至少 8 位"))
		return
	}
	cfg := s.Config()
	if cfg.Auth.Enabled && bcrypt.CompareHashAndPassword([]byte(cfg.Auth.PasswordHash), []byte(req.CurrentPassword)) != nil {
		writeErr(w, http.StatusUnauthorized, fmt.Errorf("当前密码不正确"))
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	cfg.Auth.Secret = base64.RawURLEncoding.EncodeToString(secret)
	cfg.Auth.PasswordHash = string(hash)
	cfg.Auth.Enabled = true
	if strings.TrimSpace(req.Username) != "" {
		cfg.Auth.Username = strings.TrimSpace(req.Username)
	}
	if cfg.Auth.Username == "" {
		cfg.Auth.Username = "bot"
	}
	if err := s.ApplyConfig(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, authStatus{Enabled: true, Authenticated: false, Username: cfg.Auth.Username})
}

func (s *Server) setSession(w http.ResponseWriter, r *http.Request, username, secret string, days int) {
	if days <= 0 {
		days = 7
	}
	expiry := time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	value := fmt.Sprintf("%s|%d|%s", username, expiry, signSession(secret, username, expiry))
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/", MaxAge: days * 24 * 3600,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) validSession(r *http.Request) bool {
	cfg := s.Config()
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return false
	}
	parts := strings.Split(cookie.Value, "|")
	if len(parts) != 3 {
		return false
	}
	username, expiryText, signature := parts[0], parts[1], parts[2]
	if username != cfg.Auth.Username {
		return false
	}
	expiry, err := strconv.ParseInt(expiryText, 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return false
	}
	expected := signSession(cfg.Auth.Secret, username, expiry)
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}

func signSession(secret, username string, expiry int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%s|%d", username, expiry)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// authMiddleware protects API calls when the personal account is enabled.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := s.Config()
		if !cfg.Auth.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		switch r.URL.Path {
		case "/api/health", "/api/auth/status", "/api/auth/login":
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") || s.validSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		writeErr(w, http.StatusUnauthorized, fmt.Errorf("需要登录"))
	})
}
