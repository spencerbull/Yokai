package hf

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	client := NewClient("test-token")

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if client.token != "test-token" {
		t.Errorf("expected token 'test-token', got %s", client.token)
	}

	if client.httpClient == nil {
		t.Error("HTTP client should be initialized")
	}

	if client.httpClient.Timeout != 15*time.Second {
		t.Errorf("expected timeout 15s, got %v", client.httpClient.Timeout)
	}
}

func TestSearchModels(t *testing.T) {
	t.Parallel()

	// Mock HuggingFace API response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request parameters
		query := r.URL.Query()
		if query.Get("search") != "llama" {
			t.Errorf("expected search query 'llama', got %s", query.Get("search"))
		}
		if query.Get("filter") != "text-generation" {
			t.Errorf("expected filter 'text-generation', got %s", query.Get("filter"))
		}
		if query.Get("sort") != "likes" {
			t.Errorf("expected sort 'likes', got %s", query.Get("sort"))
		}

		// Verify authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("expected auth 'Bearer test-token', got %s", auth)
		}

		// Mock response
		models := []Model{
			{
				ID:        "microsoft/DialoGPT-medium",
				Author:    "microsoft",
				Likes:     150,
				Downloads: 50000,
				Tags:      []string{"transformers", "pytorch"},
				Pipeline:  "text-generation",
			},
			{
				ID:        "facebook/opt-1.3b",
				Author:    "facebook",
				Likes:     200,
				Downloads: 75000,
				Tags:      []string{"transformers", "pytorch", "opt"},
				Pipeline:  "text-generation",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	client := NewClient("test-token")
	// Override the base URL for testing
	client.httpClient = &http.Client{
		Timeout:   15 * time.Second,
		Transport: &mockHFTransport{baseURL: server.URL},
	}

	models, err := client.SearchModels("llama", 2)
	if err != nil {
		t.Fatalf("SearchModels failed: %v", err)
	}

	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}

	// Check first model
	if models[0].ID != "microsoft/DialoGPT-medium" {
		t.Errorf("expected first model ID 'microsoft/DialoGPT-medium', got %s", models[0].ID)
	}
	if models[0].Author != "microsoft" {
		t.Errorf("expected author 'microsoft', got %s", models[0].Author)
	}
	if models[0].Likes != 150 {
		t.Errorf("expected 150 likes, got %d", models[0].Likes)
	}

	// Check second model
	if models[1].Downloads != 75000 {
		t.Errorf("expected 75000 downloads, got %d", models[1].Downloads)
	}
}

func TestSearchModelsError(t *testing.T) {
	t.Parallel()

	// Mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Invalid token"))
	}))
	defer server.Close()

	client := NewClient("invalid-token")
	client.httpClient = &http.Client{
		Timeout:   15 * time.Second,
		Transport: &mockHFTransport{baseURL: server.URL},
	}

	_, err := client.SearchModels("test", 10)
	if err == nil {
		t.Error("expected error for unauthorized request")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 error, got: %v", err)
	}
}

