package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := runCloud(ctx, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		exitError(err.Error())
	}
}

func runCloud(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printCloudUsage(os.Stdout)
		return nil
	}
	switch args[0] {
	case "login":
		return runCloudLogin(ctx, args[1:])
	case "save":
		return runCloudSave(ctx, args[1:])
	case "load":
		return runCloudLoad(ctx, args[1:])
	case "status":
		return runCloudStatus(ctx, args[1:])
	case "delete":
		return runCloudDelete(ctx, args[1:])
	case "logout":
		return runCloudLogout(args[1:])
	default:
		return fmt.Errorf("unknown cloud command %q; run 'yokai cloud --help'", args[0])
	}
}

func printCloudUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Securely back up Yokai device records.

Usage:
  yokai cloud login [flags]       Sign in with Google on this computer
  yokai cloud save [flags]        Encrypt and upload local devices
  yokai cloud load [flags]        Preview and replace local devices
  yokai cloud status [--verify]   Show login and backup status
  yokai cloud delete --yes        Permanently delete the cloud backup
  yokai cloud logout              Sign out on this computer

Run 'yokai cloud <command> --help' for command-specific options.
Only device records are synced. Other Yokai settings stay local.
`)
}

type cloudConfigClient interface {
	Put(context.Context, cloudsync.Envelope, *cloudsync.Metadata) (cloudsync.Metadata, error)
	Get(context.Context) (cloudsync.Envelope, cloudsync.Metadata, error)
	Delete(context.Context) error
}

type cloudCommandInteraction struct {
	stdin              io.Reader
	stderr             io.Writer
	isTerminal         bool
	readPassphrase     func(string) (string, error)
	readSavePassphrase func() (string, error)
	output             func(interface{})
}

func defaultCloudCommandInteraction() cloudCommandInteraction {
	return cloudCommandInteraction{
		stdin:              os.Stdin,
		stderr:             os.Stderr,
		isTerminal:         term.IsTerminal(int(os.Stdin.Fd())),
		readPassphrase:     readCloudPassphrase,
		readSavePassphrase: readCloudSavePassphrase,
		output:             outputJSON,
	}
}

type cloudLoadEffects struct {
	backup func() (string, error)
	save   func(*config.Config) error
	reload func(context.Context, *config.Config) bool
}

func runCloudLogin(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("cloud login", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(flags.Output(), "Usage: yokai cloud login [--project-id ID --api-key KEY --client-id ID --client-secret SECRET]")
		flags.PrintDefaults()
	}
	projectID := flags.String("project-id", envOrDefault("YOKAI_FIREBASE_PROJECT_ID", cloudsync.DefaultProjectID), "Firebase project ID")
	apiKey := flags.String("api-key", envOrDefault("YOKAI_FIREBASE_API_KEY", cloudsync.DefaultAPIKey), "Firebase web API key")
	clientID := flags.String("client-id", envOrDefault("YOKAI_GOOGLE_CLIENT_ID", cloudsync.DefaultGoogleClientID), "Google desktop OAuth client ID")
	clientSecret := flags.String("client-secret", envOrDefault("YOKAI_GOOGLE_CLIENT_SECRET", cloudsync.DefaultGoogleClientSecret), "Google desktop OAuth client secret")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("cloud login does not accept positional arguments")
	}

	fmt.Fprintln(os.Stderr, "Opening Google sign-in in your browser...")
	credentials, err := cloudsync.Login(ctx, cloudsync.LoginOptions{
		ProjectID:          strings.TrimSpace(*projectID),
		APIKey:             strings.TrimSpace(*apiKey),
		GoogleClientID:     strings.TrimSpace(*clientID),
		GoogleClientSecret: strings.TrimSpace(*clientSecret),
		AnnounceURL: func(url string) {
			fmt.Fprintf(os.Stderr, "If the browser does not open, continue sign-in at:\n%s\n", url)
		},
	})
	if err != nil {
		return err
	}
	outputJSON(map[string]interface{}{
		"status":     "logged_in",
		"email":      credentials.Email,
		"project_id": credentials.ProjectID,
		"message":    "Signed in on this computer. Run 'yokai cloud status' to check for an existing backup.",
	})
	return nil
}

func runCloudSave(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("cloud save", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	confirmed := flags.Bool("yes", false, "replace an existing cloud backup without prompting")
	allowEmpty := flags.Bool("allow-empty", false, "allow an empty device list to replace the cloud backup")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(flags.Output(), "Usage: yokai cloud save [--yes] [--allow-empty]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("cloud save does not accept positional arguments")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if err := validateCloudSaveConfig(cfg, *allowEmpty); err != nil {
		return err
	}
	client, credentials, err := cloudsync.NewClient(ctx)
	if err != nil {
		return err
	}
	return saveCloudConfig(ctx, client, credentials, cfg, *confirmed, defaultCloudCommandInteraction())
}

func validateCloudSaveConfig(cfg *config.Config, allowEmpty bool) error {
	if len(cfg.Devices) == 0 && !allowEmpty {
		return fmt.Errorf("local config has no devices; refusing to replace the cloud backup (use --allow-empty only if this is intentional)")
	}
	return nil
}

func saveCloudConfig(ctx context.Context, client cloudConfigClient, credentials *cloudsync.Credentials, cfg *config.Config, confirmed bool, interaction cloudCommandInteraction) error {
	existing, existingMetadata, err := client.Get(ctx)
	hasExisting := err == nil
	if err != nil && !errors.Is(err, cloudsync.ErrNoCloudConfig) {
		return err
	}
	if hasExisting && !confirmed {
		if !interaction.isTerminal {
			return fmt.Errorf("a cloud backup already exists; rerun with --yes to replace it")
		}
		_, _ = fmt.Fprintf(interaction.stderr, "Replace the backup for %s from %s with %d local device(s)?\n", credentials.Email, existingMetadata.UpdatedAt.Local().Format(time.RFC1123), len(cfg.Devices))
		ok, err := readCloudConfirmation(interaction.stdin, interaction.stderr, "Continue? [y/N]: ")
		if err != nil {
			return err
		}
		if !ok {
			interaction.output(map[string]string{"status": "cancelled", "message": "Cloud backup was not changed."})
			return nil
		}
	}
	if !hasExisting {
		_, _ = fmt.Fprintf(interaction.stderr, "Creating an encrypted backup for %s with %d device(s).\n", credentials.Email, len(cfg.Devices))
	}

	var passphrase string
	if hasExisting {
		passphrase, err = interaction.readPassphrase("Existing cloud backup passphrase: ")
		if err != nil {
			return err
		}
		if _, err := cloudsync.Decrypt(existing, passphrase); err != nil {
			return fmt.Errorf("cannot replace the existing backup: %w", err)
		}
	} else {
		passphrase, err = interaction.readSavePassphrase()
		if err != nil {
			return err
		}
	}
	envelope, err := cloudsync.Encrypt(cloudsync.Snapshot{
		Version: cloudsync.SnapshotVersion,
		Devices: append([]config.Device(nil), cfg.Devices...),
	}, passphrase)
	if err != nil {
		return err
	}
	var previous *cloudsync.Metadata
	if hasExisting {
		previous = &existingMetadata
	}
	metadata, err := client.Put(ctx, envelope, previous)
	if err != nil {
		return err
	}
	interaction.output(map[string]interface{}{
		"status":       "saved",
		"email":        credentials.Email,
		"device_count": len(cfg.Devices),
		"updated_at":   metadata.UpdatedAt,
		"message":      fmt.Sprintf("Encrypted backup saved with %d device(s).", len(cfg.Devices)),
	})
	return nil
}

func runCloudLoad(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("cloud load", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	confirmed := flags.Bool("yes", false, "replace local devices without prompting")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(flags.Output(), "Usage: yokai cloud load [--yes]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("cloud load does not accept positional arguments")
	}

	client, credentials, err := cloudsync.NewClient(ctx)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading local config: %w", err)
	}
	return loadCloudConfig(ctx, client, credentials, cfg, *confirmed, defaultCloudCommandInteraction(), cloudLoadEffects{
		backup: backupLocalConfig,
		save:   config.Save,
		reload: reloadDaemonAfterCloudLoad,
	})
}

func loadCloudConfig(ctx context.Context, client cloudConfigClient, credentials *cloudsync.Credentials, cfg *config.Config, confirmed bool, interaction cloudCommandInteraction, effects cloudLoadEffects) error {
	envelope, metadata, err := client.Get(ctx)
	if err != nil {
		return err
	}
	if !confirmed && !interaction.isTerminal {
		return fmt.Errorf("cloud load replaces the local device list; rerun with --yes after reviewing the backup")
	}
	if !confirmed {
		_, _ = fmt.Fprintf(interaction.stderr, "Backup found for %s from %s. Enter its passphrase to preview the restore.\n", credentials.Email, metadata.UpdatedAt.Local().Format(time.RFC1123))
	}
	passphrase, err := interaction.readPassphrase("Cloud encryption passphrase: ")
	if err != nil {
		return err
	}
	snapshot, err := cloudsync.Decrypt(envelope, passphrase)
	if err != nil {
		return err
	}
	if !confirmed {
		_, _ = fmt.Fprintf(interaction.stderr, "Restore %d device(s) saved %s for %s?\n", len(snapshot.Devices), metadata.UpdatedAt.Local().Format(time.RFC1123), credentials.Email)
		_, _ = fmt.Fprintf(interaction.stderr, "This replaces %d local device(s). Other Yokai settings are preserved.\n", len(cfg.Devices))
		ok, err := readCloudConfirmation(interaction.stdin, interaction.stderr, "Continue? [y/N]: ")
		if err != nil {
			return err
		}
		if !ok {
			interaction.output(map[string]string{"status": "cancelled", "message": "Local devices were not changed."})
			return nil
		}
	}
	backupPath, err := effects.backup()
	if err != nil {
		return err
	}
	applyCloudSnapshot(cfg, snapshot)
	if err := effects.save(cfg); err != nil {
		return fmt.Errorf("saving loaded config: %w", err)
	}
	reloaded := effects.reload(ctx, cfg)
	result := map[string]interface{}{
		"status":           "loaded",
		"email":            credentials.Email,
		"device_count":     len(snapshot.Devices),
		"cloud_updated_at": metadata.UpdatedAt,
		"backup_path":      backupPath,
		"daemon_reloaded":  reloaded,
		"message":          fmt.Sprintf("Restored %d device(s); other Yokai settings were preserved.", len(snapshot.Devices)),
	}
	if backupPath != "" {
		result["undo_backup_path"] = backupPath
	}
	if reloaded {
		result["daemon_reload_status"] = "reloaded"
	} else {
		result["daemon_reload_status"] = "not_running_or_unreachable"
		result["next_step"] = "Restart Yokai to use the restored device list."
	}
	interaction.output(result)
	return nil
}

func applyCloudSnapshot(cfg *config.Config, snapshot cloudsync.Snapshot) {
	cfg.Devices = append([]config.Device(nil), snapshot.Devices...)
}

func runCloudStatus(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("cloud status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	verify := flags.Bool("verify", false, "decrypt the backup and report its device count")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(flags.Output(), "Usage: yokai cloud status [--verify]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("cloud status does not accept positional arguments")
	}

	_, err := cloudsync.LoadCredentials()
	if errors.Is(err, cloudsync.ErrNotLoggedIn) {
		outputJSON(map[string]interface{}{
			"status":        "logged_out",
			"backup_status": "unknown_until_login",
			"message":       "This computer is not signed in. Run 'yokai cloud login' to check or create a backup.",
		})
		return nil
	}
	if err != nil {
		return err
	}
	client, credentials, err := cloudsync.NewClient(ctx)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading local config: %w", err)
	}
	envelope, metadata, err := client.Get(ctx)
	if errors.Is(err, cloudsync.ErrNoCloudConfig) {
		outputJSON(map[string]interface{}{
			"status":             "logged_in",
			"email":              credentials.Email,
			"project_id":         credentials.ProjectID,
			"backup_exists":      false,
			"local_device_count": len(cfg.Devices),
			"token_expiry":       credentials.Token.Expiry,
			"message":            "Signed in, but no cloud backup exists. Run 'yokai cloud save' to create one.",
		})
		return nil
	}
	if err != nil {
		return err
	}
	result := map[string]interface{}{
		"status":             "logged_in",
		"email":              credentials.Email,
		"project_id":         credentials.ProjectID,
		"backup_exists":      true,
		"backup_updated_at":  metadata.UpdatedAt,
		"encrypted_size":     metadata.Size,
		"local_device_count": len(cfg.Devices),
		"token_expiry":       credentials.Token.Expiry,
		"message":            "An encrypted cloud backup is available.",
	}
	if *verify {
		passphrase, err := readCloudPassphrase("Cloud encryption passphrase: ")
		if err != nil {
			return err
		}
		snapshot, err := cloudsync.Decrypt(envelope, passphrase)
		if err != nil {
			return err
		}
		result["backup_device_count"] = len(snapshot.Devices)
		result["verified"] = true
	}
	outputJSON(result)
	return nil
}

func runCloudDelete(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("cloud delete", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	confirmed := flags.Bool("yes", false, "permanently delete without prompting")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("cloud delete does not accept positional arguments")
	}
	if !*confirmed {
		return fmt.Errorf("cloud delete is permanent; rerun with --yes")
	}
	client, credentials, err := cloudsync.NewClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Delete(ctx); err != nil {
		if !errors.Is(err, cloudsync.ErrNoCloudConfig) {
			return err
		}
	}
	outputJSON(map[string]interface{}{
		"status":  "deleted",
		"email":   credentials.Email,
		"message": "The encrypted cloud backup was deleted. Local devices were not changed.",
	})
	return nil
}

func runCloudLogout(args []string) error {
	flags := flag.NewFlagSet("cloud logout", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() { _, _ = fmt.Fprintln(flags.Output(), "Usage: yokai cloud logout") }
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("cloud logout does not accept positional arguments")
	}
	if err := cloudsync.RemoveCredentials(); err != nil {
		return err
	}
	outputJSON(map[string]string{
		"status":  "logged_out",
		"message": "Signed out on this computer. Your encrypted cloud backup was not deleted.",
	})
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
	fmt.Fprintln(os.Stderr, "Create a passphrase with at least 12 characters and save it in a password manager.")
	fmt.Fprintln(os.Stderr, "Yokai cannot recover a lost cloud passphrase.")
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

func readCloudConfirmation(reader io.Reader, writer io.Writer, prompt string) (bool, error) {
	if _, err := fmt.Fprint(writer, prompt); err != nil {
		return false, fmt.Errorf("writing confirmation prompt: %w", err)
	}
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("reading confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
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
