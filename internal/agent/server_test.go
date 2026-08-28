package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/deployments"
)

// requireTestAuth creates a test-specific auth middleware that doesn't use global state
func requireTestAuth(expectedToken string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Skip auth if no token is configured
			if expectedToken == "" {
				next(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "missing_auth", "Authorization header required")
				return
			}

			const bearerPrefix = "Bearer "
			if !strings.HasPrefix(authHeader, bearerPrefix) {
				writeError(w, http.StatusUnauthorized, "invalid_auth", "Bearer token required")
				return
			}

			token := strings.TrimPrefix(authHeader, bearerPrefix)
			if token != expectedToken {
				writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid bearer token")
				return
			}

			next(w, r)
		}
	}
}

// setupTestServer creates a test server with routes but without starting it.
func setupTestServer(version string, token string) *http.ServeMux {
	// Create test-specific auth middleware instead of writing to global authToken
	requireAuth := requireTestAuth(token)

	mux := http.NewServeMux()

	// Health endpoint (no auth required)
	mux.HandleFunc("GET /health", handleHealth(version))

	// Protected endpoints
	mux.HandleFunc("GET /system/info", requireAuth(handleSystemInfo(version)))
	mux.HandleFunc("GET /metrics", requireAuth(handleMetrics))
	mux.HandleFunc("GET /metrics/prometheus", requireAuth(handlePrometheusMetrics))
	mux.HandleFunc("GET /containers", requireAuth(handleContainers))
	mux.HandleFunc("POST /containers", requireAuth(handleContainerDeploy))
	mux.HandleFunc("POST /deployments/preflight", requireAuth(handleDeploymentPreflight))

	return mux
}

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()

	mux := setupTestServer("test-version", "")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected JSON content type, got %s", w.Header().Get("Content-Type"))
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	expectedFields := []string{"status", "version", "uptime_seconds", "hostname", "capabilities"}
	for _, field := range expectedFields {
		if _, exists := response[field]; !exists {
			t.Errorf("expected field %s in response", field)
		}
	}

	if response["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", response["status"])
	}

	if response["version"] != "test-version" {
		t.Errorf("expected version 'test-version', got %v", response["version"])
	}
	capabilities, ok := response["capabilities"].([]interface{})
	if !ok || len(capabilities) != len(AgentCapabilities) {
		t.Fatalf("unexpected capabilities: %#v", response["capabilities"])
	}
}

func TestSystemInfoEndpoint(t *testing.T) {
	t.Parallel()

	mux := setupTestServer("test-version", "")

	req := httptest.NewRequest("GET", "/system/info", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	expectedFields := []string{"hostname", "os", "kernel", "arch", "cpu", "gpus", "docker", "ram", "disk", "version"}
	for _, field := range expectedFields {
		if _, exists := response[field]; !exists {
			t.Errorf("expected field %s in response", field)
		}
	}

	if response["version"] != "test-version" {
		t.Errorf("expected version 'test-version', got %v", response["version"])
	}

	// Check CPU info structure
	if cpuInfo, ok := response["cpu"].(map[string]interface{}); ok {
		if _, exists := cpuInfo["cores"]; !exists {
			t.Error("expected 'cores' field in CPU info")
		}
	} else {
		t.Error("expected CPU info to be an object")
	}
}

func TestMetricsEndpoint(t *testing.T) {
	t.Parallel()

	mux := setupTestServer("test-version", "")

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response SystemMetrics
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	// Check that timestamp is set and recent
	if response.Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}

	// Check structure exists (values may be zero on some systems)
	_ = response.CPU.Percent
	_ = response.RAM.TotalMB
	_ = response.Swap.TotalMB
	_ = response.Disk.TotalGB
}

