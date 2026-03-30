package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

// Daemon is the local background service that maintains SSH tunnels,
// polls agents for metrics, and exposes a REST API for the TUI.
type Daemon struct {
	cfg        *config.Config
	tunnels    *TunnelPool
	aggregator *Aggregator
	version    string
	mu         sync.RWMutex
	server     *http.Server
}

// Run starts the daemon and blocks until interrupted.
func Run(version string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	d := &Daemon{
		cfg:     cfg,
		version: version,
	}

	d.tunnels = NewTunnelPool(cfg)
	d.aggregator = NewAggregator(cfg, d.tunnels)

	// Start SSH tunnels for all devices
	d.tunnels.ConnectAll()

	// Start metrics polling
	d.aggregator.Start()

	// HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", d.handleHealth)
	mux.HandleFunc("GET /devices", d.handleDevices)
	mux.HandleFunc("GET /metrics", d.handleMetrics)
	mux.HandleFunc("GET /metrics/{deviceID}", d.handleDeviceMetrics)
	mux.HandleFunc("POST /deploy", d.handleDeploy)
	mux.HandleFunc("POST /containers/{deviceID}/{containerID}/stop", d.handleStopContainer)
	mux.HandleFunc("DELETE /containers/{deviceID}/{containerID}/remove", d.handleRemoveContainer)
	mux.HandleFunc("POST /containers/{deviceID}/{containerID}/restart", d.handleRestartContainer)
	mux.HandleFunc("POST /containers/{deviceID}/{containerID}/test", d.handleTestContainer)
	mux.HandleFunc("GET /logs/{deviceID}/{containerID}", d.handleLogs)
	mux.HandleFunc("GET /images/tags", d.handleImageTags)
	mux.HandleFunc("POST /reload", d.handleReload)

	addr := cfg.Daemon.Listen
	if addr == "" {
		addr = "127.0.0.1:7473"
	}

	d.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("yokai daemon %s listening on %s", version, addr)
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("daemon server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down daemon...")

	d.aggregator.Stop()
	d.tunnels.CloseAll()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.server.Shutdown(shutdownCtx)
}

func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	deviceCount := len(d.cfg.Devices)
	d.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": d.version,
		"devices": deviceCount,
	})
}

func (d *Daemon) handleDevices(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	type deviceStatus struct {
		config.Device
		Online     bool `json:"online"`
		TunnelPort int  `json:"tunnel_port"`
	}

	var devices []deviceStatus
	for _, dev := range d.cfg.Devices {
		ds := deviceStatus{
			Device:     dev,
			Online:     d.tunnels.IsConnected(dev.ID),
			TunnelPort: d.tunnels.LocalPort(dev.ID),
		}
		devices = append(devices, ds)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"devices": devices,
	})
}

func (d *Daemon) handleMetrics(w http.ResponseWriter, r *http.Request) {
	allMetrics := d.aggregator.AllMetrics()
	writeJSON(w, http.StatusOK, allMetrics)
}

func (d *Daemon) handleDeviceMetrics(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	metrics, ok := d.aggregator.DeviceMetrics(deviceID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "device not found",
		})
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (d *Daemon) handleDeploy(w http.ResponseWriter, r *http.Request) {
	var req DeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": err.Error(),
		})
		return
	}

	result, err := d.aggregator.Deploy(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "deploy_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (d *Daemon) handleStopContainer(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	containerID := r.PathValue("containerID")

	err := d.aggregator.StopContainer(deviceID, containerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "stop_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (d *Daemon) handleRemoveContainer(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	containerID := r.PathValue("containerID")

	err := d.aggregator.RemoveContainer(deviceID, containerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "remove_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (d *Daemon) handleRestartContainer(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	containerID := r.PathValue("containerID")

	err := d.aggregator.RestartContainer(deviceID, containerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "restart_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

func (d *Daemon) handleTestContainer(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	containerID := r.PathValue("containerID")

	result, err := d.aggregator.TestContainer(deviceID, containerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "service_test_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (d *Daemon) handleLogs(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	containerID := r.PathValue("containerID")

	// SSE streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "streaming not supported",
		})
		return
	}

	logCh, err := d.aggregator.StreamLogs(deviceID, containerID)
	if err != nil {
		_, _ = fmt.Fprintf(w, "data: {\"error\": %q}\n\n", err.Error()) // Best-effort SSE write; client may disconnect.
		flusher.Flush()
		return
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-logCh:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", line) // Best-effort SSE write; client may disconnect.
			flusher.Flush()
		}
	}
}

func (d *Daemon) handleImageTags(w http.ResponseWriter, r *http.Request) {
	image := r.URL.Query().Get("image")
	if image == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "image query param required",
		})
		return
	}

	tags, err := d.aggregator.FetchImageTags(image)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "fetch_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"image": image,
		"tags":  tags,
	})
}

func (d *Daemon) handleReload(w http.ResponseWriter, r *http.Request) {
	newCfg, err := config.Load()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "reload_failed",
			"message": fmt.Sprintf("loading config: %v", err),
		})
		return
	}

	d.mu.Lock()
	oldDevices := d.cfg.Devices
	d.cfg = newCfg
	d.tunnels.UpdateConfig(newCfg)
	d.aggregator.UpdateConfig(newCfg)
	d.mu.Unlock()

	// Determine which devices were added or removed
	oldSet := make(map[string]struct{}, len(oldDevices))
	for _, dev := range oldDevices {
		oldSet[dev.ID] = struct{}{}
	}
	newSet := make(map[string]struct{}, len(newCfg.Devices))
	for _, dev := range newCfg.Devices {
		newSet[dev.ID] = struct{}{}
	}

	// Close tunnels for removed devices
	for id := range oldSet {
		if _, ok := newSet[id]; !ok {
			d.tunnels.CloseDevice(id)
		}
	}

	// Connect tunnels for new devices
	for _, dev := range newCfg.Devices {
		if _, ok := oldSet[dev.ID]; !ok {
			d.tunnels.ConnectDevice(dev)
		}
	}

	log.Printf("config reloaded: %d devices", len(newCfg.Devices))

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "reloaded",
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "json encode error: %v\n", err)
	}
}
