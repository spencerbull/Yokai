package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	githubAPI = "https://api.github.com/repos/spencerbull/yokai/releases/latest"
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
	defer resp.Body.Close()

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

	// 2. Find the correct asset for current OS/arch
	//    Asset naming: yokai_{version}_{os}_{arch}.tar.gz
	expectedAssetName := fmt.Sprintf("yokai_%s_%s_%s.tar.gz",
		strings.TrimPrefix(release.TagName, "v"), runtime.GOOS, runtime.GOARCH)

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create temp file
	tempFile, err := os.CreateTemp("", "yokai-update-*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	// Copy download to temp file
	_, err = io.Copy(tempFile, resp.Body)
	tempFile.Close()
	if err != nil {
		return fmt.Errorf("failed to save download: %w", err)
	}

	// 4. Extract tar.gz
	tempDir, err := os.MkdirTemp("", "yokai-extract-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := extractTarGz(tempFile.Name(), tempDir); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	// Find the yokai binary in extracted files
	newBinaryPath := filepath.Join(tempDir, "yokai")
	if _, err := os.Stat(newBinaryPath); os.IsNotExist(err) {
		return fmt.Errorf("yokai binary not found in archive")
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

	// Rename current binary to .old
	oldBinaryPath := currentBinaryPath + ".old"
	if err := os.Rename(currentBinaryPath, oldBinaryPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Move new binary to current path
	if err := os.Rename(newBinaryPath, currentBinaryPath); err != nil {
		// Try to restore old binary
		os.Rename(oldBinaryPath, currentBinaryPath)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Make it executable
	if err := os.Chmod(currentBinaryPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	// Remove .old
	os.Remove(oldBinaryPath)

	// 6. Print success message
	fmt.Printf("✅ Successfully updated to %s!\n", release.TagName)
	fmt.Println("Restart yokai to use the new version.")

	return nil
}

// extractTarGz extracts a tar.gz file to the specified directory
func extractTarGz(src, dst string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

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
			outFile.Close()
			if err != nil {
				return err
			}

			if err := os.Chmod(path, header.FileInfo().Mode()); err != nil {
				return err
			}
		}
	}

	return nil
}
