package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/sing-box-manager/sing-box-manager/backend/internal/core"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Engine    *core.Engine
	Remote    *http.Client
	Token     string
	Auth      AuthOptions
	Telemetry http.Handler
	sessions  sync.Map
	loginMu   sync.Mutex
	attempts  int
	window    time.Time
}

func New(e *core.Engine, state, socket string) (*Server, error) {
	s := &Server{Engine: e}
	if er := os.MkdirAll(state, 0700); er != nil {
		return nil, er
	}
	p := filepath.Join(state, "token")
	b, er := os.ReadFile(p)
	if os.IsNotExist(er) {
		v := make([]byte, 32)
		if _, er = rand.Read(v); er != nil {
			return nil, er
		}
		b = []byte(hex.EncodeToString(v))
		er = core.Atomic(p, b, 0600)
	}
	if er != nil {
		return nil, er
	}
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("token must be a regular owner-only file")
	}
	s.Token = strings.TrimSpace(string(b))
	if len(s.Token) < 32 || len(s.Token) > 256 {
		return nil, fmt.Errorf("token must contain 32–256 characters; manual rotation required")
	}
	if socket != "" {
		s.Remote = &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}}}
	}
	return s, nil
}
func response(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, er error) {
	response(w, 400, map[string]string{"error": core.Redact(er.Error())})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	return json.NewDecoder(r.Body).Decode(v)
}
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if r.URL.Path == "/healthz" {
			response(w, 200, map[string]bool{"ok": true})
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "cross-site request denied", 403)
			return
		}
		if o := r.Header.Get("Origin"); o != "" {
			u, er := url.Parse(o)
			if er != nil || u.Host != r.Host || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Scheme == "http" && secureCookie(r)) {
				http.Error(w, "origin denied", 403)
				return
			}
		}
		if r.URL.Path == "/api/login" && r.Method == "POST" {
			if r.Method != "GET" && r.Header.Get("Origin") == "" && r.Header.Get("X-Requested-With") != "sing-box-manager" {
				http.Error(w, "CSRF header required", 403)
				return
			}

			s.login(w, r)
			return
		}
		c, er := r.Cookie("manager_session")
		if er != nil {
			http.Error(w, "authentication required", 401)
			return
		}
		entry, ok := s.sessions.Load(c.Value)
		if !ok || !time.Now().Before(entry.(*session).expires) {
			s.revoke(c.Value)
			http.Error(w, "session expired", 401)
			return
		}
		if r.Method != "GET" && r.Header.Get("Origin") == "" && r.Header.Get("X-Requested-With") != "sing-box-manager" {
			http.Error(w, "CSRF header required", 403)
			return
		}
		ctx, cancel := context.WithDeadline(r.Context(), entry.(*session).expires)
		stop := context.AfterFunc(entry.(*session).ctx, cancel)
		defer stop()
		defer cancel()
		r = r.WithContext(ctx)
		if r.URL.Path == "/api/logout" && r.Method == "POST" {
			s.revoke(c.Value)
			http.SetCookie(w, &http.Cookie{Name: "manager_session", MaxAge: -1, Path: "/", HttpOnly: true, Secure: secureCookie(r), SameSite: http.SameSiteStrictMode})
			response(w, 200, map[string]bool{"ok": true})
			return
		}
		if r.URL.Path == "/api/telemetry" && s.Telemetry != nil {
			s.Telemetry.ServeHTTP(w, r)
			return
		}
		if s.Remote != nil {
			rr := r.Clone(r.Context())
			rr.URL = &url.URL{Scheme: "http", Host: "helper", Path: r.URL.Path}
			rr.RequestURI = ""
			rr.Header = make(http.Header)
			rr.Header.Set("Content-Type", "application/json")
			res, er := s.Remote.Do(rr)
			if er != nil {
				fail(w, er)
				return
			}
			defer res.Body.Close()
			for k, v := range res.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(res.StatusCode)
			if strings.Contains(res.Header.Get("Content-Type"), "text/event-stream") {
				buf := make([]byte, 4096)
				for {
					n, er := res.Body.Read(buf)
					if n > 0 {
						rc := http.NewResponseController(w)
						rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
						if _, writeErr := w.Write(buf[:n]); writeErr != nil {
							return
						}
						if flushErr := rc.Flush(); flushErr != nil {
							return
						}
					}
					if er != nil {
						break
					}
				}
			} else {
				io.Copy(w, res.Body)
			}
			return
		}
		s.Internal().ServeHTTP(w, r)
	})
}

