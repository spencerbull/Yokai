package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// handleDeviceHardware proxies hardware info request to the device agent.
func (d *Daemon) handleDeviceHardware(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	d.proxyAgentGet(w, deviceID, "/hardware")
}

// handleDeviceSudoCheck proxies sudo check to the device agent.
func (d *Daemon) handleDeviceSudoCheck(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	d.proxyAgentGet(w, deviceID, "/sudo-check")
}

// handleDevicePackages proxies package list to the device agent.
func (d *Daemon) handleDevicePackages(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	d.proxyAgentGet(w, deviceID, "/packages")
}

// handleDevicePackagesUpgrade proxies package upgrade SSE stream.
func (d *Daemon) handleDevicePackagesUpgrade(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	d.proxyAgentSSE(w, r, deviceID, "/packages/upgrade")
}

// handleDeviceDrivers proxies driver info to the device agent.
func (d *Daemon) handleDeviceDrivers(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	d.proxyAgentGet(w, deviceID, "/drivers")
}

// handleDeviceDriversUpgrade proxies driver upgrade SSE stream.
func (d *Daemon) handleDeviceDriversUpgrade(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	d.proxyAgentSSE(w, r, deviceID, "/drivers/upgrade")
}

// handleDeviceFirmware proxies firmware info to the device agent.
func (d *Daemon) handleDeviceFirmware(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	d.proxyAgentGet(w, deviceID, "/firmware")
}

// handleDeviceFirmwareUpgrade proxies firmware upgrade SSE stream.
func (d *Daemon) handleDeviceFirmwareUpgrade(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	d.proxyAgentSSE(w, r, deviceID, "/firmware/upgrade")
}

// handleDeviceAgentUpgrade triggers agent self-update on a device.
func (d *Daemon) handleDeviceAgentUpgrade(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")

	localPort := d.tunnels.LocalPort(deviceID)
	if localPort == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "device_offline",
		})
		return
	}

	// The agent upgrades itself via the yokai upgrade command
	// We trigger it by calling the agent's system endpoint
	url := fmt.Sprintf("http://localhost:%d/upgrade", localPort)
	resp, err := d.aggregator.agentDo("POST", url, deviceID, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "agent_error",
			"message": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	writeJSON(w, resp.StatusCode, result)
}

// proxyAgentGet is a helper that forwards a GET request to a device agent.
func (d *Daemon) proxyAgentGet(w http.ResponseWriter, deviceID, path string) {
	localPort := d.tunnels.LocalPort(deviceID)
	if localPort == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "device_offline",
		})
		return
	}

	url := fmt.Sprintf("http://localhost:%d%s", localPort, path)
	resp, err := d.aggregator.agentDo("GET", url, deviceID, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "agent_error",
			"message": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// proxyAgentSSE forwards an SSE stream from the agent to the client.
func (d *Daemon) proxyAgentSSE(w http.ResponseWriter, r *http.Request, deviceID, path string) {
	localPort := d.tunnels.LocalPort(deviceID)
	if localPort == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "device_offline",
		})
		return
	}

	url := fmt.Sprintf("http://localhost:%d%s", localPort, path)
	req, err := d.aggregator.agentRequest("GET", url, deviceID, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "request_error",
			"message": err.Error(),
		})
		return
	}

	// Use a client with no timeout for streaming
	client := &http.Client{}
	resp, err := client.Do(req.WithContext(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "agent_error",
			"message": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Forward SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Stream response body directly
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			break
		}
	}
}
