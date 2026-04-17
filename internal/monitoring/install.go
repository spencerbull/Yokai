package monitoring

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	assetspkg "github.com/spencerbull/yokai/assets"
)

type remoteClient interface {
	Exec(cmd string) (string, error)
	Upload(localPath, remotePath string) error
}

// RemoteFiles describes the monitoring stack files that should be seeded on the remote host.
type RemoteFiles struct {
	TmpDir          string
	ComposeYAML     string
	PrometheusYAML  string
	AgentToken      string
	DashboardJSON   string
	DatasourceYAML  string
	DashboardYAML   string
}

// SeedRemoteFiles uploads the compose, Prometheus, Grafana provisioning, and secret files.

func SeedRemoteFiles(client remoteClient, files RemoteFiles) (string, error) {
	if files.TmpDir == "" {
		resolvedDir, err := defaultRemoteMonitoringDir(client)
		if err != nil {
			return "", err
		}
		files.TmpDir = resolvedDir
	}
	if files.DashboardJSON == "" {
		files.DashboardJSON = assetspkg.DefaultGrafanaDashboard
	}
	if files.DatasourceYAML == "" {
		files.DatasourceYAML = assetspkg.GrafanaDatasourceProvisioning
	}
	if files.DashboardYAML == "" {
		files.DashboardYAML = assetspkg.GrafanaDashboardProvisioning
	}

	remoteDirs := []string{
		files.TmpDir,
		filepath.Join(files.TmpDir, "grafana", "provisioning", "datasources"),
		filepath.Join(files.TmpDir, "grafana", "provisioning", "dashboards"),
		filepath.Join(files.TmpDir, "grafana", "dashboards"),
		filepath.Join(files.TmpDir, "prometheus", "secrets"),
	}
	if _, err := client.Exec("mkdir -p " + strings.Join(remoteDirs, " ")); err != nil {
		return "", fmt.Errorf("creating monitoring directories: %w", err)
	}

	uploads := map[string]string{
		filepath.Join(files.TmpDir, "docker-compose.yml"):                               files.ComposeYAML,
		filepath.Join(files.TmpDir, "prometheus.yml"):                                   files.PrometheusYAML,
		filepath.Join(files.TmpDir, "grafana", "provisioning", "datasources", "prometheus.yml"): files.DatasourceYAML,
		filepath.Join(files.TmpDir, "grafana", "provisioning", "dashboards", "dashboard.yml"):  files.DashboardYAML,
		filepath.Join(files.TmpDir, "grafana", "dashboards", "gpu-dashboard.json"):               files.DashboardJSON,
	}

	for remotePath, contents := range uploads {
		if err := uploadRemoteFile(client, remotePath, contents); err != nil {
			return "", err
		}
	}

	secretPath := filepath.Join(files.TmpDir, "prometheus", "secrets", "yokai-agent-token")
	if err := writeRemoteSecretFile(client, secretPath, files.AgentToken+"\n"); err != nil {
		return "", err
	}

	return files.TmpDir, nil
}

func defaultRemoteMonitoringDir(client remoteClient) (string, error) {
	out, err := client.Exec(`printf %s "$HOME"`)
	if err != nil {
		return "", fmt.Errorf("resolving remote home directory: %w", err)
	}
	home := strings.TrimSpace(out)
	if home == "" {
		return "", fmt.Errorf("resolving remote home directory: empty HOME")
	}
	return filepath.Join(home, ".local", "share", "yokai", "monitoring"), nil
}

func writeRemoteSecretFile(client remoteClient, remotePath, contents string) error {
	escaped := strings.ReplaceAll(contents, `'`, `"'"'`)
	cmd := fmt.Sprintf(`umask 077 && mkdir -p %s && printf '%%s' '%s' > %s`, filepath.Dir(remotePath), escaped, remotePath)
	if _, err := client.Exec(cmd); err != nil {
		return fmt.Errorf("writing %s: %w", remotePath, err)
	}
	return nil
}

func uploadRemoteFile(client remoteClient, remotePath, contents string) error {
	localPath, err := writeTempFile(contents)
	if err != nil {
		return fmt.Errorf("creating temp upload file for %s: %w", remotePath, err)
	}
	defer func() {
		_ = os.Remove(localPath)
	}()

	if err := client.Upload(localPath, remotePath); err != nil {
		return fmt.Errorf("uploading %s: %w", remotePath, err)
	}

	return nil
}

func writeTempFile(contents string) (string, error) {
	f, err := os.CreateTemp("", "yokai-monitoring-*")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.WriteString(contents); err != nil {
		return "", err
	}

	return f.Name(), nil
}
