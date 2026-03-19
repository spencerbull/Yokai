package agent

import (
	"bufio"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
)

// PackageUpdate represents a single upgradable package.
type PackageUpdate struct {
	Name             string `json:"name"`
	CurrentVersion   string `json:"current_version"`
	AvailableVersion string `json:"available_version"`
}

func handlePackages(w http.ResponseWriter, r *http.Request) {
	// Run apt update silently first
	exec.Command("sudo", "apt", "update", "-qq").Run()

	out, err := exec.Command("apt", "list", "--upgradable").Output()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"packages": []PackageUpdate{},
		})
		return
	}

	var packages []PackageUpdate
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "upgradable from") {
			// Format: "package/repo version arch [upgradable from: old-version]"
			parts := strings.SplitN(line, "/", 2)
			if len(parts) < 2 {
				continue
			}
			name := parts[0]

			rest := parts[1]
			fields := strings.Fields(rest)
			available := ""
			if len(fields) >= 2 {
				available = fields[1]
			}

			current := ""
			fromIdx := strings.Index(rest, "upgradable from: ")
			if fromIdx != -1 {
				tail := rest[fromIdx+len("upgradable from: "):]
				current = strings.TrimRight(tail, "]")
			}

			packages = append(packages, PackageUpdate{
				Name:             name,
				CurrentVersion:   current,
				AvailableVersion: available,
			})
		}
	}

	if packages == nil {
		packages = []PackageUpdate{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"packages": packages,
	})
}

func handlePackagesUpgrade(w http.ResponseWriter, r *http.Request) {
	streamCommand(w, r, "sudo", "apt", "upgrade", "-y")
}

// DriverInfo reports GPU driver status.
type DriverInfo struct {
	GPUType          string `json:"gpu_type"`
	CurrentVersion   string `json:"current_version"`
	AvailableVersion string `json:"available_version,omitempty"`
}

func handleDrivers(w http.ResponseWriter, r *http.Request) {
	// Check NVIDIA
	out, err := exec.Command("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader").Output()
	if err != nil {
		writeJSON(w, http.StatusOK, DriverInfo{GPUType: "none"})
		return
	}

	current := strings.TrimSpace(strings.Split(string(out), "\n")[0])

	// Check available version from apt
	available := ""
	aptOut, err := exec.Command("apt-cache", "policy", "nvidia-driver-"+majorVersion(current)).Output()
	if err == nil {
		for _, line := range strings.Split(string(aptOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Candidate:") {
				available = strings.TrimSpace(strings.TrimPrefix(line, "Candidate:"))
				break
			}
		}
	}

	writeJSON(w, http.StatusOK, DriverInfo{
		GPUType:          "nvidia",
		CurrentVersion:   current,
		AvailableVersion: available,
	})
}

