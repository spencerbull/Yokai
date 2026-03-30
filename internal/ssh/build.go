package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuildLocalBinaryForTarget builds a temporary yokai binary for the target platform.
func BuildLocalBinaryForTarget(kernelOS, arch string) (string, error) {
	goos, err := normalizeTargetOS(kernelOS)
	if err != nil {
		return "", err
	}
	goarch, err := normalizeTargetArch(arch)
	if err != nil {
		return "", err
	}

	tmpDir, err := os.MkdirTemp("", "yokai-build-*")
	if err != nil {
		return "", fmt.Errorf("creating temp build dir: %w", err)
	}

	binaryPath := filepath.Join(tmpDir, "yokai")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/yokai")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("building yokai for %s/%s: %w — output: %s", goos, goarch, err, strings.TrimSpace(string(out)))
	}

	return binaryPath, nil
}

func normalizeTargetOS(kernelOS string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kernelOS)) {
	case "linux":
		return "linux", nil
	case "darwin":
		return "darwin", nil
	default:
		return "", fmt.Errorf("unsupported remote OS %q", kernelOS)
	}
}

func normalizeTargetArch(arch string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "x86_64", "amd64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported remote architecture %q", arch)
	}
}
