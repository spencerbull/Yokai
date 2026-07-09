package upgrade

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spencerbull/yokai/internal/platform"
)

const (
	latestReleaseURL = "https://github.com/spencerbull/yokai/releases/latest"
	repoBaseURL      = "https://github.com/spencerbull/yokai/releases/download"
	projectName      = "Yokai"
	mainBinary       = "yokai"
	tuiBinary        = "yokai-tui"
)

// Release represents the latest release metadata needed for upgrades.
type Release struct {
	TagName string
}

// Check checks for a newer version on GitHub.
func Check(currentVersion string) (*Release, bool, error) {
	resp, err := http.Get(latestReleaseURL)
	if err != nil {
		return nil, false, fmt.Errorf("failed to fetch release info: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() // Best-effort close of release API response body.
	}()

	if resp.Request == nil || resp.Request.URL == nil {
		return nil, false, fmt.Errorf("failed to resolve latest release URL")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("GitHub returned status %d", resp.StatusCode)
	}

	latestTag := strings.TrimPrefix(filepath.Base(resp.Request.URL.Path), "v")
	if latestTag == "" || latestTag == "latest" {
		return nil, false, fmt.Errorf("failed to parse latest release version from %q", resp.Request.URL.String())
	}

	release := Release{TagName: "v" + latestTag}

	// Compare tag_name with currentVersion
	// Remove 'v' prefix if present for comparison
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersionClean := strings.TrimPrefix(currentVersion, "v")

	// Simple version comparison - if they're different and current is "dev" or latest != current
	updateAvailable := currentVersionClean == "dev" || (latestVersion != currentVersionClean && latestVersion != "")

	return &release, updateAvailable, nil
}

// Run downloads and replaces the current binary with the latest version.
func Run(currentVersion string) error {
	fmt.Println("Checking for updates...")

	// 1. Check for update
	release, updateAvailable, err := Check(currentVersion)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	if !updateAvailable {
		fmt.Println("You're already running the latest version!")
		return nil
	}

	fmt.Printf("Update available: %s -> %s\n", currentVersion, release.TagName)

	// 2. Find the correct asset for current OS/arch.
	archiveExt := ".tar.gz"
	if runtime.GOOS == "windows" {
		archiveExt = ".zip"
	}
	expectedAssetName := fmt.Sprintf("%s_%s_%s_%s%s",
		projectName,
		strings.TrimPrefix(release.TagName, "v"),
		runtime.GOOS,
		runtime.GOARCH,
		archiveExt,
	)

	downloadURL := fmt.Sprintf("%s/%s/%s", repoBaseURL, release.TagName, expectedAssetName)

	fmt.Printf("Downloading %s...\n", expectedAssetName)

	// 3. Download to temp file
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() // Best-effort close of download response body.
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create temp file
	tempFile, err := os.CreateTemp("", updateArchivePattern(archiveExt))
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tempFile.Name()) // Best-effort cleanup of temporary archive.
	}()

	// Copy download to temp file
	_, err = io.Copy(tempFile, resp.Body)
	if closeErr := tempFile.Close(); closeErr != nil {
		return fmt.Errorf("failed to close temp file: %w", closeErr)
	}
	if err != nil {
		return fmt.Errorf("failed to save download: %w", err)
	}

	// 4. Extract the downloaded release archive.
	tempDir, err := os.MkdirTemp("", "yokai-extract-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir) // Best-effort cleanup of extraction directory.
	}()

	if err := extractArchive(tempFile.Name(), tempDir); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	newBinaryPath, err := findExtractedBinary(tempDir, mainBinaryName())
	if err != nil {
		return err
	}
	newTUIBinaryPath, err := findExtractedBinary(tempDir, companionBinaryName())
	if err != nil {
		return err
	}

	// 5. Replace current binary
	currentBinaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find current binary path: %w", err)
	}

	// Resolve symlinks
	currentBinaryPath, err = filepath.EvalSymlinks(currentBinaryPath)
	if err != nil {
		return fmt.Errorf("failed to resolve binary path: %w", err)
	}

	fmt.Println("Installing update...")
	if err := installUpdatePair(newBinaryPath, newTUIBinaryPath, currentBinaryPath); err != nil {
		return err
	}

	// 6. Print success message
	fmt.Printf("✅ Successfully updated to %s!\n", release.TagName)
	fmt.Println("Restart yokai to use the new version.")

	return nil
}