func TestRenderPrometheusMetricsIncludesLLMAndHostSeries(t *testing.T) {
	t.Parallel()

	metrics := &SystemMetrics{
		CPU:  CPUMetrics{Percent: 42.5},
		RAM:  RAMMetrics{UsedMB: 1024, TotalMB: 8192, Percent: 12.5},
		Disk: DiskMetrics{FreeGB: 300},
		GPUs: []GPUMetrics{{Index: 0, Name: "RTX 4090", UtilPercent: 87, VRAMUsedMB: 20480, VRAMTotalMB: 24576, TempC: 70, PowerDrawW: 310, PowerLimitW: 450}},
	}
	containers := []Container{{
		Name:   "yokai-vllm-llama31-8b",
		Image:  "vllm/vllm-openai:latest",
		Status: "running",
		VLLMMetrics: &VLLMMetrics{
			Model:                   "meta-llama/Llama-3.1-8B-Instruct",
			PromptTokPerSec:         118.2,
			GenerationTokPerSec:     35.5,
			RequestsRunning:         3,
			RequestsWaiting:         1,
			PromptTokensTotal:       1200,
			GenerationTokensTotal:   900,
			CachedPromptTokensTotal: 300,
			TTFTBuckets: map[string]float64{
				"0.1":  10,
				"+Inf": 12,
			},
			TTFTSum:                  1.23,
			TTFTCount:                12,
			HasPromptTokPerSec:       true,
			HasGenerationTokPerSec:   true,
			HasRequestsRunning:       true,
			HasRequestsWaiting:       true,
			HasPromptTokensTotal:     true,
			HasGenerationTokensTotal: true,
			HasCachedPromptTokens:    true,
			HasTTFT:                  true,
		},
	}}

	body := renderPrometheusMetrics(metrics, containers)

	checks := []string{
		"yokai_cpu_percent 42.5",
		`yokai_service_up{backend="vllm",model="meta-llama/Llama-3.1-8B-Instruct",service="vllm-llama31-8b"} 1`,
		`yokai_llm_prefill_tokens_per_second{backend="vllm",model="meta-llama/Llama-3.1-8B-Instruct",service="vllm-llama31-8b"} 118.2`,
		`yokai_llm_decode_tokens_per_second{backend="vllm",model="meta-llama/Llama-3.1-8B-Instruct",service="vllm-llama31-8b"} 35.5`,
		`yokai_llm_requests_in_flight{backend="vllm",model="meta-llama/Llama-3.1-8B-Instruct",service="vllm-llama31-8b"} 3`,
		`yokai_llm_prompt_tokens_total{backend="vllm",model="meta-llama/Llama-3.1-8B-Instruct",service="vllm-llama31-8b"} 1200`,
		`yokai_llm_generated_tokens_total{backend="vllm",model="meta-llama/Llama-3.1-8B-Instruct",service="vllm-llama31-8b"} 900`,
		`yokai_llm_cached_prompt_tokens_total{backend="vllm",model="meta-llama/Llama-3.1-8B-Instruct",service="vllm-llama31-8b"} 300`,
		`yokai_llm_ttft_seconds_bucket{backend="vllm",le="0.1",model="meta-llama/Llama-3.1-8B-Instruct",service="vllm-llama31-8b"} 10`,
		`yokai_gpu_power_draw_watts{gpu="0",name="RTX 4090"} 310`,
	}

	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Fatalf("expected Prometheus output to contain %q\n%s", check, body)
		}
	}
}

func TestContainersEndpoint(t *testing.T) {
	t.Parallel()

	mux := setupTestServer("test-version", "")

	req := httptest.NewRequest("GET", "/containers", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if _, exists := response["containers"]; !exists {
		t.Error("expected 'containers' field in response")
	}

	// Containers field should be a slice (possibly empty)
	// Note: On systems without Docker or yokai containers, this might be nil
	if containers, ok := response["containers"]; ok {
		if containers == nil {
			t.Log("containers is nil (expected on systems without Docker)")
		} else if containerSlice, isSlice := containers.([]interface{}); isSlice {
			t.Logf("Found %d containers", len(containerSlice))
		} else {
			t.Error("containers field should be an array or nil")
		}
	} else {
		t.Error("expected 'containers' field in response")
	}
}

func TestAuthMiddlewareNoAuth(t *testing.T) {
	t.Parallel()

	// Test with no auth token configured (empty string)
	mux := setupTestServer("test-version", "")

	// Request without auth header should pass
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 when no auth configured, got %d", w.Code)
	}
}

func TestAuthMiddlewareWithAuth(t *testing.T) {
	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "no auth header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid auth format",
			authHeader:     "InvalidFormat token123",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "wrong token",
			authHeader:     "Bearer wrongtoken",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "correct token",
			authHeader:     "Bearer test-token-123",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up server with auth token
			mux := setupTestServer("test-version", "test-token-123")

			// Set global auth token for this test
			originalToken := authToken
			authToken = "test-token-123"
			defer func() { authToken = originalToken }()

			req := httptest.NewRequest("GET", "/metrics", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusUnauthorized {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}

				if _, exists := response["error"]; !exists {
					t.Error("expected 'error' field in unauthorized response")
				}
				if _, exists := response["message"]; !exists {
					t.Error("expected 'message' field in unauthorized response")
				}
			}
		})
	}
}

