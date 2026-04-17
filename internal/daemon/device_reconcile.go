package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

type agentContainersResponse struct {
	Containers []agentContainerRecord `json:"containers"`
}

type agentContainerRecord struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Status string            `json:"status"`
	Ports  map[string]string `json:"ports"`
}

func (d *Daemon) ensureConfiguredServices(device config.Device) (int, error) {
	services := d.servicesForDevice(device.ID)
	if len(services) == 0 {
		return 0, nil
	}

	containers, err := d.listDeviceContainers(device)
	if err != nil {
		return 0, err
	}

	containersByName := make(map[string]agentContainerRecord, len(containers))
	for _, container := range containers {
		containersByName[container.Name] = container
	}

	reconciled := 0
	for _, service := range services {
		expectedName := desiredServiceContainerName(service)
		if existing, ok := containersByName[expectedName]; ok && existing.Status == "running" {
			if err := d.updateServiceContainerID(device.ID, service.ID, firstNonEmpty(existing.ID, service.ContainerID)); err != nil {
				return reconciled, fmt.Errorf("saving service %s: %w", service.ID, err)
			}
			continue
		} else if ok {
			if err := d.removeDeviceContainer(device, firstNonEmpty(existing.ID, existing.Name)); err != nil {
				return reconciled, fmt.Errorf("removing existing container for %s: %w", service.ID, err)
			}
		}

		result, err := d.deployConfiguredService(device, service)
		if err != nil {
			return reconciled, fmt.Errorf("deploying %s: %w", service.ID, err)
		}
		if err := d.updateServiceContainerID(device.ID, service.ID, result.ContainerID); err != nil {
			return reconciled, fmt.Errorf("saving service %s: %w", service.ID, err)
		}
		reconciled++
	}

	return reconciled, nil
}

func (d *Daemon) servicesForDevice(deviceID string) []config.Service {
	d.mu.RLock()
	defer d.mu.RUnlock()

	services := make([]config.Service, 0)
	for _, service := range d.cfg.Services {
		if service.DeviceID == deviceID {
			services = append(services, service)
		}
	}
	return services
}

func (d *Daemon) listDeviceContainers(device config.Device) ([]agentContainerRecord, error) {
	resp, err := d.doDirectAgentJSON(device, http.MethodGet, "/containers", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeAgentError(resp, "listing containers")
	}

	var payload agentContainersResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parsing containers response: %w", err)
	}
	return payload.Containers, nil
}

func (d *Daemon) removeDeviceContainer(device config.Device, idOrName string) error {
	resp, err := d.doDirectAgentJSON(device, http.MethodDelete, "/containers/"+idOrName, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return decodeAgentError(resp, "removing container")
	}
	return nil
}

