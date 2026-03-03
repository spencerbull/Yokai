package docker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Tag represents a Docker image tag.
type Tag struct {
	Name        string    `json:"name"`
	LastUpdated time.Time `json:"last_updated"`
	SizeMB      int64     `json:"size_mb"`
	Nightly     bool      `json:"nightly"`
}

// Catalog fetches and caches image tags.
type Catalog struct {
	cache    map[string]cachedTags
	cacheMu  sync.Mutex
	cacheTTL time.Duration
	client   *http.Client
}

type cachedTags struct {
	tags      []Tag
	fetchedAt time.Time
}

// NewCatalog creates a tag catalog with 1h cache TTL.
func NewCatalog() *Catalog {
	return &Catalog{
		cache:    make(map[string]cachedTags),
		cacheTTL: 1 * time.Hour,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

// FetchTags returns tags for a Docker image, using cache if available.
func (c *Catalog) FetchTags(image string) ([]Tag, error) {
	c.cacheMu.Lock()
	if cached, ok := c.cache[image]; ok && time.Since(cached.fetchedAt) < c.cacheTTL {
		c.cacheMu.Unlock()
		return cached.tags, nil
	}
	c.cacheMu.Unlock()

	var tags []Tag
	var err error

	if strings.HasPrefix(image, "ghcr.io/") {
		tags, err = c.fetchGHCR(image)
	} else {
		tags, err = c.fetchDockerHub(image)
	}

	if err != nil {
		return nil, err
	}

	c.cacheMu.Lock()
	c.cache[image] = cachedTags{tags: tags, fetchedAt: time.Now()}
	c.cacheMu.Unlock()

	return tags, nil
}

func (c *Catalog) fetchDockerHub(image string) ([]Tag, error) {
	// Normalize image name (e.g., "vllm/vllm-openai" → "vllm/vllm-openai")
	parts := strings.SplitN(image, "/", 2)
	var apiPath string
	if len(parts) == 1 {
		apiPath = fmt.Sprintf("library/%s", parts[0])
	} else {
		apiPath = image
	}

	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags?page_size=50&ordering=last_updated", apiPath)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Docker Hub request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Docker Hub %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []struct {
			Name        string    `json:"name"`
			LastUpdated time.Time `json:"last_updated"`
			FullSize    int64     `json:"full_size"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing tags: %w", err)
	}

	var tags []Tag
	for _, r := range result.Results {
		tag := Tag{
			Name:        r.Name,
			LastUpdated: r.LastUpdated,
			SizeMB:      r.FullSize / (1024 * 1024),
			Nightly:     strings.Contains(r.Name, "nightly") || strings.Contains(r.Name, "dev"),
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

func (c *Catalog) fetchGHCR(image string) ([]Tag, error) {
	// ghcr.io/ggml-org/llama.cpp → ggml-org/llama.cpp
	path := strings.TrimPrefix(image, "ghcr.io/")

	url := fmt.Sprintf("https://ghcr.io/v2/%s/tags/list", path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// GHCR requires an anonymous token
	tokenURL := fmt.Sprintf("https://ghcr.io/token?scope=repository:%s:pull", path)
	tokenResp, err := c.client.Get(tokenURL)
	if err != nil {
		return nil, fmt.Errorf("GHCR token: %w", err)
	}
	defer tokenResp.Body.Close()

	var tokenResult struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
		return nil, fmt.Errorf("parsing GHCR token: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+tokenResult.Token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GHCR request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing GHCR tags: %w", err)
	}

	var tags []Tag
	for _, name := range result.Tags {
		tags = append(tags, Tag{
			Name:    name,
			Nightly: strings.Contains(name, "nightly") || strings.Contains(name, "dev"),
		})
	}

	return tags, nil
}

// KnownImages returns the default image options for each service type.
func KnownImages() map[string][]string {
	return map[string][]string{
		"vllm":    {"vllm/vllm-openai"},
		"llamacpp": {"ghcr.io/ggml-org/llama.cpp"},
		"comfyui": {"yanwk/comfyui-boot"},
	}
}