func companionBinaryName() string {
	return binaryNameForOS(tuiBinary, runtime.GOOS)
}

func mainBinaryName() string {
	return binaryNameForOS(mainBinary, runtime.GOOS)
}

func binaryNameForOS(name, goos string) string {
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}

func updateArchivePattern(archiveExt string) string {
	return "yokai-update-*" + archiveExt
}

func findExtractedBinary(root, name string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != name {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		found = path
		return filepath.SkipAll
	})
	if err != nil {
		return "", fmt.Errorf("finding %s in archive: %w", name, err)
	}
	if found == "" {
		return "", fmt.Errorf("%s binary not found in archive", name)
	}
	return found, nil
}

type updateFileOps struct {
	rename func(string, string) error
	chmod  func(string, os.FileMode) error
}

var defaultUpdateFileOps = updateFileOps{
	rename: os.Rename,
	chmod:  platform.ChmodIfSupported,
}

func installUpdatePair(newBinaryPath, newTUIBinaryPath, currentBinaryPath string) error {
	return installUpdatePairWithOps(newBinaryPath, newTUIBinaryPath, currentBinaryPath, defaultUpdateFileOps)
}

func installUpdatePairWithOps(
	newBinaryPath, newTUIBinaryPath, currentBinaryPath string,
	ops updateFileOps,
) error {
	currentDir := filepath.Dir(currentBinaryPath)
	currentTUIBinaryPath := filepath.Join(currentDir, companionBinaryName())

	// Stage both files beside the active installation. This avoids cross-device
	// rename failures after the old pair has already been moved out of the way.
	stagedBinaryPath, err := stageUpdateBinary(newBinaryPath, currentDir, ".yokai-update-*", ops)
	if err != nil {
		return fmt.Errorf("staging yokai binary: %w", err)
	}
	defer func() { _ = os.Remove(stagedBinaryPath) }()

	stagedTUIBinaryPath, err := stageUpdateBinary(newTUIBinaryPath, currentDir, ".yokai-tui-update-*", ops)
	if err != nil {
		return fmt.Errorf("staging yokai-tui binary: %w", err)
	}
	defer func() { _ = os.Remove(stagedTUIBinaryPath) }()

	oldBinaryPath, err := availableBackupPath(currentBinaryPath)
	if err != nil {
		return fmt.Errorf("reserving yokai backup path: %w", err)
	}
	oldTUIBinaryPath, err := availableBackupPath(currentTUIBinaryPath)
	if err != nil {
		return fmt.Errorf("reserving yokai-tui backup path: %w", err)
	}

	if err := ops.rename(currentBinaryPath, oldBinaryPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	hadOldTUI := true
	if err := ops.rename(currentTUIBinaryPath, oldTUIBinaryPath); err != nil {
		if os.IsNotExist(err) {
			hadOldTUI = false
		} else {
			cause := fmt.Errorf("backing up yokai-tui binary: %w", err)
			return withRollback(cause, ops.rename(oldBinaryPath, currentBinaryPath))
		}
	}

	if err := ops.rename(stagedBinaryPath, currentBinaryPath); err != nil {
		cause := fmt.Errorf("installing yokai binary: %w", err)
		return withRollback(cause, rollbackUpdatePair(
			currentBinaryPath, currentTUIBinaryPath,
			oldBinaryPath, oldTUIBinaryPath,
			hadOldTUI, ops,
		))
	}
	if err := ops.rename(stagedTUIBinaryPath, currentTUIBinaryPath); err != nil {
		cause := fmt.Errorf("installing yokai-tui binary: %w", err)
		return withRollback(cause, rollbackUpdatePair(
			currentBinaryPath, currentTUIBinaryPath,
			oldBinaryPath, oldTUIBinaryPath,
			hadOldTUI, ops,
		))
	}

	_ = os.Remove(oldBinaryPath)
	if hadOldTUI {
		_ = os.Remove(oldTUIBinaryPath)
	}
	return nil
}

func stageUpdateBinary(sourcePath, targetDir, pattern string, ops updateFileOps) (string, error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = source.Close() }()

	staged, err := os.CreateTemp(targetDir, pattern)
	if err != nil {
		return "", err
	}
	stagedPath := staged.Name()
	keep := false
	defer func() {
		_ = staged.Close()
		if !keep {
			_ = os.Remove(stagedPath)
		}
	}()

	if _, err := io.Copy(staged, source); err != nil {
		return "", err
	}
	if err := staged.Close(); err != nil {
		return "", err
	}
	if err := ops.chmod(stagedPath, 0755); err != nil {
		return "", err
	}

	keep = true
	return stagedPath, nil
}

