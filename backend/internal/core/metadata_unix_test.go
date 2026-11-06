//go:build linux || darwin

package core

import (
	"bytes"
	"encoding/json"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestExtendedMetadataRoundTrip(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "config.json")
	os.WriteFile(p, []byte(`{}`), 0640)
	if err := unix.Setxattr(p, "user.manager-test", []byte("kept"), 0); err != nil {
		t.Skipf("xattr filesystem unavailable: %v", err)
	}
	attrs, err := readMetadata(p)
	if err != nil {
		t.Fatal(err)
	}
	tx := transaction{Xattrs: attrs}
	encoded, _ := json.Marshal(tx)
	var restored transaction
	json.Unmarshal(encoded, &restored)
	if err = atomicWithMetadata(p, []byte(`{"log":{}}`), 0640, -1, -1, restored.Xattrs); err != nil {
		t.Fatal(err)
	}
	after, err := readMetadata(p)
	if err != nil || !bytes.Equal(after["user.manager-test"], []byte("kept")) {
		t.Fatalf("metadata lost %+v %v", after, err)
	}
}
func TestMetadataFailureNeverReplacesLive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config")
	os.WriteFile(p, []byte("original"), 0600)
	err := atomicWithMetadata(p, []byte("new"), 0600, -1, -1, map[string][]byte{"bad\x00attribute": []byte("x")})
	if err == nil {
		t.Fatal("invalid metadata accepted")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "original" {
		t.Fatal("replacement happened before metadata success")
	}
}
