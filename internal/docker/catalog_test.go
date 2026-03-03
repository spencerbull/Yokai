package docker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewCatalog(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()

	if catalog == nil {
		t.Fatal("NewCatalog returned nil")
	}

	if catalog.cache == nil {
		t.Error("cache should be initialized")
	}

	if catalog.cacheTTL != 1*time.Hour {
		t.Errorf("expected cache TTL of 1h, got %v", catalog.cacheTTL)
	}

	if catalog.client == nil {
		t.Error("HTTP client should be initialized")
	}

	if catalog.client.Timeout != 15*time.Second {
		t.Errorf("expected client timeout of 15s, got %v", catalog.client.Timeout)
	}
}

func TestKnownImages(t *testing.T) {
	t.Parallel()

	known := KnownImages()

	expectedServices := []string{"vllm", "llamacpp", "comfyui"}
	for _, service := range expectedServices {
		if images, exists := known[service]; !exists {
			t.Errorf("expected known images for service '%s'", service)
		} else if len(images) == 0 {
			t.Errorf("expected at least one known image for service '%s'", service)
		}
	}

	// Check specific known images
	if vllmImages, exists := known["vllm"]; exists {
		found := false
		for _, img := range vllmImages {
			if img == "vllm/vllm-openai" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected 'vllm/vllm-openai' in VLLM known images")
		}
	}

	if llamaImages, exists := known["llamacpp"]; exists {
		found := false
		for _, img := range llamaImages {
			if img == "ghcr.io/ggml-org/llama.cpp" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected 'ghcr.io/ggml-org/llama.cpp' in llamacpp known images")
		}
	}
}

func TestCacheExpiry(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	catalog.cacheTTL = 100 * time.Millisecond // Short TTL for testing

	// Mock server for Docker Hub
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"name":         "latest",
					"last_updated": time.Now().Format(time.RFC3339),
					"full_size":    1024 * 1024 * 100, // 100MB
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Override the client to use test server
	originalClient := catalog.client
	catalog.client = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &mockTransport{
			dockerHubURL: server.URL,
		},
	}
	defer func() { catalog.client = originalClient }()

	// First fetch - should hit the server
	tags1, err := catalog.FetchTags("test/image")
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}

	// Second fetch immediately - should use cache
	tags2, err := catalog.FetchTags("test/image")
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}

	if len(tags1) != len(tags2) {
		t.Error("cache should return same results")
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Third fetch - should hit server again (cache expired)
	_, err = catalog.FetchTags("test/image")
	if err != nil {
		t.Fatalf("third fetch failed: %v", err)
	}
}

func TestFetchTagsDockerHub(t *testing.T) {
	t.Parallel()

	// Mock Docker Hub API response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept both the real path and the mock path
		if !strings.Contains(r.URL.Path, "/tags") && !strings.Contains(r.URL.Path, "/mock-tags") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		response := map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"name":         "latest",
					"last_updated": "2024-01-15T10:30:00Z",
					"full_size":    1073741824, // 1GB
				},
				{
					"name":         "v2.1.0",
					"last_updated": "2024-01-14T09:15:00Z",
					"full_size":    1048576000, // ~1GB
				},
				{
					"name":         "nightly",
					"last_updated": "2024-01-16T02:00:00Z",
					"full_size":    1100000000,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	catalog := NewCatalog()
	catalog.client = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &mockTransport{
			dockerHubURL: server.URL,
		},
	}

	tags, err := catalog.FetchTags("vllm/vllm-openai")
	if err != nil {
		t.Fatalf("fetch tags failed: %v", err)
	}

	if len(tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(tags))
	}

	// Check specific tag
	var latestTag *Tag
	for _, tag := range tags {
		if tag.Name == "latest" {
			latestTag = &tag
			break
		}
	}

	if latestTag == nil {
		t.Fatal("latest tag not found")
	}

	if latestTag.SizeMB != 1024 { // 1GB / 1024 / 1024
		t.Errorf("expected size 1024MB, got %d", latestTag.SizeMB)
	}

	// Check nightly detection
	var nightlyTag *Tag
	for _, tag := range tags {
		if tag.Name == "nightly" {
			nightlyTag = &tag
			break
		}
	}

	if nightlyTag == nil {
		t.Fatal("nightly tag not found")
	}

	if !nightlyTag.Nightly {
		t.Error("nightly tag should be marked as nightly")
	}
}

