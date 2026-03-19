package agent

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// HardwareInfo describes the physical hardware of the machine.
type HardwareInfo struct {
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Serial       string `json:"serial"`
	BIOSVersion  string `json:"bios_version"`
	OS           string `json:"os"`
}

func handleHardware(w http.ResponseWriter, r *http.Request) {
	info := HardwareInfo{
		Manufacturer: dmidecodeField("system-manufacturer"),
		Model:        dmidecodeField("system-product-name"),
		Serial:       dmidecodeField("system-serial-number"),
		BIOSVersion:  dmidecodeField("bios-version"),
		OS:           getOSPrettyName(),
	}
	writeJSON(w, http.StatusOK, info)
}

func dmidecodeField(field string) string {
	out, err := exec.Command("sudo", "dmidecode", "-s", field).Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func getOSPrettyName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			val := strings.TrimPrefix(line, "PRETTY_NAME=")
			val = strings.Trim(val, `"`)
			return val
		}
	}
	return "unknown"
}

// SudoCheck reports whether the agent can run sudo commands.
type SudoCheck struct {
	HasSudo bool `json:"has_sudo"`
}

func handleSudoCheck(w http.ResponseWriter, r *http.Request) {
	err := exec.Command("sudo", "-n", "true").Run()
	writeJSON(w, http.StatusOK, SudoCheck{HasSudo: err == nil})
}

