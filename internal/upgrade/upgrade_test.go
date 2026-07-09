package upgrade

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindExtractedBinaryFindsArchiveRootBinary(t *testing.T) {
	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "yokai-tui")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	got, err := findExtractedBinary(tempDir, "yokai-tui")
	if err != nil {
		t.Fatalf("find binary: %v", err)
	}
	if got != binaryPath {
		t.Fatalf("got %q, want %q", got, binaryPath)
	}
}

func TestFindExtractedBinaryFindsNestedBinary(t *testing.T) {
	tempDir := t.TempDir()
	nestedDir := filepath.Join(tempDir, "Yokai_0.1.0_darwin_arm64")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("mkdir nested dir: %v", err)
	}
	binaryPath := filepath.Join(nestedDir, "yokai-tui")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	got, err := findExtractedBinary(tempDir, "yokai-tui")
	if err != nil {
		t.Fatalf("find binary: %v", err)
	}
	if got != binaryPath {
		t.Fatalf("got %q, want %q", got, binaryPath)
	}
}

func TestFindExtractedBinaryRequiresRegularFile(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tempDir, "yokai-tui"), 0755); err != nil {
		t.Fatalf("mkdir binary-name dir: %v", err)
	}

	if _, err := findExtractedBinary(tempDir, "yokai-tui"); err == nil {
		t.Fatal("expected missing regular file error")
	}
}
