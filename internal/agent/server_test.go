package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	expectedFields := []string{"status", "version", "uptime_seconds", "hostname"}
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
			Model:                 "meta-llama/Llama-3.1-8B-Instruct",
			PromptTokPerSec:       118.2,
			GenerationTokPerSec:   35.5,
			RequestsRunning:       3,
			RequestsWaiting:       1,
			PromptTokensTotal:     1200,
			GenerationTokensTotal: 900,
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
		`yokai_llm_generated_tokens_total{backend="vllm",model="meta-llama/Llama-3.1-8B-Instruct",service="vllm-llama31-8b"} 900`,
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