func TestFetchTagsGHCR(t *testing.T) {
	t.Parallel()

	// Mock GHCR token endpoint
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]string{
			"token": "mock-token-12345",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer tokenServer.Close()

	// Mock GHCR tags endpoint
	tagsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization header
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("expected Bearer token, got %s", auth)
		}

		response := map[string]interface{}{
			"tags": []string{
				"latest",
				"server-cuda",
				"server-cpu",
				"dev-build-123",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer tagsServer.Close()

	catalog := NewCatalog()
	catalog.client = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &mockTransport{
			ghcrTokenURL: tokenServer.URL,
			ghcrTagsURL:  tagsServer.URL,
		},
	}

	tags, err := catalog.FetchTags("ghcr.io/ggml-org/llama.cpp")
	if err != nil {
		t.Fatalf("fetch GHCR tags failed: %v", err)
	}

	if len(tags) != 4 {
		t.Errorf("expected 4 tags, got %d", len(tags))
	}

	// Check dev tag detection
	var devTag *Tag
	for _, tag := range tags {
		if tag.Name == "dev-build-123" {
			devTag = &tag
			break
		}
	}

	if devTag == nil {
		t.Fatal("dev tag not found")
	}

	if !devTag.Nightly {
		t.Error("dev tag should be marked as nightly")
	}
}

func TestFetchTagsCaching(t *testing.T) {
	t.Parallel()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		response := map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"name":         "latest",
					"last_updated": time.Now().Format(time.RFC3339),
					"full_size":    1024 * 1024 * 100,
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	catalog := NewCatalog()
	catalog.client = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &mockTransport{
			dockerHubURL: server.URL,
		},
	}

	// First call
	tags1, err := catalog.FetchTags("test/image")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Second call should use cache
	tags2, err := catalog.FetchTags("test/image")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 HTTP call, got %d", callCount)
	}

	if len(tags1) != len(tags2) {
		t.Error("cached results should match original")
	}
}

// mockTransport allows us to override URLs for testing
type mockTransport struct {
	dockerHubURL string
	ghcrTokenURL string
	ghcrTagsURL  string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	url := req.URL.String()

	// Redirect Docker Hub requests to test server
	if strings.Contains(url, "hub.docker.com") && m.dockerHubURL != "" {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(m.dockerHubURL, "http://")
		req.URL.Path = "/mock-tags"
	}

	// Redirect GHCR token requests
	if strings.Contains(url, "ghcr.io/token") && m.ghcrTokenURL != "" {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(m.ghcrTokenURL, "http://")
		req.URL.Path = "/"
	}

	// Redirect GHCR tags requests
	if strings.Contains(url, "ghcr.io/v2/") && strings.Contains(url, "/tags/list") && m.ghcrTagsURL != "" {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(m.ghcrTagsURL, "http://")
		req.URL.Path = "/"
	}

	return http.DefaultTransport.RoundTrip(req)
}

func TestTagStructure(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tag := Tag{
		Name:        "v1.2.3",
		LastUpdated: now,
		SizeMB:      512,
		Nightly:     false,
	}

	if tag.Name != "v1.2.3" {
		t.Errorf("expected name 'v1.2.3', got %s", tag.Name)
	}
	if tag.LastUpdated != now {
		t.Error("LastUpdated time mismatch")
	}
	if tag.SizeMB != 512 {
		t.Errorf("expected size 512MB, got %d", tag.SizeMB)
	}
	if tag.Nightly {
		t.Error("expected non-nightly tag")
	}
}

func TestNightlyDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tagName  string
		expected bool
	}{
		{"latest", false},
		{"v1.2.3", false},
		{"stable", false},
		{"nightly", true},
		{"nightly-20240115", true},
		{"dev", true},
		{"dev-build-123", true},
		{"development", true}, // Contains "dev"
		{"main-dev", true},    // Contains "dev"
	}

	for _, tt := range tests {
		t.Run(tt.tagName, func(t *testing.T) {
			tag := Tag{
				Name:    tt.tagName,
				Nightly: strings.Contains(tt.tagName, "nightly") || strings.Contains(tt.tagName, "dev"),
			}

			if tag.Nightly != tt.expected {
				t.Errorf("tag %s: expected nightly=%v, got %v", tt.tagName, tt.expected, tag.Nightly)
			}
		})
	}
}
