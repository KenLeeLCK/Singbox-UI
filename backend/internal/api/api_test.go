package api

import (
	"bytes"
	"encoding/json"
	"github.com/sing-box-manager/sing-box-manager/backend/internal/core"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthenticationAndCSRF(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "config.json")
	os.WriteFile(p, []byte(`{}`), 0600)
	e, _ := core.New(core.Options{StateDir: d, Config: p, Local: true})
	s, er := New(e, d, "")
	if er != nil {
		t.Fatal(er)
	}
	h := s.Handler()
	call := func(method, path, body, origin string, c *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewBufferString(body))
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if c != nil {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/api/config", "", "", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	b, _ := json.Marshal(map[string]string{"token": s.Token})
	if w := call("POST", "/api/login", string(b), "https://evil.test", nil); w.Code != 403 {
		t.Fatal("cross-origin login accepted")
	}
	w := call("POST", "/api/login", string(b), "http://localhost", nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	c := w.Result().Cookies()[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	if w = call("GET", "/api/config", "", "", c); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = call("PUT", "/api/draft", `{}`, "", c); w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
}

func TestPrepareRejectsStaleDraftAndUnsupportedSchema(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "config.json")
	os.WriteFile(p, []byte(`{}`), 0600)
	binary := filepath.Join(d, "sing-box")
	os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s' '{\"properties\":{}}'\n"), 0700)
	e, _ := core.New(core.Options{StateDir: filepath.Join(d, "state"), Config: p, Binary: binary, Local: true})
	snap, _ := e.Snapshot()
	draft := core.Draft{Config: json.RawMessage(`{"log":{"level":"debug"}}`), BaseHash: snap.Hash}
	e.Save(draft)
	os.WriteFile(p, []byte(`{"log":{"level":"warn"}}`), 0600)
	s := &Server{Engine: e}
	if err := s.prepare(); err == nil || !strings.Contains(err.Error(), "reconcile") {
		t.Fatal(err)
	}
	after, _ := e.Snapshot()
	if after.Draft.BaseHash != draft.BaseHash || string(after.Draft.Config) != string(draft.Config) {
		t.Fatal("stale draft silently rebased")
	}
	os.Remove(filepath.Join(e.StateDir, "draft.json"))
	if err := s.prepare(); err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatal(err)
	}
	after, _ = e.Snapshot()
	if after.Draft != nil {
		t.Fatal("unsupported feature created draft")
	}
}
func TestNativeAPISchemaCapability(t *testing.T) {
	supported := []byte(`{"properties":{"services":{"items":{"$ref":"#/$defs/Service"}}},"$defs":{"Service":{"oneOf":[{"properties":{"type":{"const":"api"}}}]}}}`)
	if !supportsAPI(supported) {
		t.Fatal("supported schema rejected")
	}
	if supportsAPI([]byte(`{"$defs":{"unused":{"properties":{"type":{"const":"api"}}}}}`)) {
		t.Fatal("unreachable definition accepted")
	}
}
func TestLogoutRequiresPOST(t *testing.T) {
	s := &Server{}
	s.sessions.Store("test", newSession(time.Now().Add(time.Hour)))
	r := httptest.NewRequest("GET", "/api/logout", nil)
	r.AddCookie(&http.Cookie{Name: "manager_session", Value: "test"})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	if _, ok := s.sessions.Load("test"); !ok {
		t.Fatal("GET logged user out")
	}
}

func TestSessionTransportAndExpiry(t *testing.T) {
	for _, tc := range []struct {
		name, origin, forwarded string
		secure                  bool
	}{
		{"localhost", "http://localhost", "", false},
		{"serve-preserves-host", "https://localhost", "https", true},
		{"spoofed-forwarded", "http://localhost", "https", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{Token: "isolated-test-token"}
			call := func(path, origin, token string, cookie *http.Cookie) *httptest.ResponseRecorder {
				body, _ := json.Marshal(map[string]string{"token": token})
				r := httptest.NewRequest("POST", "http://localhost"+path, bytes.NewReader(body))
				r.Header.Set("Origin", origin)
				r.Header.Set("X-Forwarded-Proto", tc.forwarded)
				r.Header.Set("X-Forwarded-Host", "evil.test")
				if cookie != nil {
					r.AddCookie(cookie)
				}
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, r)
				return w
			}
			if w := call("/api/login", tc.origin, "wrong", nil); w.Code != 401 {
				t.Fatal(w.Code)
			}
			w := call("/api/login", tc.origin, s.Token, nil)
			if w.Code != 200 {
				t.Fatal(w.Code)
			}
			c := w.Result().Cookies()[0]
			if c.Secure != tc.secure || !c.HttpOnly || c.Path != "/" || c.MaxAge != 28800 || c.SameSite != http.SameSiteStrictMode {
				t.Fatal("incorrect cookie attributes")
			}
			if w := call("/api/logout", "https://evil.test", "", c); w.Code != 403 {
				t.Fatal("cross origin accepted")
			}
			w = call("/api/logout", tc.origin, "", c)
			if w.Code != 200 || w.Result().Cookies()[0].MaxAge != -1 || w.Result().Cookies()[0].Secure != tc.secure {
				t.Fatal("logout failed")
			}
			if w := call("/api/logout", tc.origin, "", c); w.Code != 401 {
				t.Fatal("session survived logout")
			}
			s.sessions.Store(c.Value, newSession(time.Now().Add(-time.Second)))
			if w := call("/api/logout", tc.origin, "", c); w.Code != 401 {
				t.Fatal("expired session accepted")
			}
			if _, ok := s.sessions.Load(c.Value); ok {
				t.Fatal("expired session retained")
			}
		})
	}
}
