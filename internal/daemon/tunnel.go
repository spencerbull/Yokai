package daemon

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/ssh"
)

// TunnelPool manages SSH tunnels to remote devices
type TunnelPool struct {
	cfg     *config.Config
	tunnels map[string]*tunnel // keyed by device ID
	mu      sync.RWMutex
}

type tunnel struct {
	deviceID  string
	sshClient *ssh.Client // from internal/ssh
	localPort int         // local port for forwarded agent connection
	connected bool
	lastError string
	cancel    context.CancelFunc
	listener  net.Listener
}

// NewTunnelPool creates a new tunnel pool
func NewTunnelPool(cfg *config.Config) *TunnelPool {
	return &TunnelPool{
		cfg:     cfg,
		tunnels: make(map[string]*tunnel),
	}
}

// ConnectAll establishes SSH tunnels to all configured devices
func (tp *TunnelPool) ConnectAll() {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	for _, device := range tp.cfg.Devices {
		go tp.connectDevice(device)
	}
}

// connectDevice establishes a single device tunnel
func (tp *TunnelPool) connectDevice(device config.Device) {
	tp.mu.Lock()
	if _, exists := tp.tunnels[device.ID]; exists {
		tp.mu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	t := &tunnel{
		deviceID:  device.ID,
		connected: false,
		cancel:    cancel,
	}
	tp.tunnels[device.ID] = t
	tp.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := tp.establishTunnel(t, device); err != nil {
				tp.mu.Lock()
				t.lastError = err.Error()
				tp.mu.Unlock()
				log.Printf("tunnel to %s failed: %v, retrying in %ds", device.ID, err, tp.cfg.Daemon.ReconnectInterval)
				time.Sleep(time.Duration(tp.cfg.Daemon.ReconnectInterval) * time.Second)
				continue
			}

			// Keep connection alive
			tp.keepAlive(t, ctx)

			// If we get here, connection was lost, so retry
			tp.mu.Lock()
			t.connected = false
			tp.mu.Unlock()
		}
	}
}

// establishTunnel creates SSH connection and local port forwarding
func (tp *TunnelPool) establishTunnel(t *tunnel, device config.Device) error {
	// Create SSH connection
	sshConfig := ssh.ClientConfig{
		Host:     device.Host,
		Port:     fmt.Sprintf("%d", device.SSHPortOrDefault()),
		User:     device.SSHUser,
		KeyPath:  device.SSHKey,
		Password: "", // No password fallback for now
	}

	if sshConfig.User == "" {
		sshConfig.User = "root"
	}

	client, err := ssh.Connect(sshConfig)
	if err != nil {
		return fmt.Errorf("SSH connect: %w", err)
	}

	// Create local listener on random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = client.Close() // Best-effort close after listener setup failure.
		return fmt.Errorf("creating local listener: %w", err)
	}

	localPort := listener.Addr().(*net.TCPAddr).Port

	tp.mu.Lock()
	t.sshClient = client
	t.localPort = localPort
	t.listener = listener
	t.connected = true
	t.lastError = ""
	tp.mu.Unlock()

	log.Printf("tunnel established: %s -> localhost:%d -> %s:%d", device.ID, localPort, device.Host, device.AgentPort)

	// Start accepting connections
	go tp.handleConnections(t, device)

	return nil
}

// handleConnections forwards local connections through SSH tunnel
func (tp *TunnelPool) handleConnections(t *tunnel, device config.Device) {
	for {
		localConn, err := t.listener.Accept()
		if err != nil {
			log.Printf("tunnel %s: accept error: %v", device.ID, err)
			return
		}

		go tp.forwardConnection(t, localConn, device)
	}
}

