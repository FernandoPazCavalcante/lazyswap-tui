// Package applog is a tiny file-based logger.
//
// Mirrors src/common/logger.ts. Writes to ~/.lazyswap/lazyswap.log by default
// (configurable via SetPath, used in tests). Log calls never panic — write
// failures are swallowed so logging cannot crash the TUI.
//
// Levels: INFO, WARN, ERROR, TRACE. ERROR appends the error (and Go runtime
// stack) when provided; TRACE always captures a stack.
package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/FernandoPazCavalcante/lazyswap-tui/internal/paths"
)

type level string

const (
	levelInfo  level = "INFO"
	levelWarn  level = "WARN"
	levelError level = "ERROR"
	levelTrace level = "TRACE"
)

var (
	mu      sync.Mutex
	logPath string
	resolved bool
)

// SetPath overrides the log destination. Pass "/dev/null" to disable writes
// (test default). Must be called before the first write to take effect on
// the cached path.
func SetPath(p string) {
	mu.Lock()
	defer mu.Unlock()
	logPath = p
	resolved = true
}

// Path returns the resolved log file path.
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return resolveLocked()
}

func resolveLocked() string {
	if resolved {
		return logPath
	}
	if os.Getenv("LAZYSWAP_TEST") != "" {
		logPath = "/dev/null"
	} else {
		logPath = filepath.Join(paths.DataDir(), "lazyswap.log")
	}
	resolved = true
	return logPath
}

func write(lv level, msg, extra string) {
	mu.Lock()
	path := resolveLocked()
	mu.Unlock()

	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	line := fmt.Sprintf("[%s] [%-5s] %s\n", timestamp, lv, msg)
	if extra != "" {
		indented := "  " + strings.ReplaceAll(strings.TrimRight(extra, "\n"), "\n", "\n  ")
		line += indented + "\n"
	}

	if path != "/dev/null" {
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

// Info logs an informational message.
func Info(msg string) { write(levelInfo, msg, "") }

// Infof is the printf-style variant of Info.
func Infof(format string, args ...any) { Info(fmt.Sprintf(format, args...)) }

// Warn logs a recoverable issue.
func Warn(msg string) { write(levelWarn, msg, "") }

// Warnf is the printf-style variant of Warn.
func Warnf(format string, args ...any) { Warn(fmt.Sprintf(format, args...)) }

// Error logs an error. If err is non-nil, its message + a captured Go stack
// trace are appended on indented lines.
func Error(msg string, err error) {
	extra := ""
	if err != nil {
		extra = err.Error() + "\n" + captureStack()
	}
	write(levelError, msg, extra)
}

// Errorf logs a printf-style message with no attached error.
func Errorf(format string, args ...any) { write(levelError, fmt.Sprintf(format, args...), "") }

// Trace logs a message together with the current call stack. Use at the entry
// point of sensitive operations (wallet creation, swap execution).
func Trace(msg string) { write(levelTrace, msg, captureStack()) }

// Tracef is the printf-style variant of Trace.
func Tracef(format string, args ...any) { Trace(fmt.Sprintf(format, args...)) }

// captureStack returns a runtime stack trace, dropping frames inside this
// package so the output points at the caller.
func captureStack() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	raw := string(buf[:n])

	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(line, "internal/applog") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
