package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func authRequest(s *Server, method, path, token string, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"token": token})
	r := httptest.NewRequest(method, "http://127.0.0.1"+path, bytes.NewReader(body))
	r.Header.Set("X-Requested-With", "sing-box-manager")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestAllAPIsRequireSession(t *testing.T) {
	s := &Server{Token: strings.Repeat("a", 64)}
	for _, route := range []string{"GET config", "GET status", "GET schema", "GET history", "GET logs", "GET telemetry", "PUT draft", "POST validate", "POST apply", "POST restore", "POST monitoring/prepare", "POST logout"} {
		parts := strings.Split(route, " ")
		for _, cookie := range []*http.Cookie{nil, {Name: "manager_session", Value: "forged"}} {
			w := authRequest(s, parts[0], "/api/"+parts[1], "", cookie, nil)
			if w.Code != 401 || w.Header().Get("WWW-Authenticate") != "" {
				t.Fatalf("%s: %d", route, w.Code)
			}
		}
	}
}
func TestGlobalFailedLoginBudgetAndRecovery(t *testing.T) {
	s := &Server{Token: strings.Repeat("a", 64), Auth: AuthOptions{LoginAttempts: 3, LoginWindow: time.Minute}}
	good := func() *httptest.ResponseRecorder { return authRequest(s, "POST", "/api/login", s.Token, nil, nil) }
	for i := 0; i < 6; i++ {
		if w := good(); w.Code != 200 {
			t.Fatal("success consumed failure budget", w.Code)
		}
	}
	for i := 0; i < 3; i++ {
		if w := authRequest(s, "POST", "/api/login", "wrong", nil, map[string]string{"X-Forwarded-For": strconv.Itoa(i), "X-Real-IP": strconv.Itoa(i)}); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	window := s.window
	for i := 0; i < 4; i++ {
		w := good()
		if w.Code != 429 || w.Header().Get("Retry-After") == "" {
			t.Fatal("missing throttle", w.Code)
		}
	}
	if !window.Equal(s.window) {
		t.Fatal("blocked traffic extended lockout")
	}
	s.window = time.Now().Add(-time.Minute)
	if w := good(); w.Code != 200 {
		t.Fatal("failed to recover", w.Code)
	}
}
func TestConcurrentFailuresCannotBypassBudget(t *testing.T) {
	s := &Server{Token: strings.Repeat("a", 64)}
	var wg sync.WaitGroup
	var mu sync.Mutex
	statuses := map[int]int{}
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := authRequest(s, "POST", "/api/login", "wrong", nil, nil)
			mu.Lock()
			statuses[w.Code]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if statuses[401] != 10 || statuses[429] != 30 {
		t.Fatal(statuses)
	}
}
func TestSessionRotationBoundAndPublicCookie(t *testing.T) {
	s := &Server{Token: strings.Repeat("a", 64)}
	login := func(c *http.Cookie) *http.Cookie {
		w := authRequest(s, "POST", "/api/login", s.Token, c, nil)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		return w.Result().Cookies()[0]
	}
	first := login(nil)
	second := login(first)
	if first.Value == second.Value {
		t.Fatal("fixed session")
	}
	if _, ok := s.sessions.Load(first.Value); ok {
		t.Fatal("old session retained")
	}
	for i := 0; i < 40; i++ {
		login(nil)
	}
	count := 0
	s.sessions.Range(func(k, v any) bool { count++; return true })
	if count != 32 {
		t.Fatal(count)
	}
	for _, host := range []string{"103.100.176.139:5030", "example.ts.net"} {
		r := httptest.NewRequest("POST", "http://"+host+"/api/login", strings.NewReader(`{"token":"`+s.Token+`"}`))
		r.Header.Set("X-Requested-With", "sing-box-manager")
		r.Header.Set("X-Forwarded-Proto", "http")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		c := w.Result().Cookies()[0]
		if !c.Secure || !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteStrictMode {
			t.Fatal("unsafe public cookie")
		}
	}
}
func TestLoginBoundsAndOrigin(t *testing.T) {
	for _, body := range []string{strings.Repeat("x", 2048), `{"token":"wrong"} {}`, `{"token":`} {
		s := &Server{Token: strings.Repeat("a", 64)}
		r := httptest.NewRequest("POST", "http://localhost/api/login", strings.NewReader(body))
		r.Header.Set("X-Requested-With", "sing-box-manager")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	for _, origin := range []string{"https://evil.test", "null", "https://user@127.0.0.1", "https://127.0.0.1/path", "https://127.0.0.1?x=1"} {
		s := &Server{Token: strings.Repeat("a", 64)}
		w := authRequest(s, "POST", "/api/login", s.Token, nil, map[string]string{"Origin": origin})
		if w.Code != 403 {
			t.Fatal(origin, w.Code)
		}
	}
}
func TestOpenStreamsEndAtLogoutAndExpiry(t *testing.T) {
	for _, logout := range []bool{true, false} {
		s := &Server{Token: strings.Repeat("a", 64), Auth: AuthOptions{SessionTTL: 150 * time.Millisecond}}
		started, ended := make(chan struct{}), make(chan struct{})
		s.Telemetry = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(ended) })
		w := authRequest(s, "POST", "/api/login", s.Token, nil, nil)
		cookie := w.Result().Cookies()[0]
		go authRequest(s, "GET", "/api/telemetry", "", cookie, nil)
		<-started
		if logout {
			if w := authRequest(s, "POST", "/api/logout", "", cookie, nil); w.Code != 200 {
				t.Fatal(w.Code)
			}
		}
		select {
		case <-ended:
		case <-time.After(time.Second):
			t.Fatal("authenticated stream survived revocation/expiry")
		}
		if w := authRequest(s, "GET", "/api/config", "", cookie, nil); w.Code != 401 {
			t.Fatal("expired cookie accepted", w.Code)
		}
	}
}
