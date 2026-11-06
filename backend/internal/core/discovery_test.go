package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoveryRelativeDirectory(t *testing.T) {
	for _, tc := range []struct{ wd, expected string }{{"/srv/proxy", "/srv/proxy/data"}, {"", "/data"}} {
		o := Options{}
		parseDiscovery(&o, "WorkingDirectory="+tc.wd+"\nExecStart={ path=/usr/bin/sing-box ; argv[]=/usr/bin/sing-box run -c config.json --directory=data ; ignore_errors=no ; pid=123 ; }\n")
		if o.WorkingDir != tc.expected || o.Config != filepath.Join(tc.expected, "config.json") || o.ReadOnly {
			t.Fatalf("wrong relative resolution %+v", o)
		}
	}
}
func TestDiscoveryWrapperReadOnly(t *testing.T) {
	o := Options{}
	parseDiscovery(&o, "WorkingDirectory=/srv\nExecStart={ path=/bin/sh ; argv[]=/bin/sh -c /usr/bin/sing-box run -c config.json ; pid=1 ; }\n")
	if !o.ReadOnly {
		t.Fatal("shell wrapper writable")
	}
}
func discoveryFixture(t *testing.T, args string) (*Engine, string) {
	t.Helper()
	d := t.TempDir()
	fixture := filepath.Join(d, "effective-unit")
	binary := filepath.Join(d, "sing-box")
	os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0700)
	os.WriteFile(filepath.Join(d, "config.json"), []byte(`{}`), 0600)
	unit := fmt.Sprintf("WorkingDirectory=%s\nExecStart={ path=%s ; argv[]=%s run %s ; ignore_errors=no ; pid=123 ; start_time=old ; }\n", d, binary, binary, args)
	os.WriteFile(fixture, []byte(unit), 0600)
	script := "#!/bin/sh\nif [ \"$1\" = show ]; then cat \"$MANAGER_TEST_UNIT_FILE\"; else exit 0; fi\n"
	os.WriteFile(filepath.Join(d, "systemctl"), []byte(script), 0700)
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MANAGER_TEST_UNIT_FILE", fixture)
	e, er := New(Options{StateDir: filepath.Join(d, "state")})
	if er != nil {
		t.Fatal(er)
	}
	return e, fixture
}
func TestDiscoveryServiceChangeRejectsDraftApply(t *testing.T) {
	e, fixture := discoveryFixture(t, "-c config.json")
	snap, er := e.Snapshot()
	if er != nil {
		t.Fatal(er)
	}
	if er = e.Save(Draft{Config: []byte(`{"log":{"level":"debug"}}`), BaseHash: snap.Hash}); er != nil {
		t.Fatal(er)
	}
	unit, _ := os.ReadFile(fixture)
	os.WriteFile(fixture, []byte(strings.ReplaceAll(string(unit), "pid=123 ; start_time=old", "pid=456 ; start_time=new")), 0600)
	if _, er = e.Snapshot(); er != nil {
		t.Fatal("runtime PID/timestamps caused false conflict", er)
	}
	unit, _ = os.ReadFile(fixture)
	os.WriteFile(fixture, []byte(strings.ReplaceAll(string(unit), "-c config.json", "-c newer.json")), 0600)
	if er = e.Apply(); er == nil || !strings.Contains(er.Error(), "rediscover") {
		t.Fatal("changed unit deploy accepted", er)
	}
	live, _ := os.ReadFile(e.Config)
	if string(live) != "{}" {
		t.Fatal("old source overwritten after service change")
	}
}
func TestDiscoveryDirectoryRefresh(t *testing.T) {
	e, fixture := discoveryFixture(t, "-C parts")
	d := filepath.Dir(fixture)
	os.Mkdir(filepath.Join(d, "parts"), 0700)
	os.WriteFile(filepath.Join(d, "parts", "one.json"), []byte(`{}`), 0600)
	if er := e.refreshDiscovery(); er != nil {
		t.Fatal(er)
	}
	if len(e.Sources) != 1 || !e.ReadOnly {
		t.Fatalf("missing initial directory source %+v", e.Options)
	}
	os.WriteFile(filepath.Join(d, "parts", "two.json"), []byte(`{}`), 0600)
	if er := e.refreshDiscovery(); er != nil {
		t.Fatal(er)
	}
	if len(e.Sources) != 2 || e.Config != "" {
		t.Fatalf("added directory source invisible %+v", e.Options)
	}
	os.Remove(filepath.Join(d, "parts", "one.json"))
	if er := e.refreshDiscovery(); er != nil {
		t.Fatal(er)
	}
	if len(e.Sources) != 1 || !strings.HasSuffix(e.Config, "two.json") {
		t.Fatal("deleted directory source retained")
	}
}
func TestDiscoveryDirectorySnapshotTracksContent(t *testing.T) {
	realBinary := os.Getenv("SING_BOX_TEST_BINARY")
	if realBinary == "" {
		t.Skip("requires actual sing-box merge")
	}
	e, fixture := discoveryFixture(t, "-C parts")
	os.Remove(e.Binary)
	if er := os.Symlink(realBinary, e.Binary); er != nil {
		t.Fatal(er)
	}
	d := filepath.Join(filepath.Dir(fixture), "parts")
	os.Mkdir(d, 0700)
	os.WriteFile(filepath.Join(d, "one.json"), []byte(`{}`), 0600)
	before, er := e.Snapshot()
	if er != nil {
		t.Fatal(er)
	}
	os.WriteFile(filepath.Join(d, "two.json"), []byte(`{"outbounds":[{"type":"direct","tag":"new"}]}`), 0600)
	after, er := e.Snapshot()
	if er != nil {
		t.Fatal(er)
	}
	if before.Hash == after.Hash || !strings.Contains(string(after.Config), `"new"`) || len(after.Sources) != 2 || !after.ReadOnly {
		t.Fatalf("directory change not imported %+v", after)
	}
}
