package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/docker"
	sshpkg "github.com/spencerbull/yokai/internal/ssh"
)

type bootstrapDeviceRequest struct {
	deviceUpsertRequest
	SSHPassword       string `json:"ssh_password,omitempty"`
	SSHKeyPassphrase  string `json:"ssh_key_passphrase,omitempty"`
	InstallMonitoring bool   `json:"install_monitoring,omitempty"`
}

type bootstrapDeviceResponse struct {
	Device              deviceStatus            `json:"device"`
	Preflight           *sshpkg.PreflightResult `json:"preflight,omitempty"`
	AgentToken          string                  `json:"agent_token"`
	InstallMonitoring   bool                    `json:"install_monitoring"`
	MonitoringInstalled bool                    `json:"monitoring_installed"`
	Message             string                  `json:"message"`
}

func (d *Daemon) handleBootstrapDevice(w http.ResponseWriter, r *http.Request) {
	req, err := decodeBootstrapRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": err.Error()})
		return
	}

	client, err := sshpkg.Connect(sshpkg.ClientConfig{
		Host:           req.Host,
		Port:           fmt.Sprintf("%d", req.SSHPort),
		User:           req.SSHUser,
		ConnectionType: req.ConnectionType,
		KeyPath:        req.SSHKey,
		KeyPassphrase:  req.SSHKeyPassphrase,
		Password:       req.SSHPassword,
	})
	if err != nil {
		var authErr *sshpkg.TailscaleAuthError
		if errors.As(err, &authErr) {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error":   "tailscale_auth_required",
				"message": fmt.Sprintf("tailscale ssh requires browser authentication: %s", authErr.URL),
			})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ssh_connect_failed", "message": err.Error()})
		return
	}
	defer func() { _ = client.Close() }()

	preflight, err := sshpkg.Preflight(client)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "preflight_failed", "message": err.Error()})
		return
	}
	if !preflight.DockerInstalled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "docker_missing", "message": "docker is not installed on the remote device"})
		return
	}

	agentToken := req.AgentToken
	if agentToken == "" {
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_generation_failed", "message": err.Error()})
			return
		}
		agentToken = hex.EncodeToString(tokenBytes)
	}

	binaryPath, err := sshpkg.BuildLocalBinaryForTarget(preflight.KernelOS, preflight.Arch)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "build_failed", "message": err.Error()})
		return
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(binaryPath)) }()

	if err := sshpkg.DeployAgent(client, binaryPath, agentToken); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "agent_deploy_failed", "message": err.Error()})
		return
	}

	monitoringInstalled := false
	if req.InstallMonitoring {
		if err := deployMonitoringStack(client, req.Host, req.AgentPort, preflight); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "monitoring_deploy_failed", "message": err.Error()})
			return
		}
		monitoringInstalled = true
	}

	nextCfg := cloneConfigCurrent(d)
	device := buildDeviceFromRequest(req.deviceUpsertRequest, req.ID)
	device.AgentToken = agentToken
	device.AgentPort = req.AgentPort
	if preflight.GPUDetected {
		device.GPUType = "nvidia"
	}
	nextCfg.UpsertDevice(device)
	if err := config.Save(nextCfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config_save_failed", "message": err.Error()})
		return
	}

	d.applyConfigUpdate(nextCfg)
	message := fmt.Sprintf("Bootstrapped %s and deployed the Yokai agent", device.Label)
	if monitoringInstalled {
		message += " plus monitoring"
	}
	writeJSON(w, http.StatusCreated, bootstrapDeviceResponse{
		Device:              d.deviceStatus(device.ID),
		Preflight:           preflight,
		AgentToken:          agentToken,
		InstallMonitoring:   req.InstallMonitoring,
		MonitoringInstalled: monitoringInstalled,
		Message:             message,
	})
}

func decodeBootstrapRequest(r *http.Request) (bootstrapDeviceRequest, error) {
	var req bootstrapDeviceRequest
	if err := decodeJSONBody(r, &req); err != nil {
		return bootstrapDeviceRequest{}, err
	}

	deviceReq, err := decodeNormalizedDeviceRequest(req.deviceUpsertRequest)
	if err != nil {
		return bootstrapDeviceRequest{}, err
	}
	req.deviceUpsertRequest = deviceReq
	return req, nil
}

func cloneConfigCurrent(d *Daemon) *config.Config {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return cloneConfig(d.cfg)
}

func deployMonitoringStack(client *sshpkg.Client, host string, agentPort int, preflight *sshpkg.PreflightResult) error {
	monitoringCfg := docker.MonitoringConfig{
		AgentHost:      host,
		AgentPort:      agentPort,
		PrometheusPort: 9090,
		GrafanaPort:    3001,
		HasNvidiaGPU:   preflight != nil && preflight.GPUDetected,
	}

	composeYAML := docker.GenerateMonitoringCompose(monitoringCfg)
	prometheusYAML := docker.GeneratePrometheusConfig(monitoringCfg)

	tmpDir := "/tmp/yokai-monitoring"
	if _, err := client.Exec(fmt.Sprintf("mkdir -p %s", tmpDir)); err != nil {
		return fmt.Errorf("creating monitoring dir: %w", err)
	}

	writeComposeCmd := fmt.Sprintf(`cat > %s/docker-compose.yml << 'EOF'
%s
EOF`, tmpDir, composeYAML)
	if _, err := client.Exec(writeComposeCmd); err != nil {
		return fmt.Errorf("writing compose file: %w", err)
	}

	writePrometheusCmd := fmt.Sprintf(`cat > %s/prometheus.yml << 'EOF'
%s
EOF`, tmpDir, prometheusYAML)
	if _, err := client.Exec(writePrometheusCmd); err != nil {
		return fmt.Errorf("writing prometheus config: %w", err)
	}

	mkdirCmd := fmt.Sprintf("mkdir -p %s/grafana/provisioning %s/grafana/dashboards", tmpDir, tmpDir)
	if _, err := client.Exec(mkdirCmd); err != nil {
		return fmt.Errorf("creating grafana dirs: %w", err)
	}

	pullCmd := fmt.Sprintf("cd %s && docker compose pull 2>&1", tmpDir)
	if out, err := client.Exec(pullCmd); err != nil {
		return fmt.Errorf("pulling monitoring images: %w — stderr: %s", err, out)
	}

	deployCmd := fmt.Sprintf("cd %s && docker compose up -d 2>&1", tmpDir)
	if out, err := client.Exec(deployCmd); err != nil {
		time.Sleep(3 * time.Second)
		if out2, err2 := client.Exec(deployCmd); err2 != nil {
			return fmt.Errorf("starting monitoring stack: %w — stderr: %s %s", err2, out, out2)
		}
	}

	return nil
}
