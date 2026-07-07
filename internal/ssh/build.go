package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const defaultGoVersion = "1.25"

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
	cmd, err := localGoBuildCommand(binaryPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", err
	}
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("building yokai for %s/%s: %w — output: %s", goos, goarch, err, strings.TrimSpace(string(out)))
	}

	return binaryPath, nil
}

func localGoBuildCommand(binaryPath string) (*exec.Cmd, error) {
	if goPath, err := exec.LookPath("go"); err == nil {
		return exec.Command(goPath, "build", "-o", binaryPath, "./cmd/yokai"), nil
	}

	if misePath, err := exec.LookPath("mise"); err == nil {
		return exec.Command(misePath, "exec", "go@"+defaultGoVersion, "--", "go", "build", "-o", binaryPath, "./cmd/yokai"), nil
	}

	return nil, fmt.Errorf("go toolchain not found locally (expected either `go` on PATH or `mise exec go@%s`)", defaultGoVersion)
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
