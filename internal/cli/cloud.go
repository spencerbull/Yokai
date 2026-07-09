package cli

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/spencerbull/yokai/internal/cloudsync"
	"github.com/spencerbull/yokai/internal/config"
)

// RunCloud dispatches encrypted cloud device configuration commands.
func RunCloud(args []string) {
	if len(args) == 0 {
		exitError("usage: yokai cloud <login|save|load|status|delete|logout>")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var err error
	switch args[0] {
	case "login":
		err = runCloudLogin(ctx, args[1:])
	case "save":
		err = runCloudSave(ctx)
	case "load":
		err = runCloudLoad(ctx)
	case "status":
		err = runCloudStatus(ctx)
	case "delete":
		err = runCloudDelete(ctx, args[1:])
	case "logout":
		err = runCloudLogout()
	default:
		err = fmt.Errorf("unknown cloud command %q", args[0])
	}
	if err != nil {
		exitError(err.Error())
	}
}

func runCloudLogin(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("cloud login", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectID := flags.String("project-id", envOrDefault("YOKAI_FIREBASE_PROJECT_ID", cloudsync.DefaultProjectID), "Firebase project ID")
	apiKey := flags.String("api-key", envOrDefault("YOKAI_FIREBASE_API_KEY", cloudsync.DefaultAPIKey), "Firebase web API key")
	clientID := flags.String("client-id", envOrDefault("YOKAI_GOOGLE_CLIENT_ID", cloudsync.DefaultGoogleClientID), "Google desktop OAuth client ID")
	clientSecret := flags.String("client-secret", envOrDefault("YOKAI_GOOGLE_CLIENT_SECRET", cloudsync.DefaultGoogleClientSecret), "Google desktop OAuth client secret")
	if err := flags.Parse(args); err != nil {
		return err
	}

	credentials, err := cloudsync.Login(ctx, cloudsync.LoginOptions{
		ProjectID:          strings.TrimSpace(*projectID),
		APIKey:             strings.TrimSpace(*apiKey),
		GoogleClientID:     strings.TrimSpace(*clientID),
		GoogleClientSecret: strings.TrimSpace(*clientSecret),
		AnnounceURL: func(url string) {
			fmt.Fprintln(os.Stderr, "Opening Google sign-in in your browser.")
			fmt.Fprintf(os.Stderr, "If it does not open, visit:\n%s\n", url)
		},
	})
	if err != nil {
		return err
	}
	outputJSON(map[string]interface{}{
		"status":     "logged_in",
		"email":      credentials.Email,
		"project_id": credentials.ProjectID,
	})
	return nil
}

func runCloudSave(ctx context.Context) error {
	passphrase, err := readCloudSavePassphrase()
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	envelope, err := cloudsync.Encrypt(cloudsync.Snapshot{
		Version: config.ConfigVersion,
		Devices: append([]config.Device(nil), cfg.Devices...),
	}, passphrase)
	if err != nil {
		return err
	}
	client, credentials, err := cloudsync.NewClient(ctx)
	if err != nil {
		return err
	}
	metadata, err := client.Put(ctx, envelope)
	if err != nil {
		return err
	}
	outputJSON(map[string]interface{}{
		"status":       "saved",
		"email":        credentials.Email,
		"device_count": len(cfg.Devices),
		"updated_at":   metadata.UpdatedAt,
	})
	return nil
}

func runCloudLoad(ctx context.Context) error {
	client, credentials, err := cloudsync.NewClient(ctx)
	if err != nil {
		return err
	}
	envelope, metadata, err := client.Get(ctx)
	if err != nil {
		return err
	}
	passphrase, err := readCloudPassphrase("Cloud encryption passphrase: ")
	if err != nil {
		return err
	}
	snapshot, err := cloudsync.Decrypt(envelope, passphrase)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading local config: %w", err)
	}
	backupPath, err := backupLocalConfig()
	if err != nil {
		return err
	}
	cfg.Devices = append([]config.Device(nil), snapshot.Devices...)
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving loaded config: %w", err)
	}
	reloaded := reloadDaemonAfterCloudLoad(ctx, cfg)
	outputJSON(map[string]interface{}{
		"status":           "loaded",
		"email":            credentials.Email,
		"device_count":     len(snapshot.Devices),
		"cloud_updated_at": metadata.UpdatedAt,
		"backup_path":      backupPath,
		"daemon_reloaded":  reloaded,
	})
	return nil
}

func runCloudStatus(ctx context.Context) error {
	_, credentials, err := cloudsync.NewClient(ctx)
	if err != nil {
		return err
	}
	outputJSON(map[string]interface{}{
		"status":       "logged_in",
		"email":        credentials.Email,
		"project_id":   credentials.ProjectID,
		"token_expiry": credentials.Token.Expiry,
	})
	return nil
}

func runCloudDelete(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("cloud delete", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	confirmed := flags.Bool("yes", false, "permanently delete without prompting")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*confirmed {
		return fmt.Errorf("cloud delete is permanent; rerun with --yes")
	}
	client, credentials, err := cloudsync.NewClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Delete(ctx); err != nil {
		return err
	}
	outputJSON(map[string]interface{}{"status": "deleted", "email": credentials.Email})
	return nil
}

func runCloudLogout() error {
	if err := cloudsync.RemoveCredentials(); err != nil {
		return err
	}
	outputJSON(map[string]string{"status": "logged_out"})
	return nil
}

func readCloudPassphrase(prompt string) (string, error) {
	if value := os.Getenv("YOKAI_CLOUD_PASSPHRASE"); value != "" {
		return value, nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("set YOKAI_CLOUD_PASSPHRASE when stdin is not a terminal")
	}
	fmt.Fprint(os.Stderr, prompt)
	value, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("reading passphrase: %w", err)
	}
	return string(value), nil
}

func readCloudSavePassphrase() (string, error) {
	if value := os.Getenv("YOKAI_CLOUD_PASSPHRASE"); value != "" {
		return value, nil
	}
	passphrase, err := readCloudPassphrase("Cloud encryption passphrase: ")
	if err != nil {
		return "", err
	}
	confirmation, err := readCloudPassphrase("Confirm cloud encryption passphrase: ")
	if err != nil {
		return "", err
	}
	if err := validatePassphraseConfirmation(passphrase, confirmation); err != nil {
		return "", err
	}
	return passphrase, nil
}

func validatePassphraseConfirmation(passphrase, confirmation string) error {
	if passphrase != confirmation {
		return fmt.Errorf("cloud encryption passphrases do not match")
	}
	return nil
}

func backupLocalConfig() (string, error) {
	path, err := config.ConfigPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading local config for backup: %w", err)
	}
	backup := filepath.Join(filepath.Dir(path), fmt.Sprintf("config.cloud-backup-%s.json", time.Now().UTC().Format("20060102T150405.000000000Z")))
	file, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", fmt.Errorf("writing local config backup: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(backup)
		return "", fmt.Errorf("writing local config backup: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("closing local config backup: %w", err)
	}
	return backup, nil
}

func reloadDaemonAfterCloudLoad(ctx context.Context, cfg *config.Config) bool {
	return reloadDaemonWithClient(ctx, cfg, &http.Client{Timeout: 3 * time.Second})
}

func reloadDaemonWithClient(ctx context.Context, cfg *config.Config, client *http.Client) bool {
	addr := cfg.Daemon.Listen
	if addr == "" {
		addr = "127.0.0.1:7473"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/reload", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
