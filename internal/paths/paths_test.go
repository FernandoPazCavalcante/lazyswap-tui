package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOverrideAndEnsure(t *testing.T) {
	tmp := t.TempDir()
	Override(filepath.Join(tmp, ".lazyswap"))

	dir, err := EnsureDataDir()
	if err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}
	if dir != filepath.Join(tmp, ".lazyswap") {
		t.Fatalf("dir = %q, want %q", dir, filepath.Join(tmp, ".lazyswap"))
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("dir not created: %v", err)
	}
}

func TestEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LAZYSWAP_DATA_DIR", tmp)
	Override("") // clear cache so env is re-read

	dir := DataDir()
	if dir != tmp {
		t.Fatalf("dir = %q, want %q", dir, tmp)
	}
}

func TestDefaultIsHomeLazyswap(t *testing.T) {
	t.Setenv("LAZYSWAP_DATA_DIR", "")
	Override("")

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".lazyswap")
	if got := DataDir(); got != want {
		t.Fatalf("dir = %q, want %q", got, want)
	}
}
