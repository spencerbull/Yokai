package claudecode

import (
	"fmt"
	"os"
	"path/filepath"
)

const UnsupportedMessage = "Claude Code does not currently document a generic OpenAI-compatible local endpoint configuration, so Yokai cannot auto-configure it safely yet"

func DetectSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func HasYokaiConfig(_ string) bool {
	return false
}
