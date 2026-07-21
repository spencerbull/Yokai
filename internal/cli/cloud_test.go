package cli

import (
	"context"
	"errors"
	"flag"
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

func TestCloudSubcommandHelpReturnsBeforeActions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	credentials := &cloudsync.Credentials{
		ProjectID: "test-project",
		APIKey:    "test-key",
		UID:       "test-user",
		Token:     cloudsync.StoredToken{RefreshToken: "refresh-token"},
	}
	if err := cloudsync.SaveCredentials(credentials); err != nil {
		t.Fatal(err)
	}
	credentialsPath, err := cloudsync.CredentialsPath()
	if err != nil {
		t.Fatal(err)
	}

	commands := map[string]func() error{
		"login":  func() error { return runCloudLogin(context.Background(), []string{"--help"}) },
		"save":   func() error { return runCloudSave(context.Background(), []string{"--help"}) },
		"load":   func() error { return runCloudLoad(context.Background(), []string{"--help"}) },
		"status": func() error { return runCloudStatus(context.Background(), []string{"--help"}) },
		"delete": func() error { return runCloudDelete(context.Background(), []string{"--help"}) },
		"logout": func() error { return runCloudLogout([]string{"--help"}) },
	}
	for name, run := range commands {
		if err := run(); !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("%s --help error = %v", name, err)
		}
	}
	if _, err := os.Stat(credentialsPath); err != nil {
		t.Fatalf("help mutated cloud credentials: %v", err)
	}
	if err := runCloudLogout([]string{"unexpected"}); err == nil {
		t.Fatal("cloud logout accepted an unexpected argument")
	}
	if _, err := os.Stat(credentialsPath); err != nil {
		t.Fatalf("invalid arguments mutated cloud credentials: %v", err)
	}
}

func TestCloudSaveRefusesEmptyDeviceListBeforeLogin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	err := runCloudSave(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "local config has no devices") {
		t.Fatalf("runCloudSave() error = %v", err)
	}
}

func TestReadCloudConfirmationDefaultsToNo(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		input string
		want  bool
	}{
		{input: "yes\n", want: true},
		{input: "Y\n", want: true},
		{input: "\n", want: false},
		{input: "no\n", want: false},
	} {
		var output strings.Builder
		got, err := readCloudConfirmation(strings.NewReader(test.input), &output, "Continue? ")
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("confirmation %q = %v, want %v", test.input, got, test.want)
		}
		if output.String() != "Continue? " {
			t.Fatalf("prompt = %q", output.String())
		}
	}
}

type fakeCloudConfigClient struct {
	getEnvelope cloudsync.Envelope
	getMetadata cloudsync.Metadata
	getErr      error
	putCalls    int
	putEnvelope cloudsync.Envelope
	putPrevious *cloudsync.Metadata
	putErr      error
}

func (f *fakeCloudConfigClient) Get(context.Context) (cloudsync.Envelope, cloudsync.Metadata, error) {
	return f.getEnvelope, f.getMetadata, f.getErr
}

func (f *fakeCloudConfigClient) Put(_ context.Context, envelope cloudsync.Envelope, previous *cloudsync.Metadata) (cloudsync.Metadata, error) {
	f.putCalls++
	f.putEnvelope = envelope
	f.putPrevious = previous
	if f.putErr != nil {
		return cloudsync.Metadata{}, f.putErr
	}
	return cloudsync.Metadata{UpdatedAt: time.Date(2026, 7, 13, 21, 0, 0, 0, time.UTC)}, nil
}

func (f *fakeCloudConfigClient) Delete(context.Context) error { return nil }

func testCloudInteraction(input string, terminal bool, passphrase string, outputs *[]interface{}) cloudCommandInteraction {
	return cloudCommandInteraction{
		stdin:      strings.NewReader(input),
		stderr:     &strings.Builder{},
		isTerminal: terminal,
		readPassphrase: func(string) (string, error) {
			return passphrase, nil
		},
		readSavePassphrase: func() (string, error) {
			return passphrase, nil
		},
		output: func(value interface{}) {
			*outputs = append(*outputs, value)
		},
	}
}

