package applog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTmp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "test.log")
	SetPath(p)
	return p
}

func readAll(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	return string(b)
}

func TestInfo(t *testing.T) {
	p := setupTmp(t)
	Info("hello world")
	s := readAll(t, p)
	if !strings.Contains(s, "[INFO ]") || !strings.Contains(s, "hello world") {
		t.Errorf("unexpected log:\n%s", s)
	}
}

func TestWarnInfof(t *testing.T) {
	p := setupTmp(t)
	Warnf("count=%d", 7)
	s := readAll(t, p)
	if !strings.Contains(s, "[WARN ]") || !strings.Contains(s, "count=7") {
		t.Errorf("unexpected log:\n%s", s)
	}
}

func TestErrorWithErr(t *testing.T) {
	p := setupTmp(t)
	Error("boom", errors.New("inner"))
	s := readAll(t, p)
	if !strings.Contains(s, "[ERROR]") || !strings.Contains(s, "boom") || !strings.Contains(s, "inner") {
		t.Errorf("unexpected log:\n%s", s)
	}
}

func TestTraceCapturesStack(t *testing.T) {
	p := setupTmp(t)
	Trace("entry")
	s := readAll(t, p)
	if !strings.Contains(s, "[TRACE]") || !strings.Contains(s, "entry") {
		t.Errorf("unexpected log:\n%s", s)
	}
	if !strings.Contains(s, "goroutine") {
		t.Errorf("expected stack in trace output, got:\n%s", s)
	}
	// Stack must not leak applog frames.
	if strings.Contains(s, "internal/applog") {
		t.Errorf("applog frames leaked into trace:\n%s", s)
	}
}

func TestDevNullSilent(t *testing.T) {
	SetPath("/dev/null")
	// Should not panic and should produce no readable artifact.
	Info("nothing to see here")
}
