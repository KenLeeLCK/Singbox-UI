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
	if len(e.Sources) != 1 || e.ReadOnly {
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
	if er := e.refreshDiscovery(); er == nil || !strings.Contains(er.Error(), "rediscover") {
		t.Fatal("source replacement did not require rediscovery", er)
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

func TestSingleDirectoryLayouts(t *testing.T) {
	for _, tc := range []struct {
		name, args string
		files      []string
		readonly   bool
	}{
		{"production backup", "-D %s -C %s run", []string{"config.json", "config.json.bak"}, false},
		{"long flags", "--directory=%s --config-directory=%s run", []string{"config.json"}, false},
		{"multiple", "-D %s -C %s run", []string{"config.json", "other.json"}, true},
		{"hidden active", "-D %s -C %s run", []string{"config.json", ".hidden.json"}, true},
		{"ignored suffixes", "-D %s -C %s run", []string{"config.json", "other.jsonc", "other.JSON"}, false},
		{"jsonc only", "-D %s -C %s run", []string{"config.jsonc"}, true},
		{"empty", "-D %s -C %s run", nil, true},
		{"unknown flag", "-D %s -C %s run --unknown", []string{"config.json"}, true},
		{"extra argument", "-D %s -C %s run extra", []string{"config.json"}, true},
		{"missing argument", "-D %s -C %s run -c", []string{"config.json"}, true},
		{"mixed sources", "-D %s -C %s run -c config.json", []string{"config.json"}, true},
		{"repeated directory", "-D %s -C %s run -C .", []string{"config.json"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			os.Mkdir(filepath.Join(dir, "nested.json"), 0700)
			o := Options{}
			parseDiscovery(&o, "WorkingDirectory=\nExecStart={ path=/usr/bin/sing-box ; argv[]=/usr/bin/sing-box "+fmt.Sprintf(tc.args, dir, dir)+" ; }")
			if o.ReadOnly != tc.readonly {
				t.Fatalf("unexpected discovery %+v", o)
			}
			if !tc.readonly && (o.Config != filepath.Join(dir, "config.json") || !o.configDirectory || o.WorkingDir != dir) {
				t.Fatalf("wrong effective source %+v", o)
			}
		})
	}
}

func TestDirectoryUnsupportedEntries(t *testing.T) {
	for _, layout := range []string{"file", "directory", "directory-trailing-slash"} {
		t.Run(layout, func(t *testing.T) {
			dir := t.TempDir()
			parts := filepath.Join(dir, "parts")
			os.Mkdir(parts, 0700)
			os.WriteFile(filepath.Join(dir, "original.json"), []byte(`{}`), 0600)
			if layout != "file" {
				os.WriteFile(filepath.Join(parts, "config.json"), []byte(`{}`), 0600)
				os.Symlink(parts, filepath.Join(dir, "link"))
				parts = filepath.Join(dir, "link")
				if layout == "directory-trailing-slash" {
					parts += "/"
				}
			} else {
				os.Symlink(filepath.Join(dir, "original.json"), filepath.Join(parts, "config.json"))
			}
			o := Options{}
			parseDiscovery(&o, "ExecStart={ argv[]=/usr/bin/sing-box -C "+parts+" run ; }")
			if !o.ReadOnly {
				t.Fatal("symlink layout writable")
			}
		})
	}
}

func TestDirectoryValidationIsolatedAndRechecksSources(t *testing.T) {
	e, fixture := discoveryFixture(t, "-C .")
	snap, err := e.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Save(Draft{Config: []byte(`{}`), BaseHash: snap.Hash}); err != nil {
		t.Fatal(err)
	}
	// Validation must pass -C to an isolated directory and retain the effective -D.
	script := `#!/bin/sh
[ "$1" = check ] && [ "$2" = -C ] && [ "$4" = -D ] || exit 1
[ "$3" != "$5" ] && [ "$PWD" = "$5" ] || exit 2
[ "$(find "$3" -name '*.json' | wc -l | tr -d ' ')" = 1 ] || exit 3
[ "$(find "$5" -maxdepth 1 -name '*.json' | wc -l | tr -d ' ')" = 1 ] || exit 4
printf '{}' > "$5/added.json"
`
	os.WriteFile(e.Binary, []byte(script), 0700)
	if err = e.Apply(); err == nil || !strings.Contains(err.Error(), "sources changed") {
		t.Fatal("topology change accepted", err)
	}
	live, _ := os.ReadFile(filepath.Join(filepath.Dir(fixture), "config.json"))
	if string(live) != "{}" {
		t.Fatal("live file changed")
	}
	entries, _ := filepath.Glob(filepath.Join(e.StateDir, "check-*"))
	if len(entries) != 0 {
		t.Fatal("validation leaked candidates")
	}
}

func TestDirectoryRealBinaryValidation(t *testing.T) {
	binary := os.Getenv("SING_BOX_TEST_BINARY")
	if binary == "" {
		t.Skip("requires installed sing-box")
	}
	e, _ := discoveryFixture(t, "-C .")
	os.Remove(e.Binary)
	if err := os.Symlink(binary, e.Binary); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(filepath.Dir(e.Config), "config.json.bak"), []byte("invalid backup"), 0600)
	s, err := e.Snapshot()
	if err != nil || s.ReadOnly {
		t.Fatal("backup affected import", err)
	}
	if err = e.Save(Draft{Config: []byte(`{}`), BaseHash: s.Hash}); err != nil {
		t.Fatal(err)
	}
	// A relative rule-set must resolve from -D, not from the isolated candidate directory.
	os.WriteFile(filepath.Join(e.WorkingDir, "rules.source"), []byte(`{"version":3,"rules":[]}`), 0600)
	candidate := []byte(`{"route":{"rule_set":[{"type":"local","tag":"test","format":"source","path":"rules.source"}]}}`)
	if err = e.Save(Draft{Config: candidate, BaseHash: s.Hash}); err != nil {
		t.Fatal(err)
	}
	if err = e.Validate(); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(e.WorkingDir, "rules.source"))
	if err = e.Validate(); err == nil {
		t.Fatal("missing relative resource accepted")
	}
	os.WriteFile(e.Config, []byte("{ // comment\n}"), 0600)
	s, err = e.Snapshot()
	if err != nil || !s.ReadOnly {
		t.Fatal("JSONC writable", err)
	}
}