func (d *Daemon) deployConfiguredService(device config.Device, service config.Service) (*DeployResult, error) {
	request := DeployRequest{
		DeviceID:    device.ID,
		ServiceType: strings.TrimSpace(service.Type),
		Image:       strings.TrimSpace(service.Image),
		Name:        strings.TrimSpace(service.ID),
		Model:       strings.TrimSpace(service.Model),
		Ports:       servicePorts(service),
		Env:         cloneStringMap(service.Env),
		GPUIDs:      inferServiceGPUSelection(service),
		ExtraArgs:   strings.TrimSpace(service.ExtraArgs),
		Volumes:     cloneStringMap(service.Volumes),
		Plugins:     append([]string(nil), service.Plugins...),
		Runtime:     service.Runtime,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshaling deploy request: %w", err)
	}

	resp, err := d.doDirectAgentJSON(device, http.MethodPost, "/containers", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, decodeAgentError(resp, "deploying container")
	}

	var result struct {
		ID          string            `json:"id"`
		ContainerID string            `json:"container_id"`
		Status      string            `json:"status"`
		Ports       map[string]string `json:"ports"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing deploy response: %w", err)
	}

	deployed := &DeployResult{
		ContainerID: firstNonEmpty(strings.TrimSpace(result.ContainerID), strings.TrimSpace(result.ID)),
		Status:      strings.TrimSpace(result.Status),
		Ports:       result.Ports,
	}
	return deployed, nil
}

func (d *Daemon) updateServiceContainerID(deviceID, serviceID, containerID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	nextCfg := cloneConfig(d.cfg)
	updated := false
	for i := range nextCfg.Services {
		if nextCfg.Services[i].DeviceID != deviceID || nextCfg.Services[i].ID != serviceID {
			continue
		}
		nextCfg.Services[i].ContainerID = strings.TrimSpace(containerID)
		updated = true
		break
	}
	if !updated {
		return fmt.Errorf("service %q on device %q not found", serviceID, deviceID)
	}

	if err := config.Save(nextCfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	d.cfg = nextCfg
	return nil
}

func (d *Daemon) updateDeviceMonitoringInstalled(deviceID string, installed bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	nextCfg := cloneConfig(d.cfg)
	updated := false
	for i := range nextCfg.Devices {
		if nextCfg.Devices[i].ID != deviceID {
			continue
		}
		nextCfg.Devices[i].MonitoringInstalled = installed
		updated = true
		break
	}
	if !updated {
		return fmt.Errorf("device %q not found", deviceID)
	}

	if err := config.Save(nextCfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	d.cfg = nextCfg
	return nil
}

func (d *Daemon) doDirectAgentJSON(device config.Device, method, path string, body *bytes.Reader) (*http.Response, error) {
	baseURLs := []string{}
	if localPort := d.tunnels.LocalPort(device.ID); localPort > 0 {
		baseURLs = append(baseURLs, fmt.Sprintf("http://localhost:%d", localPort))
	}
	baseURLs = append(baseURLs, fmt.Sprintf("http://%s", net.JoinHostPort(device.Host, strconv.Itoa(device.AgentPort))))

	client := &http.Client{Timeout: 5 * time.Minute}
	var lastErr error
	for _, baseURL := range baseURLs {
		requestBody := bytes.NewReader(nil)
		if body != nil {
			if _, err := body.Seek(0, 0); err != nil {
				return nil, fmt.Errorf("rewinding request body: %w", err)
			}
			requestBody = body
		}

		url := baseURL + path
		req, err := http.NewRequest(method, url, requestBody)
		if err != nil {
			return nil, fmt.Errorf("building %s %s request: %w", method, path, err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if device.AgentToken != "" {
			req.Header.Set("Authorization", "Bearer "+device.AgentToken)
		}

		resp, err := client.Do(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("%s %s: %w", method, path, lastErr)
}

func containsMonitoringContainers(containers []agentContainerRecord) bool {
	for _, container := range containers {
		name := strings.ToLower(strings.TrimSpace(container.Name))
		image := strings.ToLower(strings.TrimSpace(container.Image))
		if strings.HasPrefix(name, "yokai-mon-") || strings.Contains(image, "grafana") || strings.Contains(image, "prometheus") || strings.Contains(image, "node-exporter") || strings.Contains(image, "dcgm-exporter") {
			return true
		}
	}
	return false
}

func decodeAgentError(resp *http.Response, action string) error {
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil && strings.TrimSpace(payload.Message) != "" {
		if strings.TrimSpace(payload.Error) != "" {
			return fmt.Errorf("%s: %s (%s)", action, payload.Message, payload.Error)
		}
		return fmt.Errorf("%s: %s", action, payload.Message)
	}
	return fmt.Errorf("%s: agent returned %d", action, resp.StatusCode)
}

func servicePorts(service config.Service) map[string]string {
	if service.Port <= 0 {
		return map[string]string{}
	}
	port := strconv.Itoa(service.Port)
	return map[string]string{port: port}
}

func inferServiceGPUSelection(service config.Service) string {
	return strings.TrimSpace(service.GPUIDs)
}

var serviceNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

func desiredServiceContainerName(service config.Service) string {
	name := serviceNameSanitizer.ReplaceAllString(strings.TrimSpace(service.ID), "-")
	name = strings.Trim(name, "-")
	if name == "" {
		name = "unnamed"
	}
	return "yokai-" + name
}
