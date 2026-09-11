package daemon

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spencerbull/yokai/internal/config"
)

func TestHandleHealthReportsDaemonPID(t *testing.T) {
	d := &Daemon{cfg: config.DefaultConfig(), version: "test"}
	request := httptest.NewRequest("GET", "/health", nil)
	response := httptest.NewRecorder()

	d.handleHealth(response, request)

	var body struct {
		Status string `json:"status"`
		PID    int    `json:"pid"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("health status = %q, want ok", body.Status)
	}
	if body.PID != os.Getpid() {
		t.Fatalf("health pid = %d, want %d", body.PID, os.Getpid())
	}
}