func majorVersion(version string) string {
	parts := strings.SplitN(version, ".", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return version
}

func handleDriversUpgrade(w http.ResponseWriter, r *http.Request) {
	// Get current driver major version
	out, _ := exec.Command("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader").Output()
	major := majorVersion(strings.TrimSpace(string(out)))
	streamCommandThenReboot(w, r, "sudo", "apt", "install", "-y", "nvidia-driver-"+major)
}

// FirmwareUpdate represents a single firmware update.
type FirmwareUpdate struct {
	Name      string `json:"name"`
	Current   string `json:"current"`
	Available string `json:"available"`
}

// FirmwareInfo reports BIOS/firmware status.
type FirmwareInfo struct {
	CurrentBIOS      string           `json:"current_bios"`
	Tool             string           `json:"tool"` // "dsu", "fwupd", "none"
	UpdatesAvailable bool             `json:"updates_available"`
	Updates          []FirmwareUpdate `json:"updates"`
}

func handleFirmware(w http.ResponseWriter, r *http.Request) {
	bios := dmidecodeField("bios-version")
	manufacturer := strings.ToLower(dmidecodeField("system-manufacturer"))

	// Check for Dell System Update
	if strings.Contains(manufacturer, "dell") {
		if _, err := exec.LookPath("dsu"); err == nil {
			info := parseDSUInventory(bios)
			writeJSON(w, http.StatusOK, info)
			return
		}
	}

	// Check for fwupd
	if _, err := exec.LookPath("fwupdmgr"); err == nil {
		info := parseFwupdUpdates(bios)
		writeJSON(w, http.StatusOK, info)
		return
	}

	writeJSON(w, http.StatusOK, FirmwareInfo{
		CurrentBIOS: bios,
		Tool:        "none",
		Updates:     []FirmwareUpdate{},
	})
}

func parseDSUInventory(bios string) FirmwareInfo {
	out, err := exec.Command("sudo", "dsu", "--inventory", "--non-interactive").Output()
	info := FirmwareInfo{
		CurrentBIOS: bios,
		Tool:        "dsu",
		Updates:     []FirmwareUpdate{},
	}
	if err != nil {
		return info
	}

	// Parse DSU inventory output for upgradable items
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "Upgradable") {
			fields := strings.Split(line, "|")
			if len(fields) >= 4 {
				info.Updates = append(info.Updates, FirmwareUpdate{
					Name:      strings.TrimSpace(fields[0]),
					Current:   strings.TrimSpace(fields[1]),
					Available: strings.TrimSpace(fields[2]),
				})
			}
		}
	}
	info.UpdatesAvailable = len(info.Updates) > 0
	return info
}

func parseFwupdUpdates(bios string) FirmwareInfo {
	out, err := exec.Command("fwupdmgr", "get-updates", "--json").Output()
	info := FirmwareInfo{
		CurrentBIOS: bios,
		Tool:        "fwupd",
		Updates:     []FirmwareUpdate{},
	}
	if err != nil {
		return info
	}

	// Simple parsing — fwupd JSON output contains Devices array
	if strings.Contains(string(out), "No updates available") {
		return info
	}

	// Basic line parsing for update names
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Version") && strings.Contains(line, "->") {
			info.UpdatesAvailable = true
		}
	}
	return info
}

func handleFirmwareUpgrade(w http.ResponseWriter, r *http.Request) {
	manufacturer := strings.ToLower(dmidecodeField("system-manufacturer"))

	if strings.Contains(manufacturer, "dell") {
		streamCommandThenReboot(w, r, "sudo", "dsu", "--non-interactive", "--apply-upgrades")
		return
	}

	streamCommandThenReboot(w, r, "sudo", "fwupdmgr", "update", "-y")
}

// streamCommand runs a command and streams output as SSE.
func streamCommand(w http.ResponseWriter, r *http.Request, name string, args ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	cmd := exec.CommandContext(r.Context(), name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(w, "data: error: %s\n\n", err.Error())
		flusher.Flush()
		return
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(w, "data: error: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		fmt.Fprintf(w, "data: %s\n\n", scanner.Text())
		flusher.Flush()
	}

	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
	} else {
		fmt.Fprintf(w, "event: done\ndata: completed\n\n")
	}
	flusher.Flush()
}

// streamCommandThenReboot runs a command, streams output, and reboots on success.
func streamCommandThenReboot(w http.ResponseWriter, r *http.Request, name string, args ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	cmd := exec.CommandContext(r.Context(), name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(w, "data: error: %s\n\n", err.Error())
		flusher.Flush()
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(w, "data: error: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		fmt.Fprintf(w, "data: %s\n\n", scanner.Text())
		flusher.Flush()
	}

	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Update succeeded — reboot
	fmt.Fprintf(w, "data: Update complete. Rebooting in 5 seconds...\n\n")
	flusher.Flush()

	fmt.Fprintf(w, "event: done\ndata: rebooting\n\n")
	flusher.Flush()

	// Schedule reboot in 5 seconds so the SSE response finishes first
	go func() {
		exec.Command("sleep", "5").Run()
		exec.Command("sudo", "reboot").Run()
	}()
}