func availableBackupPath(targetPath string) (string, error) {
	placeholder, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".old-*")
	if err != nil {
		return "", err
	}
	path := placeholder.Name()
	if err := placeholder.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func rollbackUpdatePair(
	currentBinaryPath, currentTUIBinaryPath string,
	oldBinaryPath, oldTUIBinaryPath string,
	hadOldTUI bool,
	ops updateFileOps,
) error {
	var rollbackErrors []error
	if err := removeIfExists(currentBinaryPath); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("removing new yokai binary: %w", err))
	}
	if err := removeIfExists(currentTUIBinaryPath); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("removing new yokai-tui binary: %w", err))
	}
	if err := ops.rename(oldBinaryPath, currentBinaryPath); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("restoring yokai binary: %w", err))
	}
	if hadOldTUI {
		if err := ops.rename(oldTUIBinaryPath, currentTUIBinaryPath); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restoring yokai-tui binary: %w", err))
		}
	}
	return errors.Join(rollbackErrors...)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func withRollback(cause, rollbackErr error) error {
	if rollbackErr == nil {
		return cause
	}
	return errors.Join(cause, fmt.Errorf("rollback failed: %w", rollbackErr))
}

// extractArchive extracts a release archive to the specified directory.
func extractArchive(src, dst string) error {
	if strings.HasSuffix(src, ".zip") {
		return extractZip(src, dst)
	}
	return extractTarGz(src, dst)
}

// extractTarGz extracts a tar.gz file to the specified directory.
func extractTarGz(src, dst string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close() // Best-effort close of source archive file.
	}()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() {
		_ = gzr.Close() // Best-effort close of gzip reader.
	}()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dst, header.Name)

		// Security check: prevent directory traversal
		if !strings.HasPrefix(path, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, header.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}

			outFile, err := os.Create(path)
			if err != nil {
				return err
			}

			_, err = io.Copy(outFile, tr)
			if closeErr := outFile.Close(); closeErr != nil {
				return closeErr
			}
			if err != nil {
				return err
			}

			if err := platform.ChmodIfSupported(path, header.FileInfo().Mode()); err != nil {
				return err
			}
		}
	}

	return nil
}

func extractZip(src, dst string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()

	for _, file := range reader.File {
		path := filepath.Join(dst, file.Name)
		if !strings.HasPrefix(path, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, file.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		in, err := file.Open()
		if err != nil {
			return err
		}

		out, err := os.Create(path)
		if err != nil {
			_ = in.Close()
			return err
		}

		_, copyErr := io.Copy(out, in)
		closeInErr := in.Close()
		closeOutErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeInErr != nil {
			return closeInErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}

		if err := platform.ChmodIfSupported(path, file.Mode()); err != nil {
			return err
		}
	}

	return nil
}