func (s *Server) Config() (map[string]any, error) {
	var snap core.Snapshot
	if s.Remote != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://helper/api/config", nil)
		r, er := s.Remote.Do(req)
		if er != nil {
			return nil, er
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return nil, fmt.Errorf("helper unavailable")
		}
		if er = json.NewDecoder(r.Body).Decode(&snap); er != nil {
			return nil, er
		}
	} else {
		var er error
		snap, er = s.Engine.Snapshot()
		if er != nil {
			return nil, er
		}
	}
	var c map[string]any
	er := json.Unmarshal(snap.Config, &c)
	return c, er
}
func (s *Server) Internal() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e := s.Engine
		var er error
		switch r.Method + " " + r.URL.Path {
		case "GET /api/config":
			var v core.Snapshot
			v, er = e.Snapshot()
			if er == nil {
				response(w, 200, v)
				return
			}
		case "GET /api/status":
			response(w, 200, e.Status())
			return
		case "GET /api/schema":
			var b []byte
			b, er = e.Schema()
			if er == nil {
				w.Header().Set("Content-Type", "application/json")
				w.Write(b)
				return
			}
		case "PUT /api/draft":
			var d core.Draft
			er = decode(w, r, &d)
			if er == nil {
				er = e.Save(d)
			}
		case "POST /api/validate":
			er = e.Validate()
		case "POST /api/apply":
			er = e.Apply()
		case "GET /api/history":
			var v []core.History
			v, er = e.History()
			if er == nil {
				response(w, 200, v)
				return
			}
		case "POST /api/restore":
			var q struct {
				ID string `json:"id"`
			}
			er = decode(w, r, &q)
			if er == nil {
				er = e.Restore(q.ID)
			}
		case "POST /api/monitoring/prepare":
			er = s.prepare()
		case "GET /api/logs":
			s.logs(w, r)
			return
		default:
			http.NotFound(w, r)
			return
		}
		if er != nil {
			fail(w, er)
		} else {
			response(w, 200, map[string]bool{"ok": true})
		}
	})
}
func (s *Server) prepare() error {
	snap, er := s.Engine.Snapshot()
	if er != nil {
		return er
	}
	var c map[string]any
	b := snap.Config
	if snap.Draft != nil {
		if snap.Draft.BaseHash != snap.Hash {
			return fmt.Errorf("live configuration changed; reconcile the saved draft before preparing monitoring")
		}
		b = snap.Draft.Config
	}
	schema, err := s.Engine.Schema()
	if err != nil {
		return fmt.Errorf("cannot detect native API support: %w", err)
	}
	if !supportsAPI(schema) {
		return fmt.Errorf("installed sing-box schema does not support the native API service")
	}

	if er = json.Unmarshal(b, &c); er != nil {
		return er
	}
	services, _ := c["services"].([]any)
	for _, v := range services {
		m, _ := v.(map[string]any)
		if m["type"] == "api" {
			return fmt.Errorf("native API service already configured")
		}
	}
	key := make([]byte, 32)
	if _, er = rand.Read(key); er != nil {
		return er
	}
	c["services"] = append(services, map[string]any{"type": "api", "tag": "manager-api", "listen": "127.0.0.1", "listen_port": 9095, "secret": hex.EncodeToString(key), "dashboard": false})
	b, er = json.MarshalIndent(c, "", "  ")
	if er != nil {
		return er
	}
	if er = s.Engine.Save(core.Draft{Config: b, BaseHash: snap.Hash}); er != nil {
		return er
	}
	return s.Engine.Validate()
}

// Traverse the installed services item schema, following only local references.
func supportsAPI(raw []byte) bool {
	var schema map[string]any
	if json.Unmarshal(raw, &schema) != nil {
		return false
	}
	props, _ := schema["properties"].(map[string]any)
	services, _ := props["services"].(map[string]any)
	var walk func(any, int) bool
	walk = func(v any, depth int) bool {
		if depth > 32 {
			return false
		}
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		if ref, ok := m["$ref"].(string); ok && strings.HasPrefix(ref, "#/") {
			var target any = schema
			for _, p := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
				obj, ok := target.(map[string]any)
				if !ok {
					return false
				}
				target = obj[strings.ReplaceAll(strings.ReplaceAll(p, "~1", "/"), "~0", "~")]
			}
			if walk(target, depth+1) {
				return true
			}
		}
		p, _ := m["properties"].(map[string]any)
		typ, _ := p["type"].(map[string]any)
		if typ["const"] == "api" {
			return true
		}
		for _, key := range []string{"oneOf", "anyOf", "allOf"} {
			a, _ := m[key].([]any)
			for _, item := range a {
				if walk(item, depth+1) {
					return true
				}
			}
		}
		return false
	}
	return walk(services["items"], 0)
}
func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	lines := make(chan string, 64)
	done := make(chan error, 1)
	go func() {
		done <- s.Engine.Journal(ctx, func(line string) error {
			select {
			case lines <- line:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	write := func(message string) error {
		rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := io.WriteString(w, message); err != nil {
			return err
		}
		return rc.Flush()
	}
	if write(": connected\n\n") != nil {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case line := <-lines:
			b, _ := json.Marshal(map[string]string{"line": line})
			if write(fmt.Sprintf("data: %s\n\n", b)) != nil {
				return
			}
		case err := <-done:
			if err != nil && ctx.Err() == nil {
				b, _ := json.Marshal(map[string]string{"error": core.Redact(err.Error())})
				write(fmt.Sprintf("event: error\ndata: %s\n\n", b))
			}
			return
		case <-ticker.C:
			if write(": heartbeat\n\n") != nil {
				return
			}
		}
	}
}
