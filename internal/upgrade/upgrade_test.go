package upgrade

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateArchivePatternPreservesFormat(t *testing.T) {
	for _, ext := range []string{".tar.gz", ".zip"} {
		if pattern := updateArchivePattern(ext); !strings.HasSuffix(pattern, ext) {
			t.Fatalf("pattern %q does not preserve extension %q", pattern, ext)
		}
	}
}

func TestExtractArchiveUsesZipPath(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "update.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(archive)
	entry, err := writer.Create("yokai.exe")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := entry.Write([]byte("windows-binary")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	destination := t.TempDir()
	if err := extractArchive(archivePath, destination); err != nil {
		t.Fatalf("extract zip: %v", err)
	}
	assertFileContents(t, filepath.Join(destination, "yokai.exe"), "windows-binary")
}

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

func TestInstallUpdatePairReplacesBothBinaries(t *testing.T) {
	installDir := t.TempDir()
	currentBinaryPath := filepath.Join(installDir, "yokai")
	currentTUIBinaryPath := filepath.Join(installDir, companionBinaryName())
	writeTestFile(t, currentBinaryPath, "old-yokai")
	writeTestFile(t, currentTUIBinaryPath, "old-tui")

	newDir := t.TempDir()
	newBinaryPath := filepath.Join(newDir, "yokai")
	newTUIBinaryPath := filepath.Join(newDir, companionBinaryName())
	writeTestFile(t, newBinaryPath, "new-yokai")
	writeTestFile(t, newTUIBinaryPath, "new-tui")

	if err := installUpdatePair(newBinaryPath, newTUIBinaryPath, currentBinaryPath); err != nil {
		t.Fatalf("install update pair: %v", err)
	}
	assertFileContents(t, currentBinaryPath, "new-yokai")
	assertFileContents(t, currentTUIBinaryPath, "new-tui")
}

func TestInstallUpdatePairRollsBackBothBinariesOnRenameFailure(t *testing.T) {
	for _, failAtCall := range []int{3, 4} {
		t.Run(fmt.Sprintf("rename_%d", failAtCall), func(t *testing.T) {
			installDir := t.TempDir()
			currentBinaryPath := filepath.Join(installDir, "yokai")
			currentTUIBinaryPath := filepath.Join(installDir, companionBinaryName())
			writeTestFile(t, currentBinaryPath, "old-yokai")
			writeTestFile(t, currentTUIBinaryPath, "old-tui")

			newDir := t.TempDir()
			newBinaryPath := filepath.Join(newDir, "yokai")
			newTUIBinaryPath := filepath.Join(newDir, companionBinaryName())
			writeTestFile(t, newBinaryPath, "new-yokai")
			writeTestFile(t, newTUIBinaryPath, "new-tui")

			renameCalls := 0
			ops := defaultUpdateFileOps
			ops.rename = func(oldPath, newPath string) error {
				renameCalls++
				if renameCalls == failAtCall {
					return errors.New("injected rename failure")
				}
				return os.Rename(oldPath, newPath)
			}

			if err := installUpdatePairWithOps(newBinaryPath, newTUIBinaryPath, currentBinaryPath, ops); err == nil {
				t.Fatal("expected install failure")
			}
			assertFileContents(t, currentBinaryPath, "old-yokai")
			assertFileContents(t, currentTUIBinaryPath, "old-tui")
		})
	}
}

func TestInstallUpdatePairLeavesActivePairUntouchedOnChmodFailure(t *testing.T) {
	for _, failAtCall := range []int{1, 2} {
		t.Run(fmt.Sprintf("chmod_%d", failAtCall), func(t *testing.T) {
			installDir := t.TempDir()
			currentBinaryPath := filepath.Join(installDir, "yokai")
			currentTUIBinaryPath := filepath.Join(installDir, companionBinaryName())
			writeTestFile(t, currentBinaryPath, "old-yokai")
			writeTestFile(t, currentTUIBinaryPath, "old-tui")

			newDir := t.TempDir()
			newBinaryPath := filepath.Join(newDir, "yokai")
			newTUIBinaryPath := filepath.Join(newDir, companionBinaryName())
			writeTestFile(t, newBinaryPath, "new-yokai")
			writeTestFile(t, newTUIBinaryPath, "new-tui")

			chmodCalls := 0
			ops := defaultUpdateFileOps
			ops.chmod = func(path string, mode os.FileMode) error {
				chmodCalls++
				if chmodCalls == failAtCall {
					return errors.New("injected chmod failure")
				}
				return os.Chmod(path, mode)
			}

			if err := installUpdatePairWithOps(newBinaryPath, newTUIBinaryPath, currentBinaryPath, ops); err == nil {
				t.Fatal("expected staging failure")
			}
			assertFileContents(t, currentBinaryPath, "old-yokai")
			assertFileContents(t, currentTUIBinaryPath, "old-tui")
		})
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(contents) != want {
		t.Fatalf("%s contains %q, want %q", path, contents, want)
	}
}
