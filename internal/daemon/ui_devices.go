package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/config"
	sshpkg "github.com/spencerbull/yokai/internal/ssh"
)

type deviceStatus struct {
	config.Device
	Online      bool   `json:"online"`
	TunnelPort  int    `json:"tunnel_port"`
	TunnelError string `json:"tunnel_error,omitempty"`
}

type sshConfigHostRecord struct {
	Alias                 string `json:"alias"`
	Host                  string `json:"host"`
	User                  string `json:"user,omitempty"`
	Port                  int    `json:"port"`
	IdentityFile          string `json:"identity_file,omitempty"`
	IdentityFileEncrypted bool   `json:"identity_file_encrypted"`
}

type deviceUpsertRequest struct {
	ID             string   `json:"id,omitempty"`
	Label          string   `json:"label,omitempty"`
	Host           string   `json:"host"`
	SSHUser        string   `json:"ssh_user,omitempty"`
	SSHKey         string   `json:"ssh_key,omitempty"`
	SSHPort        int      `json:"ssh_port,omitempty"`
	ConnectionType string   `json:"connection_type,omitempty"`
	AgentPort      int      `json:"agent_port,omitempty"`
	AgentToken     string   `json:"agent_token,omitempty"`
	GPUType        string   `json:"gpu_type,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

type deviceTestResult struct {
	DeviceID string `json:"device_id"`
	SSHOK    bool   `json:"ssh_ok"`
	AgentOK  bool   `json:"agent_ok"`
	Version  string `json:"version,omitempty"`
	Message  string `json:"message"`
}

type deviceDeleteResponse struct {
	RemovedDeviceID  string `json:"removed_device_id"`
	RemovedServices  int    `json:"removed_services"`
	CleanupRequested bool   `json:"cleanup_requested"`
}

type deviceUpgradeResult struct {
	DeviceID string `json:"device_id"`
	OK       bool   `json:"ok"`
	Message  string `json:"message"`
}

type bulkDeviceActionResponse struct {
	Results []interface{} `json:"results"`
}

func (d *Daemon) handleSSHConfigHosts(w http.ResponseWriter, r *http.Request) {
	hosts := sshpkg.DiscoverSSHHosts()
	records := make([]sshConfigHostRecord, 0, len(hosts))
	for _, host := range hosts {
		port := 22
		if host.Port != "" {
			fmt.Sscanf(host.Port, "%d", &port)
		}
		records = append(records, sshConfigHostRecord{
			Alias:                 host.Alias,
			Host:                  firstNonEmpty(host.HostName, host.Alias),
			User:                  host.User,
			Port:                  port,
			IdentityFile:          host.IdentityFile,
			IdentityFileEncrypted: host.IdentityFile != "" && sshpkg.IsKeyEncrypted(host.IdentityFile),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"hosts": records})
}

func (d *Daemon) handleDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"devices": d.deviceStatuses(),
	})
}

func (d *Daemon) handleCreateDevice(w http.ResponseWriter, r *http.Request) {
	req, err := decodeDeviceRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": err.Error()})
		return
	}

	d.mu.RLock()
	nextCfg := cloneConfig(d.cfg)
	d.mu.RUnlock()

	device := buildDeviceFromRequest(req, "")
	nextCfg.UpsertDevice(device)
	if err := config.Save(nextCfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config_save_failed", "message": err.Error()})
		return
	}

	d.applyConfigUpdate(nextCfg)
	writeJSON(w, http.StatusCreated, d.deviceStatus(device.ID))
}

func (d *Daemon) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	req, err := decodeDeviceRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": err.Error()})
		return
	}

	d.mu.RLock()
	nextCfg := cloneConfig(d.cfg)
	current := nextCfg.FindDevice(deviceID)
	d.mu.RUnlock()
	if current == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device_not_found", "message": fmt.Sprintf("device %q was not found", deviceID)})
		return
	}

	device := buildDeviceFromRequest(req, current.ID)
	nextCfg.UpsertDevice(device)
	if err := config.Save(nextCfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config_save_failed", "message": err.Error()})
		return
	}

	d.applyConfigUpdate(nextCfg)
	writeJSON(w, http.StatusOK, d.deviceStatus(device.ID))
}

func (d *Daemon) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")

	d.mu.RLock()
	nextCfg := cloneConfig(d.cfg)
	device := nextCfg.FindDevice(deviceID)
	d.mu.RUnlock()
	if device == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device_not_found", "message": fmt.Sprintf("device %q was not found", deviceID)})
		return
	}

	removedServices := nextCfg.RemoveServicesByDevice(deviceID)
	nextCfg.RemoveDevice(deviceID)
	if err := config.Save(nextCfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config_save_failed", "message": err.Error()})
		return
	}

	d.applyConfigUpdate(nextCfg)
	writeJSON(w, http.StatusOK, deviceDeleteResponse{
		RemovedDeviceID:  deviceID,
		RemovedServices:  removedServices,
		CleanupRequested: false,
	})
}

func (d *Daemon) handleTestDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	device := d.lookupDevice(deviceID)
	if device == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device_not_found", "message": fmt.Sprintf("device %q was not found", deviceID)})
		return
	}

	writeJSON(w, http.StatusOK, testSingleDevice(*device))
}

func (d *Daemon) handleUpgradeDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	device := d.lookupDevice(deviceID)
	if device == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device_not_found", "message": fmt.Sprintf("device %q was not found", deviceID)})
		return
	}

	result := upgradeSingleDevice(*device)
	status := http.StatusOK
	if !result.OK {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, result)
}

func (d *Daemon) handleTestAllDevices(w http.ResponseWriter, r *http.Request) {
	devices := d.allDevices()
	results := make([]interface{}, 0, len(devices))
	for _, device := range devices {
		results = append(results, testSingleDevice(device))
	}
	writeJSON(w, http.StatusOK, bulkDeviceActionResponse{Results: results})
}

func (d *Daemon) handleUpgradeAllDevices(w http.ResponseWriter, r *http.Request) {
	devices := d.allDevices()
	results := make([]interface{}, 0, len(devices))
	for _, device := range devices {
		results = append(results, upgradeSingleDevice(device))
	}
	writeJSON(w, http.StatusOK, bulkDeviceActionResponse{Results: results})
}

func (d *Daemon) lookupDevice(deviceID string) *config.Device {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg.FindDevice(deviceID)
}

func (d *Daemon) allDevices() []config.Device {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]config.Device(nil), d.cfg.Devices...)
}

func testSingleDevice(device config.Device) deviceTestResult {
	client, err := sshpkg.Connect(sshpkg.ClientConfig{
		Host:           device.Host,
		Port:           fmt.Sprintf("%d", device.SSHPortOrDefault()),
		User:           device.SSHUser,
		ConnectionType: device.ConnectionType,
		KeyPath:        device.SSHKey,
	})
	if err != nil {
		return deviceTestResult{
			DeviceID: device.ID,
			SSHOK:    false,
			AgentOK:  false,
			Message:  compactSSHTestError(err),
		}
	}
	defer func() { _ = client.Close() }()

	result := deviceTestResult{
		DeviceID: device.ID,
		SSHOK:    true,
		AgentOK:  false,
		Message:  "SSH connected, but agent is not responding",
	}

	healthURL := fmt.Sprintf("http://%s:%d/health", device.Host, device.AgentPort)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(healthURL)
	if err != nil {
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		result.Message = fmt.Sprintf("Agent returned status %d", resp.StatusCode)
		return result
	}

	var payload struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	result.AgentOK = true
	result.Version = payload.Version
	result.Message = "Device online"
	if payload.Version != "" {
		result.Message = fmt.Sprintf("Device online (v%s)", payload.Version)
	}
	return result
}

func upgradeSingleDevice(device config.Device) deviceUpgradeResult {
	client, err := sshpkg.Connect(sshpkg.ClientConfig{
		Host:           device.Host,
		Port:           fmt.Sprintf("%d", device.SSHPortOrDefault()),
		User:           device.SSHUser,
		ConnectionType: device.ConnectionType,
		KeyPath:        device.SSHKey,
	})
	if err != nil {
		return deviceUpgradeResult{DeviceID: device.ID, OK: false, Message: fmt.Sprintf("SSH: %s", compactSSHTestError(err))}
	}
	defer func() { _ = client.Close() }()

	preflight, err := sshpkg.Preflight(client)
	if err != nil {
		return deviceUpgradeResult{DeviceID: device.ID, OK: false, Message: fmt.Sprintf("preflight failed: %s", err.Error())}
	}

	localBinary, err := sshpkg.BuildLocalBinaryForTarget(preflight.KernelOS, preflight.Arch)
	if err != nil {
		return deviceUpgradeResult{DeviceID: device.ID, OK: false, Message: fmt.Sprintf("build failed: %s", err.Error())}
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(localBinary)) }()

	if err := sshpkg.UpgradeAgent(client, localBinary, device.AgentPort); err != nil {
		return deviceUpgradeResult{DeviceID: device.ID, OK: false, Message: err.Error()}
	}

	return deviceUpgradeResult{DeviceID: device.ID, OK: true, Message: "Agent upgraded successfully"}
}

func (d *Daemon) deviceStatuses() []deviceStatus {
	d.mu.RLock()
	devices := append([]config.Device(nil), d.cfg.Devices...)
	d.mu.RUnlock()

	statuses := make([]deviceStatus, 0, len(devices))
	for _, dev := range devices {
		statuses = append(statuses, deviceStatus{
			Device:      dev,
			Online:      d.tunnels.IsConnected(dev.ID),
			TunnelPort:  d.tunnels.LocalPort(dev.ID),
			TunnelError: d.tunnels.LastError(dev.ID),
		})
	}
	return statuses
}

func (d *Daemon) deviceStatus(deviceID string) deviceStatus {
	for _, status := range d.deviceStatuses() {
		if status.ID == deviceID {
			return status
		}
	}
	return deviceStatus{}
}

func decodeDeviceRequest(r *http.Request) (deviceUpsertRequest, error) {
	var req deviceUpsertRequest
	if err := decodeJSONBody(r, &req); err != nil {
		return deviceUpsertRequest{}, err
	}

	return decodeNormalizedDeviceRequest(req)
}

func decodeNormalizedDeviceRequest(req deviceUpsertRequest) (deviceUpsertRequest, error) {

	req.Host = strings.TrimSpace(req.Host)
	req.Label = strings.TrimSpace(req.Label)
	req.SSHUser = strings.TrimSpace(req.SSHUser)
	req.SSHKey = strings.TrimSpace(req.SSHKey)
	req.ConnectionType = strings.TrimSpace(req.ConnectionType)
	req.AgentToken = strings.TrimSpace(req.AgentToken)
	if req.Host == "" {
		return deviceUpsertRequest{}, fmt.Errorf("host is required")
	}
	if req.AgentPort <= 0 {
		req.AgentPort = 7474
	}
	if req.SSHPort <= 0 {
		req.SSHPort = 22
	}
	if req.ConnectionType == "" {
		req.ConnectionType = "manual"
	}
	return req, nil
}

func buildDeviceFromRequest(req deviceUpsertRequest, existingID string) config.Device {
	deviceID := existingID
	if deviceID == "" {
		deviceID = firstNonEmpty(strings.TrimSpace(req.ID), req.Host)
	}

	return config.Device{
		ID:             deviceID,
		Label:          firstNonEmpty(req.Label, req.Host),
		Host:           req.Host,
		SSHUser:        req.SSHUser,
		SSHKey:         req.SSHKey,
		SSHPort:        req.SSHPort,
		ConnectionType: req.ConnectionType,
		AgentPort:      req.AgentPort,
		AgentToken:     req.AgentToken,
		GPUType:        req.GPUType,
		Tags:           append([]string(nil), req.Tags...),
	}
}

func cloneConfig(cfg *config.Config) *config.Config {
	next := *cfg
	next.Devices = append([]config.Device(nil), cfg.Devices...)
	next.Services = append([]config.Service(nil), cfg.Services...)
	return &next
}

func compactSSHTestError(err error) string {
	message := strings.Join(strings.Fields(err.Error()), " ")
	if strings.Contains(message, "no SSH auth methods available") || strings.Contains(message, "unable to authenticate") {
		return message + " (hint: if your key is passphrase-protected, make sure ssh-agent is running and the key is loaded via ssh-add)"
	}
	return message
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
