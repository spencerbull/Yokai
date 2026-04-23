package hf

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// FileURL returns the public download URL for a file inside a HuggingFace
// repository. The path is the repo-relative filename as returned by the tree
// API.
func FileURL(modelID, filename string) string {
	return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", modelID, filename)
}

// Downloader streams files from HuggingFace to local disk. Downloads that are
// already complete (matching remote Content-Length) are a no-op.
type Downloader struct {
	Token  string
	Client *http.Client
}

// NewDownloader creates a new Downloader. A long HTTP timeout is used so that
// multi-gigabyte GGUF shards can complete.
func NewDownloader(token string) *Downloader {
	return &Downloader{
		Token:  token,
		Client: &http.Client{Timeout: 2 * time.Hour},
	}
}

// Download writes the repo file at `filename` to `destPath`. Parent
// directories are created as needed. When the destination already exists and
// matches the remote Content-Length it is left untouched.
func (d *Downloader) Download(modelID, filename, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("creating dest dir: %w", err)
	}

	remoteSize, err := d.headSize(modelID, filename)
	if err == nil && remoteSize > 0 {
		if info, statErr := os.Stat(destPath); statErr == nil && info.Size() == remoteSize {
			return nil
		}
	}

	req, err := http.NewRequest("GET", FileURL(modelID, filename), nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	d.setAuth(req)

	resp, err := d.Client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", filename, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("downloading %s: HTTP %d: %s", filename, resp.StatusCode, string(body))
	}

	tmp := destPath + ".partial"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating %s: %w", tmp, err)
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("closing %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, destPath); err != nil {
		return fmt.Errorf("renaming %s: %w", tmp, err)
	}
	return nil
}

// headSize issues a HEAD request to discover the remote file size. A zero
// return value indicates an unknown size (e.g. the HEAD endpoint redirected to
// a streaming CDN that did not report Content-Length).
func (d *Downloader) headSize(modelID, filename string) (int64, error) {
	req, err := http.NewRequest("HEAD", FileURL(modelID, filename), nil)
	if err != nil {
		return 0, err
	}
	d.setAuth(req)

	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HEAD %s: status %d", filename, resp.StatusCode)
	}
	return resp.ContentLength, nil
}

func (d *Downloader) setAuth(req *http.Request) {
	if d.Token != "" {
		req.Header.Set("Authorization", "Bearer "+d.Token)
	}
}
