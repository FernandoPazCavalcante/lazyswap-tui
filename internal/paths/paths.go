// Package paths is the single source of truth for filesystem locations.
//
// Default data dir = ~/.lazyswap/ (wallets.db, lazyswap.log).
// Override via LAZYSWAP_DATA_DIR env var (used by tests and custom layouts).
package paths

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	mu      sync.Mutex
	dataDir string
	ensured bool
)

// DataDir returns the resolved data directory without creating it.
func DataDir() string {
	mu.Lock()
	defer mu.Unlock()
	return resolveLocked()
}

// EnsureDataDir guarantees the data directory exists and returns its path.
// Idempotent: subsequent calls skip the mkdir.
func EnsureDataDir() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	dir := resolveLocked()
	if ensured {
		return dir, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	ensured = true
	return dir, nil
}

// Override sets the data dir explicitly (for tests). Resets the ensured flag.
func Override(dir string) {
	mu.Lock()
	defer mu.Unlock()
	dataDir = dir
	ensured = false
}

func resolveLocked() string {
	if dataDir != "" {
		return dataDir
	}
	if env := os.Getenv("LAZYSWAP_DATA_DIR"); env != "" {
		dataDir = env
		return dataDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Last-resort fallback — should never happen on Linux/macOS.
		home = "."
	}
	dataDir = filepath.Join(home, ".lazyswap")
	return dataDir
}
