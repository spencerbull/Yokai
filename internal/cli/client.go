package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

// daemonClient talks to the local yokai daemon API.
type daemonClient struct {
	baseURL string
	http    *http.Client
}

func newDaemonClient(cfg *config.Config) *daemonClient {
	addr := cfg.Daemon.Listen
	if addr == "" {
		addr = "127.0.0.1:7473"
	}
	return &daemonClient{
		baseURL: "http://" + addr,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *daemonClient) get(path string) (json.RawMessage, error) {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("daemon unreachable (is 'yokai daemon' running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &errResp) == nil && (errResp.Error != "" || errResp.Message != "") {
			msg := errResp.Error
			if errResp.Message != "" {
				msg = errResp.Message
			}
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	return json.RawMessage(body), nil
}

func (c *daemonClient) post(path string, body io.Reader) (json.RawMessage, error) {
	resp, err := c.http.Post(c.baseURL+path, "application/json", body)
	if err != nil {
		return nil, fmt.Errorf("daemon unreachable (is 'yokai daemon' running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && (errResp.Error != "" || errResp.Message != "") {
			msg := errResp.Error
			if errResp.Message != "" {
				msg = errResp.Message
			}
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	return json.RawMessage(respBody), nil
}

func (c *daemonClient) doDelete(path string) (json.RawMessage, error) {
	req, err := http.NewRequest("DELETE", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("daemon unreachable (is 'yokai daemon' running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && (errResp.Error != "" || errResp.Message != "") {
			msg := errResp.Error
			if errResp.Message != "" {
				msg = errResp.Message
			}
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	return json.RawMessage(respBody), nil
}

// getSSE connects to an SSE endpoint and sends lines to a channel.
// The caller should close the returned channel by cancelling the request context.
func (c *daemonClient) getSSE(path string) (<-chan string, error) {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Use a client with no timeout for streaming
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("daemon unreachable: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("log stream failed with status %d", resp.StatusCode)
	}

	ch := make(chan string, 100)
	go func() {
		defer func() { _ = resp.Body.Close() }()
		defer close(ch)

		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				// Parse SSE lines
				data := string(buf[:n])
				for _, line := range splitLines(data) {
					if len(line) > 6 && line[:6] == "data: " {
						ch <- line[6:]
					} else if line != "" {
						ch <- line
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()

	return ch, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
