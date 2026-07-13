package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/cloudsync"
	"github.com/spencerbull/yokai/internal/config"
)

func TestValidatePassphraseConfirmation(t *testing.T) {
	t.Parallel()
	if err := validatePassphraseConfirmation("matching passphrase", "matching passphrase"); err != nil {
		t.Fatalf("matching passphrases returned error: %v", err)
	}
	if err := validatePassphraseConfirmation("first passphrase", "second passphrase"); err == nil {
		t.Fatal("mismatched passphrases were accepted")
	}
}

func TestReloadDaemonWithClientTimesOut(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.Daemon.Listen = strings.TrimPrefix(server.URL, "http://")
	client := &http.Client{Timeout: 25 * time.Millisecond}
	started := time.Now()
	if reloadDaemonWithClient(context.Background(), cfg, client) {
		t.Fatal("stalled daemon reload unexpectedly succeeded")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("stalled daemon reload took %s", elapsed)
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("YOKAI_TEST_DEFAULT", " override ")
	if got := envOrDefault("YOKAI_TEST_DEFAULT", "fallback"); got != "override" {
		t.Fatalf("envOrDefault() = %q", got)
	}
	t.Setenv("YOKAI_TEST_DEFAULT", "")
	if got := envOrDefault("YOKAI_TEST_DEFAULT", "fallback"); got != "fallback" {
		t.Fatalf("envOrDefault() fallback = %q", got)
	}
}

func TestApplyCloudSnapshotPreservesNonDeviceSettings(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	cfg.HFToken = "keep-token"
	cfg.Services = []config.Service{{ID: "keep-service"}}
	cfg.Preferences.Theme = "keep-theme"
	cfg.Devices = []config.Device{{ID: "old-device"}}

	applyCloudSnapshot(cfg, cloudsync.Snapshot{
		Version: cloudsync.SnapshotVersion,
		Devices: []config.Device{{ID: "new-device"}},
	})

	if len(cfg.Devices) != 1 || cfg.Devices[0].ID != "new-device" {
		t.Fatalf("devices = %#v", cfg.Devices)
	}
	if cfg.HFToken != "keep-token" || len(cfg.Services) != 1 || cfg.Services[0].ID != "keep-service" || cfg.Preferences.Theme != "keep-theme" {
		t.Fatalf("non-device settings changed: %#v", cfg)
	}
}

func TestBackupLocalConfigCreatesPrivateExactCopy(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	cfg := config.DefaultConfig()
	cfg.HFToken = "preserve-me"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	originalPath, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}

	backupPath, err := backupLocalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(backupPath) != filepath.Dir(originalPath) {
		t.Fatalf("backup path = %q", backupPath)
	}
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(original) {
		t.Fatal("backup content differs from original")
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode = %o", info.Mode().Perm())
	}
}