func testCloudEnvelope(t *testing.T, devices []config.Device, passphrase string) cloudsync.Envelope {
	t.Helper()
	envelope, err := cloudsync.Encrypt(cloudsync.Snapshot{Version: cloudsync.SnapshotVersion, Devices: devices}, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func TestSaveCloudConfigDoesNotOverwriteOnCancelOrWrongPassphrase(t *testing.T) {
	const passphrase = "correct test passphrase"
	existing := testCloudEnvelope(t, []config.Device{{ID: "cloud-device"}}, passphrase)
	credentials := &cloudsync.Credentials{Email: "user@example.com"}
	cfg := config.DefaultConfig()
	cfg.Devices = []config.Device{{ID: "local-device"}}

	t.Run("cancel", func(t *testing.T) {
		client := &fakeCloudConfigClient{getEnvelope: existing, getMetadata: cloudsync.Metadata{UpdatedAt: time.Now()}}
		var outputs []interface{}
		err := saveCloudConfig(context.Background(), client, credentials, cfg, false, testCloudInteraction("n\n", true, passphrase, &outputs))
		if err != nil {
			t.Fatal(err)
		}
		if client.putCalls != 0 || len(outputs) != 1 {
			t.Fatalf("cancel putCalls=%d outputs=%#v", client.putCalls, outputs)
		}
	})

	t.Run("wrong passphrase", func(t *testing.T) {
		client := &fakeCloudConfigClient{getEnvelope: existing}
		var outputs []interface{}
		err := saveCloudConfig(context.Background(), client, credentials, cfg, true, testCloudInteraction("", false, "incorrect test passphrase", &outputs))
		if err == nil || client.putCalls != 0 || len(outputs) != 0 {
			t.Fatalf("wrong passphrase error=%v putCalls=%d outputs=%#v", err, client.putCalls, outputs)
		}
	})
}

func TestSaveCloudConfigConfirmedAndAllowEmptyPaths(t *testing.T) {
	const passphrase = "correct test passphrase"
	credentials := &cloudsync.Credentials{Email: "user@example.com"}

	t.Run("confirmed replacement", func(t *testing.T) {
		existing := testCloudEnvelope(t, []config.Device{{ID: "cloud-device"}}, passphrase)
		existingMetadata := cloudsync.Metadata{UpdatedAt: time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC)}
		client := &fakeCloudConfigClient{getEnvelope: existing, getMetadata: existingMetadata}
		cfg := config.DefaultConfig()
		cfg.Devices = []config.Device{{ID: "local-device"}}
		var outputs []interface{}
		if err := saveCloudConfig(context.Background(), client, credentials, cfg, true, testCloudInteraction("", false, passphrase, &outputs)); err != nil {
			t.Fatal(err)
		}
		if client.putCalls != 1 || client.putPrevious == nil || !client.putPrevious.UpdatedAt.Equal(existingMetadata.UpdatedAt) || len(outputs) != 1 {
			t.Fatalf("replacement putCalls=%d previous=%#v outputs=%#v", client.putCalls, client.putPrevious, outputs)
		}
	})

	t.Run("explicit empty backup", func(t *testing.T) {
		cfg := config.DefaultConfig()
		if err := validateCloudSaveConfig(cfg, false); err == nil {
			t.Fatal("empty config passed without allow-empty")
		}
		if err := validateCloudSaveConfig(cfg, true); err != nil {
			t.Fatal(err)
		}
		client := &fakeCloudConfigClient{getErr: cloudsync.ErrNoCloudConfig}
		var outputs []interface{}
		if err := saveCloudConfig(context.Background(), client, credentials, cfg, true, testCloudInteraction("", false, passphrase, &outputs)); err != nil {
			t.Fatal(err)
		}
		if client.putCalls != 1 {
			t.Fatalf("empty backup putCalls=%d", client.putCalls)
		}
		if client.putPrevious != nil {
			t.Fatalf("new backup previous = %#v", client.putPrevious)
		}
	})
}

func TestSaveCloudConfigDoesNotReportSuccessAfterConcurrentChange(t *testing.T) {
	const passphrase = "correct test passphrase"
	existingMetadata := cloudsync.Metadata{UpdatedAt: time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC)}
	client := &fakeCloudConfigClient{
		getEnvelope: testCloudEnvelope(t, []config.Device{{ID: "cloud-device"}}, passphrase),
		getMetadata: existingMetadata,
		putErr:      cloudsync.ErrCloudConfigChanged,
	}
	credentials := &cloudsync.Credentials{Email: "user@example.com"}
	cfg := config.DefaultConfig()
	cfg.Devices = []config.Device{{ID: "local-device"}}
	var outputs []interface{}

	err := saveCloudConfig(context.Background(), client, credentials, cfg, true, testCloudInteraction("", false, passphrase, &outputs))
	if !errors.Is(err, cloudsync.ErrCloudConfigChanged) {
		t.Fatalf("saveCloudConfig() error = %v", err)
	}
	if client.putCalls != 1 || len(outputs) != 0 {
		t.Fatalf("conflict putCalls=%d outputs=%#v", client.putCalls, outputs)
	}
}

