package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripConflictAndHistory(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "config.json")
	original := []byte(`{"outbounds":[{"type":"direct","tag":"direct","future_field":{"x":1}}],"route":{"final":"direct"}}`)
	os.WriteFile(p, original, 0640)
	e, er := New(Options{StateDir: filepath.Join(d, "state"), Config: p, Local: true})
	if er != nil {
		t.Fatal(er)
	}
	s, _ := e.Snapshot()
	if er = e.Save(Draft{original, s.Hash}); er != nil {
		t.Fatal(er)
	}
	s, _ = e.Snapshot()
	if string(s.Draft.Config) != string(original) {
		t.Fatal("unknown fields changed")
	}
	os.WriteFile(p, []byte(`{}`), 0640)
	if er = e.Save(Draft{original, s.Hash}); er == nil {
		t.Fatal("stale draft accepted")
	}
	if er = e.Apply(); er == nil {
		t.Fatal("local deploy accepted")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "{}" {
		t.Fatal("live config modified")
	}
	id := "20261106T120000.000000001"
	os.WriteFile(filepath.Join(e.StateDir, "history", id+".json"), original, 0600)
	if er = e.Restore(id); er != nil {
		t.Fatal(er)
	}
	if er = e.Restore("../../config"); er == nil {
		t.Fatal("traversal accepted")
	}
}
func TestDependencies(t *testing.T) {
	cases := []string{`{"outbounds":[{"tag":"x"},{"tag":"x"}]}`, `{"inbounds":[{"listen_port":65536}]}`, `{"outbounds":[{"tag":"s","outbounds":["missing"]}]}`, `{"route":{"rules":[{"rule_set":"missing"}]}}`, `{"route":{"final":"missing"}}`, `null`, `[]`}
	for _, c := range cases {
		if ValidateJSON([]byte(c)) == nil {
			t.Errorf("accepted %s", c)
		}
	}
	if er := ValidateJSON([]byte(`{"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"direct"}}`)); er != nil {
		t.Fatal(er)
	}
}
func TestAtomic(t *testing.T) {
	p := filepath.Join(t.TempDir(), "test")
	if er := Atomic(p, []byte("hello"), 0600); er != nil {
		t.Fatal(er)
	}
	s, _ := os.Stat(p)
	if s.Mode().Perm() != 0600 {
		t.Fatal("mode lost")
	}
}
func TestRedact(t *testing.T) {
	for _, s := range []string{"password=abc", `{"secret":"abc"}`, "authorization: Bearer abc", "https://name:abc@example.com"} {
		if strings.Contains(Redact(s), "abc") {
			t.Errorf("secret leaked: %s", Redact(s))
		}
	}
}
func TestRecoveryRestoresBeforeRestartFailure(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "config")
	backup := filepath.Join(d, "backup")
	os.WriteFile(p, []byte("new"), 0600)
	os.WriteFile(backup, []byte("old"), 0600)
	e, _ := New(Options{StateDir: d, Config: p, Local: true})
	b, _ := json.Marshal(transaction{Path: p, Backup: backup, Mode: 0600})
	os.WriteFile(filepath.Join(d, "transaction.json"), b, 0600)
	if er := e.Recover(); er == nil {
		t.Fatal("local restart should fail")
	}
	b, _ = os.ReadFile(p)
	if string(b) != "old" {
		t.Fatal("rollback did not restore")
	}
	if _, er := os.Stat(filepath.Join(d, "transaction.json")); er != nil {
		t.Fatal("unconfirmed rollback record removed")
	}
}
func TestRealBinarySchemaAndCheck(t *testing.T) {
	binary := os.Getenv("SING_BOX_TEST_BINARY")
	if binary == "" {
		t.Skip("set SING_BOX_TEST_BINARY for installed integration")
	}
	d := t.TempDir()
	p := filepath.Join(d, "config.json")
	os.WriteFile(p, []byte(`{}`), 0600)
	e, _ := New(Options{StateDir: filepath.Join(d, "state"), Config: p, Binary: binary, Local: true})
	b, er := e.Schema()
	if er != nil || !json.Valid(b) {
		t.Fatalf("schema: %v", er)
	}
	s, _ := e.Snapshot()
	e.Save(Draft{Config: json.RawMessage(`{"outbounds":[{"type":"direct","tag":"direct"}]}`), BaseHash: s.Hash})
	if er = e.Validate(); er != nil {
		t.Fatal(er)
	}
	e.Save(Draft{Config: json.RawMessage(`{"outbounds":[{"type":"imaginary"}]}`), BaseHash: s.Hash})
	if er = e.Validate(); er == nil {
		t.Fatal("invalid protocol accepted")
	}
}

