package hf

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://huggingface.co/api"

// Client interacts with the HuggingFace API.
type Client struct {
	token      string
	httpClient *http.Client
}

// Model represents a HuggingFace model.
type Model struct {
	ID        string   `json:"id"`
	Author    string   `json:"author"`
	Likes     int      `json:"likes"`
	Downloads int      `json:"downloads"`
	Tags      []string `json:"tags"`
	Pipeline  string   `json:"pipeline_tag"`
}

// GGUFFile represents a GGUF quantization file.
type GGUFFile struct {
	Filename string `json:"rfilename"`
	SizeMB   int64
}

// NewClient creates a HuggingFace API client.
func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SearchModels searches for text-generation models.
func (c *Client) SearchModels(query string, limit int) ([]Model, error) {
	if limit <= 0 {
		limit = 20
	}

	params := url.Values{
		"search": {query},
		"filter": {"text-generation"},
		"sort":   {"likes"},
		"limit":  {fmt.Sprintf("%d", limit)},
	}

	reqURL := fmt.Sprintf("%s/models?%s", baseURL, params.Encode())
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HF API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HF API %d: %s", resp.StatusCode, string(body))
	}

	var models []Model
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, fmt.Errorf("parsing models: %w", err)
	}

	return models, nil
}

// ListGGUFFiles lists .gguf files in a model repo (for llama.cpp).
func (c *Client) ListGGUFFiles(modelID string) ([]GGUFFile, error) {
	reqURL := fmt.Sprintf("%s/models/%s/tree/main", baseURL, modelID)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HF API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HF API %d", resp.StatusCode)
	}

	var files []struct {
		Path string `json:"rfilename"`
		Size int64  `json:"size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("parsing file tree: %w", err)
	}

	var ggufFiles []GGUFFile
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f.Path), ".gguf") {
			ggufFiles = append(ggufFiles, GGUFFile{
				Filename: f.Path,
				SizeMB:   f.Size / (1024 * 1024),
			})
		}
	}

	return ggufFiles, nil
}

// ValidateToken checks if the token is valid by hitting the whoami endpoint.
func (c *Client) ValidateToken() (string, error) {
	req, err := http.NewRequest("GET", "https://huggingface.co/api/whoami-v2", nil)
	if err != nil {
		return "", err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HF API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("invalid token")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HF API %d", resp.StatusCode)
	}

	var result struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Name, nil
}

func (c *Client) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