func TestLoadCloudConfigDoesNotMutateOnCancelOrWrongPassphrase(t *testing.T) {
	const passphrase = "correct test passphrase"
	envelope := testCloudEnvelope(t, []config.Device{{ID: "cloud-device"}}, passphrase)
	credentials := &cloudsync.Credentials{Email: "user@example.com"}
	newEffects := func(calls *int) cloudLoadEffects {
		return cloudLoadEffects{
			backup: func() (string, error) { (*calls)++; return "backup.json", nil },
			save:   func(*config.Config) error { (*calls)++; return nil },
			reload: func(context.Context, *config.Config) bool { (*calls)++; return true },
		}
	}

	for _, test := range []struct {
		name       string
		confirmed  bool
		input      string
		passphrase string
		wantErr    bool
	}{
		{name: "cancel", confirmed: false, input: "n\n", passphrase: passphrase},
		{name: "wrong passphrase", confirmed: true, passphrase: "incorrect test passphrase", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeCloudConfigClient{getEnvelope: envelope, getMetadata: cloudsync.Metadata{UpdatedAt: time.Now()}}
			cfg := config.DefaultConfig()
			cfg.HFToken = "preserve-token"
			cfg.Devices = []config.Device{{ID: "local-device"}}
			var outputs []interface{}
			calls := 0
			err := loadCloudConfig(context.Background(), client, credentials, cfg, test.confirmed, testCloudInteraction(test.input, true, test.passphrase, &outputs), newEffects(&calls))
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v", err)
			}
			if calls != 0 || cfg.Devices[0].ID != "local-device" || cfg.HFToken != "preserve-token" {
				t.Fatalf("load mutated state: calls=%d cfg=%#v", calls, cfg)
			}
		})
	}
}

func TestLoadCloudConfigConfirmedPreservesOtherSettings(t *testing.T) {
	const passphrase = "correct test passphrase"
	envelope := testCloudEnvelope(t, []config.Device{{ID: "cloud-device"}}, passphrase)
	client := &fakeCloudConfigClient{getEnvelope: envelope, getMetadata: cloudsync.Metadata{UpdatedAt: time.Now()}}
	cfg := config.DefaultConfig()
	cfg.HFToken = "preserve-token"
	cfg.Services = []config.Service{{ID: "preserve-service"}}
	cfg.Devices = []config.Device{{ID: "local-device"}}
	var outputs []interface{}
	backupCalls, saveCalls, reloadCalls := 0, 0, 0
	effects := cloudLoadEffects{
		backup: func() (string, error) { backupCalls++; return "backup.json", nil },
		save: func(saved *config.Config) error {
			saveCalls++
			if saved.HFToken != "preserve-token" || len(saved.Services) != 1 || saved.Devices[0].ID != "cloud-device" {
				t.Fatalf("saved config = %#v", saved)
			}
			return nil
		},
		reload: func(context.Context, *config.Config) bool { reloadCalls++; return true },
	}
	if err := loadCloudConfig(context.Background(), client, &cloudsync.Credentials{Email: "user@example.com"}, cfg, true, testCloudInteraction("", false, passphrase, &outputs), effects); err != nil {
		t.Fatal(err)
	}
	if backupCalls != 1 || saveCalls != 1 || reloadCalls != 1 || len(outputs) != 1 {
		t.Fatalf("effects backup=%d save=%d reload=%d outputs=%#v", backupCalls, saveCalls, reloadCalls, outputs)
	}
}

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