func TestDeployEndpointBadRequest(t *testing.T) {
	tests := []struct {
		name         string
		requestBody  string
		expectedCode int
		expectedErr  string
	}{
		{
			name:         "invalid json",
			requestBody:  `{"invalid": json}`,
			expectedCode: http.StatusBadRequest,
			expectedErr:  "invalid_json",
		},
		{
			name:         "missing image",
			requestBody:  `{"name": "test-container"}`,
			expectedCode: http.StatusBadRequest,
			expectedErr:  "missing_image",
		},
		{
			name:         "missing name",
			requestBody:  `{"image": "nginx:latest"}`,
			expectedCode: http.StatusBadRequest,
			expectedErr:  "missing_name",
		},
		{
			name:         "empty request",
			requestBody:  `{}`,
			expectedCode: http.StatusBadRequest,
			expectedErr:  "missing_image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mux := setupTestServer("test-version", "")

			req := httptest.NewRequest("POST", "/containers", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected status %d, got %d", tt.expectedCode, w.Code)
			}

			var response map[string]interface{}
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}

			if errorCode, exists := response["error"]; !exists || errorCode != tt.expectedErr {
				t.Errorf("expected error code '%s', got %v", tt.expectedErr, errorCode)
			}
		})
	}
}

func TestCoordinatedLaunchBarrierPrecedesImageInspection(t *testing.T) {
	binDir := t.TempDir()
	markerPath := filepath.Join(binDir, "inspect-started")
	releasePath := filepath.Join(binDir, "release-inspect")
	dockerPath := filepath.Join(binDir, "docker")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "inspect" ] && [ "$2" = "test-image" ]; then
  : > %q
  while [ ! -f %q ]; do sleep 0.01; done
  printf '%%s\n' %q
  exit 0
fi
if [ "$1" = "manifest" ]; then
  printf '%%s\n' '{"schemaVersion":2,"architecture":"%s","os":"linux"}'
  exit 0
fi
if [ "$1" = "run" ]; then
  printf '%%064d\n' 0
  exit 0