// forwardConnection pipes data between local connection and remote agent
func (tp *TunnelPool) forwardConnection(t *tunnel, localConn net.Conn, device config.Device) {
	defer func() {
		_ = localConn.Close() // Best-effort close; connection may already be closed.
	}()

	agentPort := device.AgentPort
	if agentPort == 0 {
		agentPort = 7474 // default agent port
	}

	remoteAddr := fmt.Sprintf("localhost:%d", agentPort)
	remoteConn, err := t.sshClient.Underlying().Dial("tcp", remoteAddr)
	if err != nil {
		log.Printf("tunnel %s: dial remote %s: %v", device.ID, remoteAddr, err)
		return
	}
	defer func() {
		_ = remoteConn.Close() // Best-effort close; connection may already be closed.
	}()

	// Bidirectional copy
	done := make(chan struct{})

	go func() {
		_, _ = io.Copy(remoteConn, localConn) // Best-effort tunnel stream copy.
		done <- struct{}{}
	}()

	go func() {
		_, _ = io.Copy(localConn, remoteConn) // Best-effort tunnel stream copy.
		done <- struct{}{}
	}()

	<-done
}

// keepAlive sends SSH keepalive requests to prevent tunnel drops
func (tp *TunnelPool) keepAlive(t *tunnel, ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tp.mu.RLock()
			if !t.connected {
				tp.mu.RUnlock()
				return
			}

			// Send keepalive by executing a simple command
			_, err := t.sshClient.Exec("echo keepalive")
			tp.mu.RUnlock()

			if err != nil {
				log.Printf("tunnel %s: keepalive failed: %v", t.deviceID, err)
				tp.mu.Lock()
				t.connected = false
				t.lastError = err.Error()
				tp.mu.Unlock()
				return
			}
		}
	}
}

// UpdateConfig replaces the config pointer (caller holds Daemon.mu).
func (tp *TunnelPool) UpdateConfig(cfg *config.Config) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.cfg = cfg
}

// ConnectDevice starts a tunnel for a single device (public wrapper around connectDevice).
func (tp *TunnelPool) ConnectDevice(device config.Device) {
	go tp.connectDevice(device)
}

// CloseDevice closes the tunnel for a single device.
func (tp *TunnelPool) CloseDevice(deviceID string) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	t, exists := tp.tunnels[deviceID]
	if !exists {
		return
	}

	if t.cancel != nil {
		t.cancel()
	}
	if t.listener != nil {
		_ = t.listener.Close()
	}
	if t.sshClient != nil {
		_ = t.sshClient.Close()
	}
	delete(tp.tunnels, deviceID)
}

// CloseAll closes all SSH connections and TCP listeners
func (tp *TunnelPool) CloseAll() {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	for _, t := range tp.tunnels {
		if t.cancel != nil {
			t.cancel()
		}
		if t.listener != nil {
			_ = t.listener.Close() // Best-effort listener close during shutdown.
		}
		if t.sshClient != nil {
			_ = t.sshClient.Close() // Best-effort SSH close during shutdown.
		}
	}

	tp.tunnels = make(map[string]*tunnel)
}

// IsConnected returns true if the device tunnel is connected
func (tp *TunnelPool) IsConnected(deviceID string) bool {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	t, exists := tp.tunnels[deviceID]
	return exists && t.connected
}

// LocalPort returns the local port for the SSH tunnel to this device's agent
func (tp *TunnelPool) LocalPort(deviceID string) int {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	t, exists := tp.tunnels[deviceID]
	if !exists {
		return 0
	}
	return t.localPort
}

// LastError returns the most recent tunnel error for a device.
func (tp *TunnelPool) LastError(deviceID string) string {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	t, exists := tp.tunnels[deviceID]
	if !exists {
		return ""
	}
	return t.lastError
}

// Reconnect reconnects a specific device tunnel
func (tp *TunnelPool) Reconnect(deviceID string) error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	t, exists := tp.tunnels[deviceID]
	if !exists {
		return fmt.Errorf("device %s not found", deviceID)
	}

	// Cancel existing connection
	if t.cancel != nil {
		t.cancel()
	}
	if t.listener != nil {
		_ = t.listener.Close() // Best-effort listener close during reconnect.
	}
	if t.sshClient != nil {
		_ = t.sshClient.Close() // Best-effort SSH close during reconnect.
	}

	// Find device config
	var device *config.Device
	for _, d := range tp.cfg.Devices {
		if d.ID == deviceID {
			device = &d
			break
		}
	}

	if device == nil {
		return fmt.Errorf("device %s not in config", deviceID)
	}

	// Start new connection
	go tp.connectDevice(*device)
	return nil
}
