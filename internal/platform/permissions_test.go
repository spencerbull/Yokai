package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestChmodIfSupported(t *testing.T) {
	// Create a temp file to chmod.
	dir := t.TempDir()
	f := filepath.Join(dir, "testfile")
	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := ChmodIfSupported(f, 0755)
	if err != nil {
		t.Fatalf("ChmodIfSupported returned error: %v", err)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(f)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		// Mask off type bits; compare permission bits only.
		got := info.Mode().Perm()
		if got != 0755 {
			t.Errorf("expected mode 0755, got %04o", got)
		}
	}
}

func TestChmodIfSupported_NonexistentFile(t *testing.T) {
	err := ChmodIfSupported("/no/such/file", 0755)
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("expected nil on Windows, got %v", err)
		}
	} else {
		if err == nil {
			t.Fatal("expected error for nonexistent file on Unix")
		}
	}
}
