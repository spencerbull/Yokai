package upgrade

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
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
	githubAPI   = "https://api.github.com/repos/spencerbull/yokai/releases/latest"
	projectName = "Yokai"
	tuiBinary   = "yokai-tui"
)

// Release represents a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Check checks for a newer version on GitHub.
func Check(currentVersion string) (*Release, bool, error) {
	// GET the latest release from GitHub API
	resp, err := http.Get(githubAPI)
	if err != nil {
		return nil, false, fmt.Errorf("failed to fetch release info: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() // Best-effort close of release API response body.
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read response body: %w", err)
	}

	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, false, fmt.Errorf("failed to parse release JSON: %w", err)
	}

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

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == expectedAssetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no compatible binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

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
	tempFile, err := os.CreateTemp("", "yokai-update-*.tar.gz")
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

	// 4. Extract tar.gz
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

	// Find the yokai binary in extracted files
	newBinaryPath := filepath.Join(tempDir, "yokai")
	if _, err := os.Stat(newBinaryPath); os.IsNotExist(err) {
		return fmt.Errorf("yokai binary not found in archive")
	}
	newTUIBinaryPath := filepath.Join(tempDir, companionBinaryName())

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
	currentDir := filepath.Dir(currentBinaryPath)
	currentTUIBinaryPath := filepath.Join(currentDir, companionBinaryName())

	// Rename current binary to .old
	oldBinaryPath := currentBinaryPath + ".old"
	if err := os.Rename(currentBinaryPath, oldBinaryPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}
	oldTUIBinaryPath := currentTUIBinaryPath + ".old"
	hasNewTUIBinary := false
	if _, err := os.Stat(newTUIBinaryPath); err == nil {
		hasNewTUIBinary = true
		if err := backupCompanionBinary(currentTUIBinaryPath, oldTUIBinaryPath); err != nil {
			_ = os.Rename(oldBinaryPath, currentBinaryPath)
			return err
		}
	}

	// Move new binary to current path
	if err := os.Rename(newBinaryPath, currentBinaryPath); err != nil {
		// Try to restore old binary
		_ = os.Rename(oldBinaryPath, currentBinaryPath) // Best-effort rollback to previous binary.
		return fmt.Errorf("failed to install new binary: %w", err)
	}
	if hasNewTUIBinary {
		if err := os.Rename(newTUIBinaryPath, currentTUIBinaryPath); err != nil {
			_ = os.Remove(currentBinaryPath)
			_ = os.Rename(oldBinaryPath, currentBinaryPath)
			_ = restoreCompanionBinary(currentTUIBinaryPath, oldTUIBinaryPath)
			return fmt.Errorf("failed to install yokai-tui binary: %w", err)
		}
	}

	// Make it executable
	if err := platform.ChmodIfSupported(currentBinaryPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}
	if hasNewTUIBinary {
		if err := platform.ChmodIfSupported(currentTUIBinaryPath, 0755); err != nil {
			return fmt.Errorf("failed to make yokai-tui executable: %w", err)
		}
	}

	// Remove .old
	_ = os.Remove(oldBinaryPath) // Best-effort cleanup of backup binary.
	if hasNewTUIBinary {
		_ = os.Remove(oldTUIBinaryPath)
	}

	// 6. Print success message
	fmt.Printf("✅ Successfully updated to %s!\n", release.TagName)
	fmt.Println("Restart yokai to use the new version.")

	return nil
}

func companionBinaryName() string {
	if runtime.GOOS == "windows" {
		return tuiBinary + ".exe"
	}
	return tuiBinary
}

func backupCompanionBinary(currentPath, backupPath string) error {
	if err := os.Rename(currentPath, backupPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to backup yokai-tui binary: %w", err)
	}
	return nil
}

func restoreCompanionBinary(currentPath, backupPath string) error {
	if err := os.Rename(backupPath, currentPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
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
