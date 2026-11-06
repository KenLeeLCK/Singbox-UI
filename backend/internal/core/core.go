package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sing-box-manager/sing-box-manager/backend/internal/telemetry"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Options struct {
	StateDir         string
	Config           string
	Binary           string
	Unit             string
	WorkingDir       string
	Sources          []string
	ReadOnly         bool
	Local            bool
	configDirectory  bool
	discoveredConfig string
	discoveryID      string
	discovered       bool
}
type Engine struct {
	Options
	mu sync.Mutex
}
type Snapshot struct {
	Config   json.RawMessage `json:"config"`
	Hash     string          `json:"hash"`
	Sources  []string        `json:"sources"`
	ReadOnly bool            `json:"readOnly"`
	Draft    *Draft          `json:"draft,omitempty"`
}
type Draft struct {
	Config   json.RawMessage `json:"config"`
	BaseHash string          `json:"baseHash"`
}
type History struct {
	ID      string `json:"id"`
	Created string `json:"created"`
}
type transaction struct {
	Path   string
	Backup string
	Mode   uint32
	UID    int
	GID    int
	Phase  string
	Xattrs map[string][]byte
}

func Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, name, args...)
	c.Dir = dir
	b, e := c.CombinedOutput()
	if e != nil {
		return nil, fmt.Errorf("%s failed: %s", name, Redact(string(b)))
	}
	return b, nil
}
func hash(b []byte) string                                 { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Atomic(path string, b []byte, mode os.FileMode) error { return atomicOwned(path, b, mode, -1, -1) }
func atomicOwned(path string, b []byte, mode os.FileMode, uid, gid int) error {
	return atomicWithMetadata(path, b, mode, uid, gid, nil)
}
func atomicWithMetadata(path string, b []byte, mode os.FileMode, uid, gid int, attrs map[string][]byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".manager-*")
	if e != nil {
		return e
	}
	n := f.Name()
	defer os.Remove(n)
	if uid >= 0 && os.Geteuid() == 0 {
		if e = f.Chown(uid, gid); e != nil {
			f.Close()
			return e
		}
	}
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = setMetadata(n, attrs)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(n, path)
	}
	if e == nil {
		d, er := os.Open(filepath.Dir(path))
		if er == nil {
			e = d.Sync()
			d.Close()
		}
	}
	return e
}
func New(o Options) (*Engine, error) {
	if !filepath.IsAbs(o.StateDir) {
		p, e := filepath.Abs(o.StateDir)
		if e != nil {
			return nil, e
		}
		o.StateDir = p
	}
	if e := os.MkdirAll(filepath.Join(o.StateDir, "history"), 0700); e != nil {
		return nil, e
	}
	if o.Unit == "" {
		o.Unit = "sing-box.service"
	}
	if o.Binary == "" {
		o.Binary, _ = exec.LookPath("sing-box")
	}
	if o.Config == "" {
		o.discovered = !o.Local
		discover(&o)
	}
	if o.Config != "" {
		o.Config, _ = filepath.Abs(o.Config)
	}
	if len(o.Sources) == 0 && o.Config != "" {
		o.Sources = []string{o.Config}
	}
	o.discoveredConfig = o.Config
	return &Engine{Options: o}, nil
}
func discover(o *Options) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, e := Run(ctx, "", "systemctl", "show", o.Unit, "--property=ExecStart,WorkingDirectory", "--no-pager")
	if e != nil {
		o.ReadOnly = true
		return
	}
	parseDiscovery(o, string(b))
}
func parseDiscovery(o *Options, s string) {
	o.WorkingDir = "/"
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "WorkingDirectory=") {
			value := strings.TrimPrefix(line, "WorkingDirectory=")
			if value != "" {
				if !filepath.IsAbs(value) {
					o.ReadOnly = true
					value = filepath.Join("/", value)
				}
				o.WorkingDir = filepath.Clean(value)
			}
		}
	}
	a := strings.Index(s, "argv[]=")
	if a < 0 {
		o.ReadOnly = true
		return
	}
	args := strings.Fields(strings.Split(s[a+7:], ";")[0])
	normalized := []string{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "--config-directory=") || strings.HasPrefix(arg, "--directory=") {
			p := strings.SplitN(arg, "=", 2)
			normalized = append(normalized, p...)
		} else {
			normalized = append(normalized, arg)
		}
	}
	args = normalized
	if strings.Count(s, "argv[]=") != 1 {
		o.ReadOnly = true
	}
	for _, arg := range args {
		if strings.ContainsAny(arg, "\"'\\$") {
			o.ReadOnly = true
		}
	}
	runs, selectors := 0, 0
	serviceWD := o.WorkingDir
	for i, v := range args {
		if (v == "-D" || v == "--directory") && i+1 < len(args) {
			dir := args[i+1]
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(serviceWD, dir)
			}
			o.WorkingDir = filepath.Clean(dir)
		}
	}
	for i := 0; i < len(args); i++ {
		v := args[i]
		if i > 0 && v == "run" {
			runs++
			continue
		}
		if i > 0 && v != "--disable-color" && v != "-c" && v != "--config" && v != "-C" && v != "--config-directory" && v != "-D" && v != "--directory" {
			o.ReadOnly = true
		}
		if v == "-c" || v == "--config" || v == "-C" || v == "--config-directory" || v == "-D" || v == "--directory" {
			if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "-") {
				o.ReadOnly = true
				break
			}
		}
		if (v == "-c" || v == "--config") && i+1 < len(args) {
			i++
			p := args[i]
			selectors++
			if p == "stdin" || filepath.Ext(p) != ".json" {
				o.ReadOnly = true
			}
			if !filepath.IsAbs(p) {
				p = filepath.Join(o.WorkingDir, p)
			}
			o.Sources = append(o.Sources, p)
		}
		if (v == "-D" || v == "--directory") && i+1 < len(args) {
			i++ // effective directory was resolved in the first pass
		}
		if (v == "-C" || v == "--config-directory") && i+1 < len(args) {
			i++
			dir := args[i]
			selectors++
			o.configDirectory = true
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(o.WorkingDir, dir)
			}
			info, err := os.Lstat(dir)
			if err != nil || !info.IsDir() {
				o.ReadOnly = true
				continue
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				o.ReadOnly = true
				continue
			}
			for _, entry := range entries {
				// Match sing-box 1.14: nonrecursive, case-sensitive .json suffix.
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
					continue
				}
				path := filepath.Join(dir, entry.Name())
				o.Sources = append(o.Sources, path)
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					o.ReadOnly = true
				}
			}
		}
	}
	if runs != 1 || selectors != 1 {
		o.ReadOnly = true
	}
	if len(args) > 0 && filepath.IsAbs(args[0]) {
		o.Binary = args[0]
		base := filepath.Base(args[0])
		if base != "sing-box" && !strings.HasPrefix(base, "sing-box-") {
			o.ReadOnly = true
		}
	} else {
		o.ReadOnly = true
	}
	identity, _ := json.Marshal(struct {
		Args   []string
		WD     string
		Binary string
	}{args, o.WorkingDir, o.Binary})
	o.discoveryID = hash(identity)
	if len(o.Sources) == 1 {
		o.Config = o.Sources[0]
	} else {
		o.ReadOnly = true
	}
	if len(o.Sources) == 0 {
		o.ReadOnly = true
	}
}
func (e *Engine) refreshDiscovery() error {
	if !e.discovered {
		return nil
	}
	current := Options{Unit: e.Unit}
	discover(&current)
	if current.discoveryID == "" || current.discoveryID != e.discoveryID || (e.discoveredConfig != "" && current.Config != "" && e.discoveredConfig != current.Config) {
		return errors.New("effective sing-box service configuration changed or cannot be inspected; restart the Manager helper to rediscover before editing")
	}
	if e.discoveredConfig == "" {
		e.discoveredConfig = current.Config
	}
	e.Sources = current.Sources
	e.Config = current.Config
	e.ReadOnly = current.ReadOnly
	return nil
}
func (e *Engine) snapshot() (Snapshot, error) {
	if err := e.refreshDiscovery(); err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{Config: json.RawMessage(`{}`), Sources: e.Sources, ReadOnly: e.ReadOnly}
	for _, path := range e.Sources {
		info, err := os.Stat(path)
		if err != nil {
			return s, err
		}
		if !info.Mode().IsRegular() {
			return s, errors.New("configuration source must be a regular file")
		}
	}
	if e.Config == "" {
		if len(e.Sources) == 0 {
			return s, nil
		}
		// Use the installed merger for its own semantics, never flatten on write.
		dir, err := os.MkdirTemp(e.StateDir, "inspect-")
		if err != nil {
			return s, err
		}
		defer os.RemoveAll(dir)
		target := filepath.Join(dir, "merged.json")
		args := []string{"merge", target}
		var all []byte
		for _, p := range e.Sources {
			args = append(args, "-c", p)
			b, err := os.ReadFile(p)
			if err != nil {
				return s, err
			}
			all = append(all, []byte(p)...)
			all = append(all, b...)
		}
		if e.WorkingDir != "" {
			args = append(args, "-D", e.WorkingDir)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err = Run(ctx, e.WorkingDir, e.Binary, args...); err != nil {
			return s, err
		}
		b, err := os.ReadFile(target)
		if err != nil {
			return s, err
		}
		b, err = inspectionJSON(b)
		if err != nil {
			return s, err
		}
		s.Config = b
		s.Hash = hash(all)
		return s, nil
	}
	b, er := os.ReadFile(e.Config)
	if er != nil {
		return s, er
	}
	if !json.Valid(b) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"format", "-c", e.Config}
		if e.WorkingDir != "" {
			args = append(args, "-D", e.WorkingDir)
		}
		normalized, err := Run(ctx, e.WorkingDir, e.Binary, args...)
		if err != nil {
			return s, err
		}
		normalized, err = inspectionJSON(normalized)
		if err != nil {
			return s, err
		}
		s.Config = normalized
		s.Hash = hash(b)
		s.ReadOnly = true
		return s, nil
	}
	s.Config = b
	s.Hash = hash(b)
	var d Draft
	if b, er = os.ReadFile(filepath.Join(e.StateDir, "draft.json")); er == nil && json.Unmarshal(b, &d) == nil {
		s.Draft = &d
	}
	return s, nil
}
func (e *Engine) Snapshot() (Snapshot, error) { e.mu.Lock(); defer e.mu.Unlock(); return e.snapshot() }
func (e *Engine) Save(d Draft) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, er := e.snapshot()
	if er != nil {
		return er
	}
	if s.ReadOnly {
		return errors.New("configuration sources cannot safely be written; read-only")
	}
	if s.Hash != d.BaseHash {
		return errors.New("live configuration changed; reload and reconcile draft")
	}
	if er = ValidateJSON(d.Config); er != nil {
		return er
	}
	b, _ := json.Marshal(d)
	return Atomic(filepath.Join(e.StateDir, "draft.json"), b, 0600)
}
func ValidateJSON(b []byte) error {
	var c map[string]any
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if er := dec.Decode(&c); er != nil {
		return er
	}
	if c == nil || !json.Valid(b) {
		return errors.New("configuration must be a JSON object")
	}
	tags := map[string]map[string]bool{}
	for _, key := range []string{"inbounds", "outbounds", "endpoints"} {
		tags[key] = map[string]bool{}
		arr, _ := c[key].([]any)
		for _, v := range arr {
			m, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid %s entry", key)
			}
			t, _ := m["tag"].(string)
			if t != "" {
				if tags[key][t] {
					return fmt.Errorf("duplicate %s tag %q", key, t)
				}
				tags[key][t] = true
			}
			if p, ok := m["listen_port"].(json.Number); ok {
				n, er := p.Int64()
				if er != nil || n < 1 || n > 65535 {
					return errors.New("listen_port must be 1–65535")
				}
			}
		}
	}
	for tag := range tags["endpoints"] {
		if tags["outbounds"][tag] {
			return fmt.Errorf("duplicate outbound/endpoint tag %q", tag)
		}
	}
	if er := validateListeners(c); er != nil {
		return er
	}
	route, _ := c["route"].(map[string]any)
	tags["rule_set"] = map[string]bool{}
	sets, _ := route["rule_set"].([]any)
	for _, v := range sets {
		m, _ := v.(map[string]any)
		aliases := []any{m["tag"]}
		if list, ok := m["tag"].([]any); ok {
			aliases = list
		}
		if len(aliases) == 0 {
			return errors.New("rule set tag missing")
		}
		if len(aliases) > 1 {
			typ, _ := m["type"].(string)
			if typ == "" || typ == "inline" {
				return errors.New("inline rule set does not support multiple tags")
			}
			field := ""
			if typ == "local" {
				field = "path"
			}
			if typ == "remote" {
				field = "url"
			}
			if field != "" {
				value, _ := m[field].(string)
				if !strings.Contains(value, "{tag}") {
					return fmt.Errorf("multi-tag rule set requires {tag} placeholder in %s", field)
				}
			}
		}
		for _, alias := range aliases {
			tag, ok := alias.(string)
			if !ok || tag == "" || tags["rule_set"][tag] {
				return errors.New("rule set tag missing, invalid, or duplicated")
			}
			tags["rule_set"][tag] = true
		}
	}
	var walk func(any) error
	walk = func(v any) error {
		switch m := v.(type) {
		case map[string]any:
			for k, v := range m {
				group := ""
				switch k {
				case "outbound", "outbounds", "detour", "final", "default":
					group = "outbounds"
				case "inbound":
					group = "inbounds"
				case "rule_set":
					if _, ok := v.(string); ok {
						group = "rule_set"
					} else if a, ok := v.([]any); ok && len(a) > 0 {
						if _, ok := a[0].(string); ok {
							group = "rule_set"
						}
					}
				}
				if group != "" {
					vals := []any{v}
					if a, ok := v.([]any); ok {
						vals = a
					}
					for _, q := range vals {
						if t, ok := q.(string); ok && t != "" && !tags[group][t] && !(group == "outbounds" && tags["endpoints"][t]) {
							return fmt.Errorf("unresolved %s reference %q", k, t)
						}
					}
				}
				if er := walk(v); er != nil {
					return er
				}
			}
		case []any:
			for _, v := range m {
				if er := walk(v); er != nil {
					return er
				}
			}
		}
		return nil
	}
	if er := walk(route); er != nil {
		return er
	}
	return walk(c["outbounds"])
}
func (e *Engine) validate(b []byte) error {
	if er := ValidateJSON(b); er != nil {
		return er
	}
	if e.Binary == "" {
		return errors.New("sing-box binary unavailable")
	}
	// Never put an active .json candidate in the live configuration directory.
	dir, er := os.MkdirTemp(e.StateDir, "check-")
	if er != nil {
		return er
	}
	defer os.RemoveAll(dir)
	f, er := os.CreateTemp(dir, "candidate-*.json")
	if er != nil {
		return er
	}
	name := f.Name()
	defer os.Remove(name)
	_, er = f.Write(b)
	closeErr := f.Close()
	if er == nil {
		er = closeErr
	}
	if er != nil {
		return er
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"check", "-c", name}
	if e.configDirectory {
		args = []string{"check", "-C", dir}
	}
	if e.WorkingDir != "" {
		args = append(args, "-D", e.WorkingDir)
	}
	_, er = Run(ctx, e.WorkingDir, e.Binary, args...)
	return er
}
func (e *Engine) Validate() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, er := e.snapshot()
	if er != nil {
		return er
	}
	if s.Draft == nil {
		return errors.New("save a draft first")
	}
	return e.validate(s.Draft.Config)
}
func (e *Engine) restart() error {
	if e.Local {
		return errors.New("service deployment unavailable in local mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if _, er := Run(ctx, "", "systemctl", "restart", e.Unit); er != nil {
		return er
	}
	time.Sleep(3 * time.Second)
	_, er := Run(ctx, "", "systemctl", "is-active", "--quiet", e.Unit)
	if er != nil {
		return er
	}
	return e.health(ctx)
}
func (e *Engine) Recover() error { e.mu.Lock(); defer e.mu.Unlock(); return e.recover() }
func (e *Engine) recover() error {
	p := filepath.Join(e.StateDir, "transaction.json")
	b, er := os.ReadFile(p)
	if os.IsNotExist(er) {
		return nil
	}
	if er != nil {
		return er
	}
	var t transaction
	if er = json.Unmarshal(b, &t); er != nil {
		return er
	}
	if t.Path != e.Config {
		return errors.New("transaction configuration path does not match")
	}
	b, er = os.ReadFile(t.Backup)
	if er != nil {
		return er
	}
	if er = atomicWithMetadata(t.Path, b, os.FileMode(t.Mode), t.UID, t.GID, t.Xattrs); er != nil {
		return er
	}
	if !e.Local {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, resetErr := Run(ctx, "", "systemctl", "reset-failed", e.Unit)
		cancel()
		if resetErr != nil {
			return fmt.Errorf("rollback reset failed: %w", resetErr)
		}
	}
	if er = e.restart(); er != nil {
		return fmt.Errorf("rollback restored file but service recovery failed: %w", er)
	}
	return os.Remove(p)
}
func (e *Engine) Apply() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Local {
		return errors.New("local mode is inspect/validate only; deployment requires systemd helper")
	}
	if _, er := os.Stat(filepath.Join(e.StateDir, "transaction.json")); er == nil {
		return errors.New("unresolved transaction; restart helper for recovery")
	}
	s, er := e.snapshot()
	if er != nil {
		return er
	}
	if s.ReadOnly || s.Draft == nil {
		return errors.New("writable saved draft required")
	}
	if s.Hash != s.Draft.BaseHash {
		return errors.New("live configuration changed")
	}
	if er = e.validate(s.Draft.Config); er != nil {
		return er
	}
	if er = e.refreshDiscovery(); er != nil {
		return er
	}
	if e.ReadOnly || len(e.Sources) != 1 || len(s.Sources) != 1 || e.Sources[0] != s.Sources[0] {
		return errors.New("effective configuration sources changed during validation")
	}
	before, er := os.ReadFile(e.Config)
	if er != nil {
		return er
	}
	if hash(before) != s.Hash {
		return errors.New("live configuration changed during validation")
	}
	info, er := os.Lstat(e.Config)
	if er != nil {
		return er
	}
	if !info.Mode().IsRegular() {
		return errors.New("configuration must be regular file")
	}
	attrs, er := readMetadata(e.Config)
	if er != nil {
		return er
	}
	id := time.Now().UTC().Format("20060102T150405.000000000")
	backup := filepath.Join(e.StateDir, "history", id+".json")
	if er = Atomic(backup, before, 0600); er != nil {
		return er
	}
	t := transaction{Path: e.Config, Backup: backup, Mode: uint32(info.Mode().Perm()), Phase: "prepared", Xattrs: attrs}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		t.UID = int(st.Uid)
		t.GID = int(st.Gid)
	}
	b, _ := json.Marshal(t)
	if er = Atomic(filepath.Join(e.StateDir, "transaction.json"), b, 0600); er != nil {
		return er
	}
	er = atomicWithMetadata(e.Config, s.Draft.Config, info.Mode().Perm(), t.UID, t.GID, t.Xattrs)
	if er == nil {
		er = e.restart()
	}
	if er != nil {
		re := e.recover()
		return fmt.Errorf("deployment failed: %v; rollback: %v", er, re)
	}
	if er = os.Remove(filepath.Join(e.StateDir, "transaction.json")); er != nil {
		return er
	}
	return os.Remove(filepath.Join(e.StateDir, "draft.json"))
}
func (e *Engine) History() ([]History, error) {
	files, er := filepath.Glob(filepath.Join(e.StateDir, "history", "*.json"))
	if er != nil {
		return nil, er
	}
	h := []History{}
	for _, p := range files {
		id := strings.TrimSuffix(filepath.Base(p), ".json")
		created, err := time.Parse("20060102T150405.000000000", id)
		if err != nil {
			continue
		}
		h = append(h, History{id, created.UTC().Format(time.RFC3339Nano)})
	}
	return h, nil
}
func (e *Engine) Restore(id string) error {
	if !regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}$`).MatchString(id) {
		return errors.New("invalid history ID")
	}
	b, er := os.ReadFile(filepath.Join(e.StateDir, "history", id+".json"))
	if er != nil {
		return er
	}
	s, er := e.Snapshot()
	if er != nil {
		return er
	}
	return e.Save(Draft{b, s.Hash})
}
func (e *Engine) Schema() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return Run(ctx, e.WorkingDir, e.Binary, "schema")
}
func (e *Engine) Status() map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	v, ve := Run(ctx, "", e.Binary, "version")
	state := "unavailable"
	var uptime string
	if !e.Local {
		b, er := Run(ctx, "", "systemctl", "is-active", e.Unit)
		if er == nil {
			state = strings.TrimSpace(string(b))
		}
		b, _ = Run(ctx, "", "systemctl", "show", e.Unit, "--property=ActiveEnterTimestamp", "--value")
		uptime = strings.TrimSpace(string(b))
	}
	return map[string]any{"version": strings.TrimSpace(string(v)), "installed": ve == nil, "state": state, "running": state == "active", "unit": e.Unit, "uptime": uptime, "readOnly": e.ReadOnly, "local": e.Local, "monitoring": "unavailable"}
}

// Health checks inspect only known TCP listeners and configured native API.
// They verify readiness, not end-to-end proxy correctness.
func (e *Engine) health(ctx context.Context) error {
	b, er := os.ReadFile(e.Config)
	if er != nil {
		return er
	}
	var c map[string]any
	if er = json.Unmarshal(b, &c); er != nil {
		return er
	}
	arr, _ := c["inbounds"].([]any)
	tcp := map[string]bool{"mixed": true, "socks": true, "http": true, "shadowsocks": true, "vless": true, "trojan": true, "anytls": true, "vmess": true, "naive": true}
	for _, v := range arr {
		m, _ := v.(map[string]any)
		typ, _ := m["type"].(string)
		if !tcp[typ] {
			continue
		}
		if n, ok := m["network"].(string); ok && n == "udp" {
			continue
		}
		p, ok := m["listen_port"].(float64)
		if !ok || p == 0 {
			continue
		}
		host, _ := m["listen"].(string)
		if host == "" || host == "0.0.0.0" {
			host = "127.0.0.1"
		}
		if host == "::" {
			host = "::1"
		}
		addr := net.JoinHostPort(host, strconv.Itoa(int(p)))
		conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("listener %s failed readiness check: %w", addr, err)
		}
		conn.Close()
	}
	return telemetry.Check(ctx, c)
}

func validateListeners(c map[string]any) error {
	seen := map[string]bool{}
	for _, section := range []string{"inbounds", "services"} {
		arr, _ := c[section].([]any)
		for _, v := range arr {
			m, _ := v.(map[string]any)
			p, ok := m["listen_port"].(json.Number)
			if !ok {
				continue
			}
			host, _ := m["listen"].(string)
			if host == "" {
				host = "::"
			}
			if ip := net.ParseIP(host); ip != nil {
				host = ip.String()
			}
			typ, _ := m["type"].(string)
			protocols := []string{}
			switch typ {
			case "hysteria", "hysteria2", "tuic":
				protocols = []string{"udp"}
			case "http", "trojan", "vless", "vmess", "anytls", "naive", "api", "ssm-api":
				protocols = []string{"tcp"}
			case "mixed", "socks", "shadowsocks":
				protocols = []string{"tcp", "udp"}
			default:
				continue
			}
			if network, ok := m["network"].(string); ok && (network == "tcp" || network == "udp") {
				protocols = []string{network}
			}
			for _, protocol := range protocols {
				k := net.JoinHostPort(host, p.String()) + "/" + protocol
				if seen[k] {
					return fmt.Errorf("duplicate listener %s", k)
				}
				seen[k] = true
			}
		}
	}
	return nil
}