fi
exit 1
`, markerPath, releasePath, runtime.GOARCH, runtime.GOARCH)
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	body, err := json.Marshal(ContainerRequest{
		Image: "test-image", Name: "yokai-deployment-dep-barrier-g1-head", SkipPull: true,
		Labels: map[string]string{
			LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelDeploymentID: "dep-barrier", LabelGeneration: "1", LabelRole: "head",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/containers", bytes.NewReader(body))
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handleContainerDeploy(response, request)
		close(done)
	}()
	defer func() { _ = os.WriteFile(releasePath, []byte("release"), 0600) }()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(markerPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("image inspection did not start")
		}
		time.Sleep(time.Millisecond)
	}
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 20*time.Millisecond)
	waitErr := coordinatedCandidateLaunches.wait(waitCtx, "yokai-deployment-dep-barrier-g1-head")
	cancelWait()
	if !errors.Is(waitErr, context.DeadlineExceeded) {
		t.Fatalf("rollback deletion did not wait behind pre-launch image inspection: %v", waitErr)
	}
	if err := os.WriteFile(releasePath, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("coordinated launch handler did not finish")
	}
	if response.Code != http.StatusCreated {
		t.Fatalf("coordinated launch failed after barrier release: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCoordinatedLaunchManifestTimeoutReleasesRollbackBarrier(t *testing.T) {
	const launchTimeout = 200 * time.Millisecond
	const testRollbackBudget = 2 * time.Second
	if testRollbackBudget >= deployments.DefaultCandidateLaunchRPCTimeout {
		t.Fatalf("test rollback budget %s must remain below production RPC budget %s", testRollbackBudget, deployments.DefaultCandidateLaunchRPCTimeout)
	}

	binDir := t.TempDir()
	markerPath := filepath.Join(binDir, "manifest-started")
	dockerPath := filepath.Join(binDir, "docker")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = manifest ] && [ "$2" = inspect ] && [ "$3" = test-image ]; then
  : > %q
  exec sleep 30
fi
if [ "$1" = inspect ] && [ "$2" = test-image ]; then
  printf '%%s\n' %q
  exit 0
fi
exit 1
`, markerPath, runtime.GOARCH)
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	body, err := json.Marshal(ContainerRequest{
		Image: "test-image", Name: "yokai-deployment-dep-manifest-timeout-g1-head", SkipPull: true,
		Labels: map[string]string{
			LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelDeploymentID: "dep-manifest-timeout", LabelGeneration: "1", LabelRole: "head",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/containers", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handlerDone := make(chan struct{})
	started := time.Now()
	go func() {
		handleContainerDeployWithLaunchTimeout(response, request, launchTimeout)
		close(handlerDone)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(markerPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("context-bound manifest inspect did not start")
		}
		time.Sleep(time.Millisecond)
	}

	blockedCtx, cancelBlocked := context.WithTimeout(context.Background(), 20*time.Millisecond)
	blockedErr := coordinatedCandidateLaunches.wait(blockedCtx, "yokai-deployment-dep-manifest-timeout-g1-head")
	cancelBlocked()
	if !errors.Is(blockedErr, context.DeadlineExceeded) {
		t.Fatalf("rollback barrier did not cover manifest inspection: %v", blockedErr)
	}

	rollbackCtx, cancelRollback := context.WithTimeout(context.Background(), testRollbackBudget)
	rollbackErr := coordinatedCandidateLaunches.wait(rollbackCtx, "yokai-deployment-dep-manifest-timeout-g1-head")
	cancelRollback()
	if rollbackErr != nil {
		t.Fatalf("manifest timeout did not release rollback barrier: %v", rollbackErr)
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("coordinated launch handler did not return after manifest cancellation")
	}
	if elapsed := time.Since(started); elapsed >= testRollbackBudget {
		t.Fatalf("manifest process and barrier exceeded bounded test budget: %s", elapsed)
	}
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "deploy_failed") {
		t.Fatalf("manifest cancellation returned unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestManagedMemberLogTailEndpointIsBoundedSanitizedAndNonFollowing(t *testing.T) {
	const sentinel = "exact-request-key"
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/bin/sh
if [ "$1" = inspect ]; then
  printf '%s\n' '"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"'
  printf '%s\n' '"/candidate-worker"'
  printf '%s\n' '{"io.yokai.managed":"true","io.yokai.ownership":"managed","io.yokai.deployment.id":"dep-test","io.yokai.deployment.generation":"1","io.yokai.deployment.role":"worker"}'
  exit 0
fi
if [ "$1" = logs ] && [ "$2" = --tail ] && [ "$3" = 2000 ] && [ "$4" = candidate-worker ] && [ -z "$5" ]; then
  printf '%s\n' 'rank 1 scheduler exception exact-request-key --api-key=generic-secret'
  exit 0
fi
exit 9
`
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	request := httptest.NewRequest(http.MethodPost, "/deployments/dep-test/members/candidate-worker/logs/tail?generation=1&role=worker&name=candidate-worker", strings.NewReader(`{"redact":"`+sentinel+`"}`))
	request.SetPathValue("deploymentID", "dep-test")
	request.SetPathValue("id", "candidate-worker")
	recorder := httptest.NewRecorder()
	handleDeploymentMemberLogTail(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("log capture failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "rank 1 scheduler exception") || strings.Contains(body, sentinel) || strings.Contains(body, "generic-secret") {
		t.Fatalf("unsafe or incomplete log response: %s", body)
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	testData := map[string]interface{}{
		"message": "test",
		"number":  42,
		"boolean": true,
	}

	writeJSON(w, http.StatusCreated, testData)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected JSON content type, got %s", w.Header().Get("Content-Type"))
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if response["message"] != "test" {
		t.Errorf("expected message 'test', got %v", response["message"])
	}
	if response["number"].(float64) != 42 {
		t.Errorf("expected number 42, got %v", response["number"])
	}
	if response["boolean"] != true {
		t.Errorf("expected boolean true, got %v", response["boolean"])
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()

	writeError(w, http.StatusBadRequest, "test_error", "This is a test error")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected JSON content type, got %s", w.Header().Get("Content-Type"))
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if response["error"] != "test_error" {
		t.Errorf("expected error 'test_error', got %v", response["error"])
	}
	if response["message"] != "This is a test error" {
		t.Errorf("expected message 'This is a test error', got %v", response["message"])
	}
}

func TestSystemInfoHelpers(t *testing.T) {
	t.Parallel()

	// These functions read from system files, so we just verify they don't panic
	// and return reasonable values

	t.Run("getOSInfo", func(t *testing.T) {
		osInfo := getOSInfo()
		if osInfo == "" {
			t.Error("expected non-empty OS info")
		}
	})

	t.Run("getKernelVersion", func(t *testing.T) {
		kernel := getKernelVersion()
		if kernel == "" {
			t.Error("expected non-empty kernel version")
		}
	})

	t.Run("getCPUInfo", func(t *testing.T) {
		cpuInfo := getCPUInfo()
		if cores, exists := cpuInfo["cores"]; !exists || cores.(int) <= 0 {
			t.Error("expected positive number of CPU cores")
		}
		if _, exists := cpuInfo["model"]; !exists {
			t.Error("expected CPU model field")
		}
	})

	t.Run("getGPUInfo", func(t *testing.T) {
		gpuInfo := getGPUInfo()
		// GPU info can be empty on systems without NVIDIA GPUs
		t.Logf("Found %d GPUs", len(gpuInfo))
	})

	t.Run("getDockerInfo", func(t *testing.T) {
		dockerInfo := getDockerInfo()
		if available, exists := dockerInfo["available"]; !exists {
			t.Error("expected 'available' field in Docker info")
		} else if available.(bool) {
			if _, exists := dockerInfo["version"]; !exists {
				t.Error("expected 'version' field when Docker is available")
			}
		}
	})

	t.Run("getTotalRAM", func(t *testing.T) {
		ramInfo := getTotalRAM()
		if totalMB, exists := ramInfo["total_mb"]; !exists || totalMB.(int64) < 0 {
			t.Error("expected non-negative total RAM")
		}
	})

	t.Run("getTotalDisk", func(t *testing.T) {
		diskInfo := getTotalDisk()
		if totalGB, exists := diskInfo["total_gb"]; !exists || totalGB.(int64) < 0 {
			t.Error("expected non-negative total disk space")
		}
	})
}

func TestValidContainerRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container deploy test in short mode")
	}

	t.Parallel()

	mux := setupTestServer("test-version", "")

	// Create a valid container request
	validRequest := ContainerRequest{
		Image: "hello-world:latest",
		Name:  "test-hello",
		Ports: map[string]string{
			"80": "8080",
		},
		Env: map[string]string{
			"TEST_VAR": "test_value",
		},
	}

	requestBody, err := json.Marshal(validRequest)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	req := httptest.NewRequest("POST", "/containers", bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// This may fail if Docker is not available, but we're testing the validation logic
	if w.Code == http.StatusBadRequest {
		var response map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&response); err == nil {
			if errorCode := response["error"]; errorCode == "missing_image" || errorCode == "missing_name" {
				t.Errorf("valid request should not fail validation, got error: %v", errorCode)
			}
		}
	}

	// If Docker is not available, we expect a 500 error, which is fine for this test
	if w.Code != http.StatusCreated && w.Code != http.StatusInternalServerError {
		t.Logf("Container deploy returned status %d (may be expected if Docker unavailable)", w.Code)
	}
}

func TestMergeContainerMetrics(t *testing.T) {
	t.Parallel()

	metrics := []ContainerMetrics{
		{
			ID:         "abc123456789",
			Name:       "yokai-vllm-1",
			Status:     "running",
			CPUPercent: 25.0,
			MemUsedMB:  1024,
		},
	}

	dockerContainers := []Container{
		{
			ID:     "abc1234567890123456789",
			Name:   "yokai-vllm-1",
			Image:  "vllm/vllm-openai:latest",
			Status: "running",
		},
		{
			ID:     "def9876543210123456789",
			Name:   "yokai-llama-1",
			Image:  "ghcr.io/ggml-org/llama.cpp:server-cuda",
			Status: "stopped",
		},
	}

	merged := mergeContainerMetrics(metrics, dockerContainers)
	if len(merged) != 2 {
		t.Fatalf("expected 2 containers after merge, got %d", len(merged))
	}

	if merged[0].Image != "vllm/vllm-openai:latest" {
		t.Errorf("expected merged image for running container, got %q", merged[0].Image)
	}

	if merged[1].Name != "yokai-llama-1" {
		t.Errorf("expected appended container name, got %q", merged[1].Name)
	}

	if merged[1].Status != "stopped" {
		t.Errorf("expected appended container status 'stopped', got %q", merged[1].Status)
	}
}

func TestShortContainerID(t *testing.T) {
	t.Parallel()

	if got := shortContainerID("1234567890123456"); got != "123456789012" {
		t.Errorf("expected 12-char ID, got %q", got)
	}

	if got := shortContainerID("abc"); got != "abc" {
		t.Errorf("expected short ID to remain unchanged, got %q", got)
	}
}

func TestLoadAuthTokenPrefersEnvPath(t *testing.T) {
	base := t.TempDir()
	envPath := filepath.Join(base, "env-agent.json")
	homeDir := filepath.Join(base, "home")
	homeConfig := filepath.Join(homeDir, ".config", "yokai")

	if err := os.MkdirAll(homeConfig, 0700); err != nil {
		t.Fatalf("mkdir home config: %v", err)
	}
	if err := os.WriteFile(envPath, []byte(`{"token":"env-token"}`), 0600); err != nil {
		t.Fatalf("write env token: %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeConfig, "agent.json"), []byte(`{"token":"home-token"}`), 0600); err != nil {
		t.Fatalf("write home token: %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("YOKAI_AGENT_CONFIG", envPath)

	authToken = ""
	loadAuthToken()

	if authToken != "env-token" {
		t.Fatalf("expected env token, got %q", authToken)
	}
}

func TestLoadAuthTokenFallsBackToHome(t *testing.T) {
	base := t.TempDir()
	homeDir := filepath.Join(base, "home")
	homeConfig := filepath.Join(homeDir, ".config", "yokai")

	if err := os.MkdirAll(homeConfig, 0700); err != nil {
		t.Fatalf("mkdir home config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeConfig, "agent.json"), []byte(`{"token":"home-token"}`), 0600); err != nil {
		t.Fatalf("write home token: %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("YOKAI_AGENT_CONFIG", "")

	authToken = ""
	loadAuthToken()

	if authToken != "home-token" {
		t.Fatalf("expected home token, got %q", authToken)
	}
}

func TestLoadAuthTokenPrefersSystemPathBeforeHome(t *testing.T) {
	base := t.TempDir()
	homeDir := filepath.Join(base, "home")
	homeConfig := filepath.Join(homeDir, ".config", "yokai")
	systemPath := filepath.Join(base, "etc-agent.json")

	if err := os.MkdirAll(homeConfig, 0700); err != nil {
		t.Fatalf("mkdir home config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeConfig, "agent.json"), []byte(`{"token":"home-token"}`), 0600); err != nil {
		t.Fatalf("write home token: %v", err)
	}
	if err := os.WriteFile(systemPath, []byte(`{"token":"system-token"}`), 0600); err != nil {
		t.Fatalf("write system token: %v", err)
	}

	oldSystemPath := systemAgentConfigPath
	systemAgentConfigPath = systemPath
	defer func() {
		systemAgentConfigPath = oldSystemPath
	}()

	t.Setenv("HOME", homeDir)
	t.Setenv("YOKAI_AGENT_CONFIG", "")

	authToken = ""
	loadAuthToken()

	if authToken != "system-token" {
		t.Fatalf("expected system token, got %q", authToken)
	}
}

func TestLoadAuthTokenMissingClearsValue(t *testing.T) {
	t.Setenv("YOKAI_AGENT_CONFIG", filepath.Join(t.TempDir(), "missing.json"))

	authToken = "stale"
	loadAuthToken()

	if authToken != "" {
		t.Fatalf("expected empty token when no config exists, got %q", authToken)
	}
}

func TestLegacyLifecycleRejectsDeploymentManagedContainer(t *testing.T) {
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	id := strings.Repeat("a", 64)
	script := "#!/bin/sh\nif [ \"$1\" = inspect ]; then\nprintf '%s\\n' '\"" + id + "\"' '\"/candidate\"' '{\"io.yokai.managed\":\"true\",\"io.yokai.ownership\":\"managed\",\"io.yokai.deployment.id\":\"dep-test\",\"io.yokai.deployment.generation\":\"3\",\"io.yokai.deployment.role\":\"head\"}'\nexit 0\nfi\nexit 99\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for name, handler := range map[string]http.HandlerFunc{"stop": handleContainerStop, "delete": handleContainerDelete, "restart": handleContainerRestart} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/containers/candidate/"+name, nil)
			request.SetPathValue("id", "candidate")
			response := httptest.NewRecorder()
			handler(response, request)
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "grouped_deployment_required") {
				t.Fatalf("legacy lifecycle bypass was not rejected: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestLegacyLifecycleFailsClosedWhenIdentityInspectionFails(t *testing.T) {
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for name, handler := range map[string]http.HandlerFunc{"stop": handleContainerStop, "delete": handleContainerDelete, "restart": handleContainerRestart} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/containers/candidate/"+name, nil)
			request.SetPathValue("id", "candidate")
			response := httptest.NewRecorder()
			handler(response, request)
			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "container_identity_unavailable") {
				t.Fatalf("identity failure did not fail closed: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestLegacyLifecycleMissingIdentityFallsThroughToExistingNotFoundChecks(t *testing.T) {
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nif [ \"$1\" = inspect ]; then\n  printf '%s\\n' 'Error: No such object: missing' >&2\n  exit 1\nfi\nexit 99\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for name, handler := range map[string]http.HandlerFunc{"delete": handleContainerDelete, "restart": handleContainerRestart} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/containers/missing/"+name, nil)
			request.SetPathValue("id", "missing")
			response := httptest.NewRecorder()
			handler(response, request)
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "container_not_found") {
				t.Fatalf("missing container did not reach existing 404 check: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestLegacyRestartWaitsForInFlightStopBarrier(t *testing.T) {
	binDir := t.TempDir()
	stopStarted := filepath.Join(binDir, "stop-started")
	releaseStop := filepath.Join(binDir, "release-stop")
	restartCalled := filepath.Join(binDir, "restart-called")
	dockerPath := filepath.Join(binDir, "docker")
	id := strings.Repeat("b", 64)
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = inspect ]; then
  case "$2" in
    *State.Status*) printf 'running\n'; exit 0 ;;
  esac
  printf '%%s\n' '"%s"' '"/previous"' '{}'
  exit 0
fi
if [ "$1" = stop ]; then
  : > %q
  while [ ! -f %q ]; do sleep 0.01; done
  exit 0
fi
if [ "$1" = restart ]; then
  : > %q
  exit 0
fi
exit 1
`, id, stopStarted, releaseStop, restartCalled)
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	defer func() { _ = os.WriteFile(releaseStop, []byte("release"), 0600) }()

	stopRequest := httptest.NewRequest(http.MethodPost, "/containers/previous/stop", nil)
	stopRequest.SetPathValue("id", id)
	stopResponse := httptest.NewRecorder()
	stopDone := make(chan struct{})
	go func() {
		handleContainerStop(stopResponse, stopRequest)
		close(stopDone)
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(stopStarted); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("docker stop did not begin")
		}
		time.Sleep(time.Millisecond)
	}

	restartRequest := httptest.NewRequest(http.MethodPost, "/containers/previous/restart", nil)
	restartRequest.SetPathValue("id", id)
	restartResponse := httptest.NewRecorder()
	restartDone := make(chan struct{})
	go func() {
		handleContainerRestart(restartResponse, restartRequest)
		close(restartDone)
	}()
	blockedUntil := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(blockedUntil) {
		if _, err := os.Stat(restartCalled); err == nil {
			t.Fatal("restart raced the in-flight docker stop")
		}
		select {
		case <-restartDone:
			t.Fatal("restart handler returned before the stop settled")
		default:
		}
		time.Sleep(time.Millisecond)
	}

	if err := os.WriteFile(releaseStop, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, done := range map[string]<-chan struct{}{"stop": stopDone, "restart": restartDone} {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s handler did not finish", name)
		}
	}
	if stopResponse.Code != http.StatusOK || restartResponse.Code != http.StatusOK {
		t.Fatalf("unexpected lifecycle responses: stop=%d %s restart=%d %s", stopResponse.Code, stopResponse.Body.String(), restartResponse.Code, restartResponse.Body.String())
	}
	if _, err := os.Stat(restartCalled); err != nil {
		t.Fatalf("restart did not run after stop settled: %v", err)
	}
}

func TestDeploymentMemberRestartWaitsForInFlightStopBarrier(t *testing.T) {
	binDir := t.TempDir()
	stopStarted := filepath.Join(binDir, "managed-stop-started")
	releaseStop := filepath.Join(binDir, "managed-release-stop")
	restartCalled := filepath.Join(binDir, "managed-restart-called")
	dockerPath := filepath.Join(binDir, "docker")
	id := strings.Repeat("c", 64)
	labels := `{"io.yokai.managed":"true","io.yokai.ownership":"managed","io.yokai.deployment.id":"dep-test","io.yokai.deployment.generation":"1","io.yokai.deployment.role":"head"}`
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = inspect ]; then
  printf '%%s\n' '"%s"' '"/previous"' '%s'
  exit 0
fi
if [ "$1" = stop ]; then
  : > %q
  while [ ! -f %q ]; do sleep 0.01; done
  exit 0
fi
if [ "$1" = restart ]; then
  : > %q
  exit 0
fi
exit 1
`, id, labels, stopStarted, releaseStop, restartCalled)
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	defer func() { _ = os.WriteFile(releaseStop, []byte("release"), 0600) }()

	newRequest := func(action string) *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/deployments/dep-test/members/"+id+"/"+action+"?generation=1&role=head&name=previous", nil)
		request.SetPathValue("deploymentID", "dep-test")
		request.SetPathValue("id", id)
		return request
	}
	stopResponse := httptest.NewRecorder()
	stopDone := make(chan struct{})
	go func() {
		handleDeploymentMemberStop(stopResponse, newRequest("stop"))
		close(stopDone)
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(stopStarted); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("managed docker stop did not begin")
		}
		time.Sleep(time.Millisecond)
	}

	restartResponse := httptest.NewRecorder()
	restartDone := make(chan struct{})
	go func() {
		handleDeploymentMemberRestart(restartResponse, newRequest("restart"))
		close(restartDone)
	}()
	blockedUntil := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(blockedUntil) {
		if _, err := os.Stat(restartCalled); err == nil {
			t.Fatal("managed restart raced the in-flight docker stop")
		}
		select {
		case <-restartDone:
			t.Fatal("managed restart returned before the stop settled")
		default:
		}
		time.Sleep(time.Millisecond)
	}
	if err := os.WriteFile(releaseStop, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, done := range map[string]<-chan struct{}{"stop": stopDone, "restart": restartDone} {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("managed %s handler did not finish", name)
		}
	}
	if stopResponse.Code != http.StatusOK || restartResponse.Code != http.StatusOK {
		t.Fatalf("unexpected managed lifecycle responses: stop=%d %s restart=%d %s", stopResponse.Code, stopResponse.Body.String(), restartResponse.Code, restartResponse.Body.String())
	}
	if _, err := os.Stat(restartCalled); err != nil {
		t.Fatalf("managed restart did not run after stop settled: %v", err)
	}
}

func TestDeploymentMemberLifecycleRequiresExactProvenance(t *testing.T) {
	base := map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelDeploymentID: "dep-test", LabelGeneration: "3", LabelRole: "head"}
	if err := validateDeploymentMemberProvenance("candidate", base, "dep-test", "3", "head", "candidate"); err != nil {
		t.Fatalf("correct provenance was rejected: %v", err)
	}
	tests := map[string]func(map[string]string) (string, string, string, string){
		"managed": func(labels map[string]string) (string, string, string, string) {
			labels[LabelManaged] = "false"
			return "dep-test", "3", "head", "candidate"
		},
		"ownership": func(labels map[string]string) (string, string, string, string) {
			labels[LabelOwnership] = OwnershipObserved
			return "dep-test", "3", "head", "candidate"
		},
		"deployment": func(labels map[string]string) (string, string, string, string) {
			return "other", "3", "head", "candidate"
		},
		"generation": func(labels map[string]string) (string, string, string, string) {
			return "dep-test", "4", "head", "candidate"
		},
		"role": func(labels map[string]string) (string, string, string, string) {
			return "dep-test", "3", "worker", "candidate"
		},
		"name": func(labels map[string]string) (string, string, string, string) {
			return "dep-test", "3", "head", "other"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			labels := make(map[string]string, len(base))
			for key, value := range base {
				labels[key] = value
			}
			deploymentID, generation, role, expectedName := mutate(labels)
			if err := validateDeploymentMemberProvenance("candidate", labels, deploymentID, generation, role, expectedName); err == nil {
				t.Fatalf("%s mismatch passed provenance validation", name)
			}
		})
	}
}