func TestListGGUFFiles(t *testing.T) {
	t.Parallel()

	// Mock HuggingFace API file tree response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the URL path contains the model ID or is our test endpoint
		if !strings.Contains(r.URL.Path, "/models/microsoft/DialoGPT-small/tree/main") &&
			!strings.Contains(r.URL.Path, "/api/models") &&
			r.URL.Path != "/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Mock file tree response
		files := []map[string]interface{}{
			{
				"rfilename": "model.gguf",
				"size":      1073741824, // 1GB
			},
			{
				"rfilename": "model-q4_0.gguf",
				"size":      536870912, // 512MB
			},
			{
				"rfilename": "config.json",
				"size":      1024,
			},
			{
				"rfilename": "tokenizer.json",
				"size":      2048,
			},
			{
				"rfilename": "model-f16.GGUF", // Test case sensitivity
				"size":      2147483648,       // 2GB
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(files)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.httpClient = &http.Client{
		Timeout:   15 * time.Second,
		Transport: &mockHFTransport{baseURL: server.URL},
	}

	ggufFiles, err := client.ListGGUFFiles("microsoft/DialoGPT-small")
	if err != nil {
		t.Fatalf("ListGGUFFiles failed: %v", err)
	}

	// Should find 3 .gguf files (case-insensitive)
	if len(ggufFiles) != 3 {
		t.Errorf("expected 3 GGUF files, got %d", len(ggufFiles))
	}

	// Check specific files
	foundFiles := make(map[string]bool)
	for _, file := range ggufFiles {
		foundFiles[file.Filename] = true

		// Verify size calculation (bytes to MB)
		switch file.Filename {
		case "model.gguf":
			if file.SizeMB != 1024 { // 1GB / 1024
				t.Errorf("expected model.gguf size 1024MB, got %d", file.SizeMB)
			}
		case "model-q4_0.gguf":
			if file.SizeMB != 512 { // 512MB
				t.Errorf("expected model-q4_0.gguf size 512MB, got %d", file.SizeMB)
			}
		case "model-f16.GGUF":
			if file.SizeMB != 2048 { // 2GB / 1024
				t.Errorf("expected model-f16.GGUF size 2048MB, got %d", file.SizeMB)
			}
		}
	}

	if !foundFiles["model.gguf"] {
		t.Error("model.gguf not found")
	}
	if !foundFiles["model-q4_0.gguf"] {
		t.Error("model-q4_0.gguf not found")
	}
	if !foundFiles["model-f16.GGUF"] {
		t.Error("model-f16.GGUF not found (case insensitive)")
	}
}

func TestValidateToken(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectedError bool
		expectedName  string
	}{
		{
			name:          "valid token",
			statusCode:    http.StatusOK,
			responseBody:  `{"name": "testuser"}`,
			expectedError: false,
			expectedName:  "testuser",
		},
		{
			name:          "invalid token",
			statusCode:    http.StatusUnauthorized,
			responseBody:  `{"error": "Invalid token"}`,
			expectedError: true,
		},
		{
			name:          "server error",
			statusCode:    http.StatusInternalServerError,
			responseBody:  `{"error": "Server error"}`,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify it hits the whoami endpoint
				if !strings.HasSuffix(r.URL.Path, "/whoami-v2") {
					t.Errorf("expected whoami-v2 endpoint, got %s", r.URL.Path)
				}

				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.httpClient = &http.Client{
				Timeout: 15 * time.Second,
				Transport: &mockHFTransport{
					whoamiURL: server.URL + "/api/whoami-v2",
				},
			}

			name, err := client.ValidateToken()

			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if name != tt.expectedName {
					t.Errorf("expected name %s, got %s", tt.expectedName, name)
				}
			}
		})
	}
}

func TestSearchModelsWithDefaults(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		limit := query.Get("limit")

		// Default limit should be 20
		if limit != "20" {
			t.Errorf("expected default limit 20, got %s", limit)
		}

		// Return minimal response
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer server.Close()

	client := NewClient("")
	client.httpClient = &http.Client{
		Transport: &mockHFTransport{baseURL: server.URL},
	}

	// Test with limit 0 (should default to 20)
	_, err := client.SearchModels("test", 0)
	if err != nil {
		t.Errorf("SearchModels with default limit failed: %v", err)
	}

	// Test with negative limit (should default to 20)
	_, err = client.SearchModels("test", -5)
	if err != nil {
		t.Errorf("SearchModels with negative limit failed: %v", err)
	}
}

func TestSetAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "with token",
			token:    "hf_test123",
			expected: "Bearer hf_test123",
		},
		{
			name:     "empty token",
			token:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.token)
			req, _ := http.NewRequest("GET", "http://example.com", nil)

			client.setAuth(req)

			auth := req.Header.Get("Authorization")
			if auth != tt.expected {
				t.Errorf("expected auth header '%s', got '%s'", tt.expected, auth)
			}
		})
	}
}

// mockHFTransport allows redirecting HuggingFace API calls to test servers
type mockHFTransport struct {
	baseURL   string
	whoamiURL string
}

func (m *mockHFTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	originalURL := req.URL.String()

	// Redirect models API calls
	if strings.Contains(originalURL, "huggingface.co/api/models") && m.baseURL != "" {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(m.baseURL, "http://")
		req.URL.Path = "/api/models"
	}

	// Redirect whoami calls
	if strings.Contains(originalURL, "whoami-v2") && m.whoamiURL != "" {
		req.URL, _ = req.URL.Parse(m.whoamiURL)
	}

	return http.DefaultTransport.RoundTrip(req)
}

func TestModelStructure(t *testing.T) {
	t.Parallel()

	model := Model{
		ID:        "gpt2",
		Author:    "openai",
		Likes:     500,
		Downloads: 1000000,
		Tags:      []string{"transformers", "pytorch"},
		Pipeline:  "text-generation",
	}

	if model.ID != "gpt2" {
		t.Errorf("expected ID 'gpt2', got %s", model.ID)
	}
	if model.Author != "openai" {
		t.Errorf("expected author 'openai', got %s", model.Author)
	}
	if len(model.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(model.Tags))
	}
}

func TestGGUFFileStructure(t *testing.T) {
	t.Parallel()

	file := GGUFFile{
		Filename: "model-q4_0.gguf",
		SizeMB:   512,
	}

	if file.Filename != "model-q4_0.gguf" {
		t.Errorf("expected filename 'model-q4_0.gguf', got %s", file.Filename)
	}
	if file.SizeMB != 512 {
		t.Errorf("expected size 512MB, got %d", file.SizeMB)
	}
}
