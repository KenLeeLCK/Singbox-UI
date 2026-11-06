//go:build linux

package core

import (
	"bytes"
	"encoding/binary"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxPOSIXACLPreservation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{}`), 0640)
	acl := new(bytes.Buffer)
	binary.Write(acl, binary.LittleEndian, uint32(2))
	for _, entry := range []struct {
		tag, perm uint16
		id        uint32
	}{{1, 6, 0xffffffff}, {2, 4, 1234}, {4, 0, 0xffffffff}, {16, 4, 0xffffffff}, {32, 0, 0xffffffff}} {
		binary.Write(acl, binary.LittleEndian, entry.tag)
		binary.Write(acl, binary.LittleEndian, entry.perm)
		binary.Write(acl, binary.LittleEndian, entry.id)
	}
	if er := unix.Setxattr(p, "system.posix_acl_access", acl.Bytes(), 0); er != nil {
		t.Skipf("POSIX ACL unavailable: %v", er)
	}
	attrs, er := readMetadata(p)
	if er != nil {
		t.Fatal(er)
	}
	if er = atomicWithMetadata(p, []byte(`{"log":{}}`), 0640, -1, -1, attrs); er != nil {
		t.Fatal(er)
	}
	after, er := readMetadata(p)
	if er != nil || !bytes.Equal(attrs["system.posix_acl_access"], after["system.posix_acl_access"]) {
		t.Fatalf("ACL changed %+v %v", after, er)
	}
}
