package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AuthOptions apply to the single administrator account across all ingress paths.
// No client IP or forwarding header is trusted or used as a rate-limit key.
type AuthOptions struct {
	LoginAttempts int
	LoginWindow   time.Duration
	SessionTTL    time.Duration
}

func (s *Server) authOptions() AuthOptions {
	o := s.Auth
	if o.LoginAttempts <= 0 {
		o.LoginAttempts = 10
	}
	if o.LoginWindow <= 0 {
		o.LoginWindow = time.Minute
	}
	if o.SessionTTL <= 0 {
		o.SessionTTL = 8 * time.Hour
	}
	return o
}

type session struct {
	expires time.Time
	ctx     context.Context
	cancel  context.CancelFunc
}

func newSession(expires time.Time) *session {
	ctx, cancel := context.WithDeadline(context.Background(), expires)
	return &session{expires: expires, ctx: ctx, cancel: cancel}
}
func (s *Server) revoke(key string) {
	if value, ok := s.sessions.LoadAndDelete(key); ok {
		value.(*session).cancel()
	}
}

// Public hosts must use Secure cookies even on non-browser requests lacking
// Origin. Only explicit localhost/loopback HTTP origins can use tunnel cookies.
// Forwarded headers never influence this decision.
func secureCookie(r *http.Request) bool {
	if r.TLS != nil || strings.HasPrefix(r.Header.Get("Origin"), "https://") {
		return true
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	return host != "localhost" && (ip == nil || !ip.IsLoopback())
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	o := s.authOptions()
	now := time.Now()
	// Reserve before reading the body. Concurrent slow requests cannot exceed the
	// failure budget. Successful logins refund their slot only in the same window.
	s.loginMu.Lock()
	if now.Sub(s.window) >= o.LoginWindow {
		s.window = now
		s.attempts = 0
	}
	window := s.window
	if s.attempts >= o.LoginAttempts {
		retry := int(time.Until(s.window.Add(o.LoginWindow)).Seconds()) + 1
		s.loginMu.Unlock()
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		http.Error(w, "too many login attempts; retry later", http.StatusTooManyRequests)
		return
	}
	s.attempts++
	s.loginMu.Unlock()
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer rc.SetReadDeadline(time.Time{})
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var q struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(r.Body)
	if decoder.Decode(&q) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		http.Error(w, "invalid login request", 400)
		return
	}
	// Compare fixed-length digests; do not leak differences in token length.
	got, want := sha256.Sum256([]byte(q.Token)), sha256.Sum256([]byte(s.Token))
	if s.Token == "" || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
		http.Error(w, "invalid token", 401)
		return
	}
	v := make([]byte, 32)
	if _, err := rand.Read(v); err != nil {
		http.Error(w, "cannot create session", 500)
		return
	}
	key := hex.EncodeToString(v)
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.window.Equal(window) {
		s.attempts--
	}
	// Rotate any presented session and cap total state, evicting the oldest.
	if c, err := r.Cookie("manager_session"); err == nil {
		s.revoke(c.Value)
	}
	count := 0
	oldestKey := ""
	var oldest time.Time
	s.sessions.Range(func(k, v any) bool {
		entry := v.(*session)
		if !now.Before(entry.expires) {
			s.revoke(k.(string))
			return true
		}
		count++
		if oldestKey == "" || entry.expires.Before(oldest) {
			oldestKey = k.(string)
			oldest = entry.expires
		}
		return true
	})
	if count >= 32 {
		s.revoke(oldestKey)
	}
	s.sessions.Store(key, newSession(now.Add(o.SessionTTL)))
	http.SetCookie(w, &http.Cookie{Name: "manager_session", Value: key, Path: "/", HttpOnly: true, Secure: secureCookie(r), SameSite: http.SameSiteStrictMode, MaxAge: max(1, int(o.SessionTTL.Seconds()))})
	response(w, 200, map[string]bool{"ok": true})
}
