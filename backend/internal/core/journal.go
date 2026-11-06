package core

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var secretPattern = regexp.MustCompile(`(?i)(password|passwd|secret|token|authorization|uuid|private_key)(["' ]*[:=]["' ]*|\s+Bearer\s+)([^\s,;}]+)`)
var bearerPattern = regexp.MustCompile(`(?i)(bearer\s+)[^\s,;}]+`)
var ansiPattern = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[@-_])`)
var urlPattern = regexp.MustCompile(`(https?://)[^/@\s]+:[^/@\s]+@`)

func Redact(s string) string {
	s = ansiPattern.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		if (r < 32 && r != '\n' && r != '\t') || (r >= 127 && r <= 159) {
			return -1
		}
		return r
	}, s)
	return urlPattern.ReplaceAllString(secretPattern.ReplaceAllString(bearerPattern.ReplaceAllString(s, "$1[REDACTED]"), "$1$2[REDACTED]"), "$1[REDACTED]@")
}

var journalSlots = make(chan struct{}, 8)

type boundedStderr struct{ data []byte }

func (b *boundedStderr) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 8192 - len(b.data)
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

func (e *Engine) Journal(ctx context.Context, emit func(string) error) error {
	if e.Local {
		return fmt.Errorf("live systemd logs unavailable in local mode")
	}
	select {
	case journalSlots <- struct{}{}:
		defer func() { <-journalSlots }()
	default:
		return fmt.Errorf("live log stream limit reached (8); close another log window")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c := exec.CommandContext(ctx, "journalctl", "-u", e.Unit, "-f", "-n", "200", "--output=short-iso", "--no-pager")
	c.WaitDelay = time.Second
	var stderr boundedStderr
	c.Stderr = &stderr
	p, er := c.StdoutPipe()
	if er != nil {
		return er
	}
	if er = c.Start(); er != nil {
		return er
	}
	scanner := bufio.NewScanner(p)
	scanner.Buffer(make([]byte, 4096), 256*1024)
	var streamErr error
	for scanner.Scan() {
		if er = emit(Redact(strings.TrimSpace(scanner.Text()))); er != nil {
			streamErr = er
			cancel()
			break
		}
	}
	if streamErr == nil {
		streamErr = scanner.Err()
	}
	if streamErr != nil {
		cancel()
	}
	waitErr := c.Wait()
	if streamErr != nil {
		return streamErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		return fmt.Errorf("journalctl failed: %v: %s", waitErr, Redact(string(stderr.data)))
	}
	return nil
}
