package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Explicit opt-in: starts only a disposable binary and loopback listeners.
func TestRealSingBoxNativeAPI(t *testing.T) {
	binary := os.Getenv("SING_BOX_BIN")
	if binary == "" {
		t.Skip("set SING_BOX_BIN to a verified sing-box 1.14 binary")
	}
	apiPort, proxyPort := freePort(t), freePort(t)
	config := map[string]any{
		"log":       map[string]any{"level": "error"},
		"inbounds":  []any{map[string]any{"type": "http", "tag": "test-in", "listen": "127.0.0.1", "listen_port": proxyPort}},
		"outbounds": []any{map[string]any{"type": "direct", "tag": "test-out"}},
		"services":  []any{map[string]any{"type": "api", "tag": "manager-api", "listen": "127.0.0.1", "listen_port": apiPort, "secret": "isolated-test-only"}},
	}
	path := filepath.Join(t.TempDir(), "config.json")
	data, _ := json.Marshal(config)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
		t.Fatalf("check: %s: %v", output, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "run", "-c", path)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	monitor := New(func() (map[string]any, error) { return config, nil })
	go monitor.Run(ctx)
	await(t, 10*time.Second, func() bool { return monitor.Snapshot().Available })
	first := monitor.Snapshot()
	if first.Version == "" || first.UploadTotal == nil {
		t.Fatalf("missing real capabilities: %+v", first)
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", 256*1024)))
	}))
	defer target.Close()
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
	transport := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if err != nil || n != 256*1024 {
		t.Fatalf("proxy payload %d: %v", n, err)
	}
	await(t, 5*time.Second, func() bool {
		s := monitor.Snapshot()
		return s.DownloadTotal != nil && *s.DownloadTotal >= 256*1024 && s.UploadRate != nil
	})
	s := monitor.Snapshot()
	t.Logf("native API=%s, upload=%d, download=%d, connections=%v, startedAt=%v", s.Version, *s.UploadTotal, *s.DownloadTotal, s.ConnectionsAvailable, s.StartedAt)
}
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func await(t *testing.T, d time.Duration, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for real integration condition")
}
