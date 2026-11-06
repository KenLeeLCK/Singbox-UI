package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func journalFixture(t *testing.T, script string) *Engine {
	t.Helper()
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "journalctl"), []byte("#!/bin/sh\n"+script), 0700)
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &Engine{Options: Options{Unit: "sing-box.service"}}
}
func TestJournalFailureRedactsStderr(t *testing.T) {
	e := journalFixture(t, "printf 'token=privatevalue permission denied' >&2\nexit 1\n")
	err := e.Journal(context.Background(), func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "privatevalue") {
		t.Fatal(err)
	}
}
func TestJournalCallbackCancelsChild(t *testing.T) {
	e := journalFixture(t, "echo line\nexec sleep 30\n")
	start := time.Now()
	err := e.Journal(context.Background(), func(string) error { return errors.New("client disconnected") })
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("cancellation failed: %v", err)
	}
}
func TestJournalOversizeCancelsChild(t *testing.T) {
	e := journalFixture(t, "head -c 300000 /dev/zero | tr '\\000' x\nexec sleep 30\n")
	start := time.Now()
	err := e.Journal(context.Background(), func(string) error { return nil })
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("oversize cancellation failed: %v", err)
	}
}
func TestJournalLimitsAndLocal(t *testing.T) {
	e := &Engine{Options: Options{Local: true}}
	if err := e.Journal(context.Background(), func(string) error { return nil }); err == nil || !strings.Contains(err.Error(), "local mode") {
		t.Fatal(err)
	}
	e.Local = false
	for i := 0; i < cap(journalSlots); i++ {
		journalSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(journalSlots); i++ {
			<-journalSlots
		}
	}()
	if err := e.Journal(context.Background(), func(string) error { return nil }); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatal(err)
	}
}