func TestApplyRollbackAndRecoveryWithIsolatedServiceDouble(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "bin")
	os.Mkdir(bin, 0700)
	// Only this test's PATH resolves systemctl here. No host service is touched.
	script := `#!/bin/sh
if [ "$1" = restart ] && [ -f "$MANAGER_TEST_FAIL_ONCE" ]; then rm "$MANAGER_TEST_FAIL_ONCE"; exit 1; fi
exit 0
`
	os.WriteFile(filepath.Join(bin, "systemctl"), []byte(script), 0700)
	checker := filepath.Join(bin, "sing-box")
	os.WriteFile(checker, []byte("#!/bin/sh\nexit 0\n"), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	failPath := filepath.Join(d, "fail")
	t.Setenv("MANAGER_TEST_FAIL_ONCE", failPath)
	p := filepath.Join(d, "config.json")
	old := []byte(`{"log":{"level":"info"}}`)
	os.WriteFile(p, old, 0640)
	e, er := New(Options{StateDir: filepath.Join(d, "state"), Config: p, Binary: checker})
	if er != nil {
		t.Fatal(er)
	}
	s, _ := e.Snapshot()
	if er = e.Save(Draft{Config: json.RawMessage(`{"log":{"level":"debug"}}`), BaseHash: s.Hash}); er != nil {
		t.Fatal(er)
	}
	os.WriteFile(failPath, []byte("fail"), 0600)
	if er = e.Apply(); er == nil {
		t.Fatal("failed restart accepted")
	}
	b, _ := os.ReadFile(p)
	if string(b) != string(old) {
		t.Fatal("failed deploy did not roll back")
	}
	if _, er = os.Stat(filepath.Join(e.StateDir, "transaction.json")); !os.IsNotExist(er) {
		t.Fatal("recovered transaction not resolved")
	}
	if er = e.Apply(); er != nil {
		t.Fatal(er)
	}
	b, _ = os.ReadFile(p)
	if !strings.Contains(string(b), "debug") {
		t.Fatal("successful apply not persisted")
	}
	// Model a helper crash after replacement but before confirmation.
	backup := filepath.Join(e.StateDir, "history", "crash.json")
	os.WriteFile(backup, old, 0600)
	tx, _ := json.Marshal(transaction{Path: p, Backup: backup, Mode: 0640})
	os.WriteFile(filepath.Join(e.StateDir, "transaction.json"), tx, 0600)
	if er = e.Recover(); er != nil {
		t.Fatal(er)
	}
	b, _ = os.ReadFile(p)
	if string(b) != string(old) {
		t.Fatal("crash recovery failed")
	}
}

func TestDuplicateListenersAndCrossTags(t *testing.T) {
	for _, raw := range []string{`{"outbounds":[{"type":"direct","tag":"same"}],"endpoints":[{"type":"wireguard","tag":"same"}]}`, `{"inbounds":[{"type":"mixed","listen":"127.0.0.1","listen_port":8080},{"type":"http","listen":"127.0.0.1","listen_port":8080}]}`, `{"inbounds":[{"type":"http","listen":"::1","listen_port":8080}],"services":[{"type":"api","listen":"0:0:0:0:0:0:0:1","listen_port":8080}]}`} {
		if ValidateJSON([]byte(raw)) == nil {
			t.Errorf("accepted conflict %s", raw)
		}
	}
	if er := ValidateJSON([]byte(`{"inbounds":[{"type":"http","listen":"127.0.0.1","listen_port":8080},{"type":"hysteria2","listen":"127.0.0.1","listen_port":8080}]}`)); er != nil {
		t.Fatal("distinct transports rejected", er)
	}
}
func TestJSONCReadOnlyImport(t *testing.T) {
	binary := os.Getenv("SING_BOX_TEST_BINARY")
	if binary == "" {
		t.Skip("requires installed binary")
	}
	d := t.TempDir()
	p := filepath.Join(d, "config.json")
	raw := []byte("{ // retain this comment\n\"outbounds\":[{\"type\":\"direct\",\"tag\":\"direct\"}]}\n")
	os.WriteFile(p, raw, 0600)
	e, _ := New(Options{StateDir: filepath.Join(d, "state"), Config: p, Binary: binary, Local: true})
	snap, er := e.Snapshot()
	if er != nil {
		t.Fatal(er)
	}
	if !snap.ReadOnly || !json.Valid(snap.Config) || snap.Hash != hash(raw) {
		t.Fatalf("bad normalized view: %+v", snap)
	}
	if er = e.Save(Draft{Config: snap.Config, BaseHash: snap.Hash}); er == nil {
		t.Fatal("comment-destructive save accepted")
	}
	b, _ := os.ReadFile(p)
	if string(b) != string(raw) {
		t.Fatal("import changed source")
	}
}
func TestHistoryISODate(t *testing.T) {
	d := t.TempDir()
	e, _ := New(Options{StateDir: d, Local: true})
	os.WriteFile(filepath.Join(d, "history", "20261106T120000.000000001.json"), []byte(`{}`), 0600)
	h, er := e.History()
	if er != nil || len(h) != 1 || h[0].Created != "2026-11-06T12:00:00.000000001Z" {
		t.Fatalf("bad history %+v %v", h, er)
	}
}
func TestJSONCInspectionStringSafety(t *testing.T) {
	raw := []byte(`{/* comment */"url":"https://example.com/a/*b*/","quote":"\"//keep",//comment
"list":[1,2,],}`)
	b, er := inspectionJSON(raw)
	if er != nil {
		t.Fatal(er)
	}
	var c map[string]any
	json.Unmarshal(b, &c)
	if c["url"] != "https://example.com/a/*b*/" || c["quote"] != `"//keep` {
		t.Fatal("string changed", string(b))
	}
}

func TestRuleSetAliases(t *testing.T) {
	valid := []byte(`{"inbounds":[{"type":"mixed","tag":"entry"}],"outbounds":[{"type":"direct","tag":"direct"},{"type":"selector","tag":"pick","outbounds":["direct"]}],"route":{"rule_set":[{"type":"local","tag":["sites","alias"],"path":"/rules/{tag}.json"}],"rules":[{"inbound":["entry"],"rule_set":["sites","alias"],"action":"route","outbound":"pick"}]}}`)
	if er := ValidateJSON(valid); er != nil {
		t.Fatal(er)
	}
	for _, raw := range []string{`{"route":{"rule_set":[{"type":"local","path":"/rules/{tag}.json","tag":["sites","sites"]}]}}`, `{"route":{"rule_set":[{"type":"local","path":"/rules/{tag}.json","tag":["sites","alias"]},{"tag":"alias"}]}}`, `{"route":{"rule_set":[{"tag":[]}]}}`, `{"route":{"rule_set":[{"tag":["sites",1]}]}}`, `{"route":{"rule_set":[{"type":"local","path":"/rules/{tag}.json","tag":["sites","alias"]}],"rules":[{"rule_set":["alias","missing"]}]}}`} {
		if ValidateJSON([]byte(raw)) == nil {
			t.Fatalf("accepted invalid aliases/references %s", raw)
		}
	}
	binary := os.Getenv("SING_BOX_TEST_BINARY")
	if binary == "" {
		return
	}
	d := t.TempDir()
	p := filepath.Join(d, "config.json")
	os.WriteFile(p, []byte(`{}`), 0600)
	e, _ := New(Options{StateDir: filepath.Join(d, "state"), Config: p, Binary: binary, Local: true})
	native := []byte(`{"outbounds":[{"type":"direct","tag":"direct"}],"route":{"rule_set":[{"type":"local","tag":["sites","alias"],"path":"/rules/{tag}.json"}],"rules":[{"rule_set":["sites","alias"],"action":"route","outbound":"direct"}]}}`)
	var nativeConfig map[string]any
	json.Unmarshal(native, &nativeConfig)
	rule := nativeConfig["route"].(map[string]any)["rule_set"].([]any)[0].(map[string]any)
	rule["path"] = filepath.Join(d, "{tag}.json")
	for _, tag := range []string{"sites", "alias"} {
		os.WriteFile(filepath.Join(d, tag+".json"), []byte(`{"version":3,"rules":[{"domain_suffix":["example.com"]}]}`), 0600)
	}
	native, _ = json.Marshal(nativeConfig)
	if er := e.validate(native); er != nil {
		t.Fatal("actual installed checker rejected multiple rule-set tags", er)
	}
}
func TestRedactANSIAndControls(t *testing.T) {
	s := "\x1b[31mFATAL\x1b[0m token=secretvalue\x00\x1b]8;;https://example.com\x07link\x1b]8;;\x07\n"
	got := Redact(s)
	if strings.ContainsAny(got, "\x1b\x00\x07") || strings.Contains(got, "secretvalue") || !strings.Contains(got, "FATAL") {
		t.Fatalf("unsafe rendered output %q", got)
	}
}
