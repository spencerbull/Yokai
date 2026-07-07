package daemon

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/spencerbull/yokai/internal/config"
)

func TestHandleGetSettings(t *testing.T) {
	configureTestConfigHome(t)

	cfg := config.DefaultConfig()
	cfg.HFToken = "hf_config_token"
	cfg.Preferences.Theme = "storm"
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveHistory(&config.History{
		Images: []string{"image-a", "image-b"},
		Models: []string{"model-a"},
	}); err != nil {
		t.Fatalf("save history: %v", err)
	}

	d := &Daemon{cfg: cfg}
	req := httptest.NewRequest("GET", "/settings", nil)
	rr := httptest.NewRecorder()

	d.handleGetSettings(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp uiSettingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.HF.Configured || resp.HF.Source != "config" {
		t.Fatalf("unexpected HF settings: %#v", resp.HF)
	}
	if resp.Preferences.Theme != "storm" {
		t.Fatalf("expected theme storm, got %q", resp.Preferences.Theme)
	}
	if len(resp.History.Images) != 2 || resp.History.Images[0] != "image-a" {
		t.Fatalf("unexpected history: %#v", resp.History)
	}
}

func TestHandlePutHFToken(t *testing.T) {
	configureTestConfigHome(t)

	cfg := config.DefaultConfig()
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	d := &Daemon{cfg: cfg}
	body := bytes.NewBufferString(`{"token":" hf_saved_token "}`)
	req := httptest.NewRequest("PUT", "/settings/hf-token", body)
	rr := httptest.NewRecorder()

	d.handlePutHFToken(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if d.cfg.HFToken != "hf_saved_token" {
		t.Fatalf("expected trimmed token to be saved, got %q", d.cfg.HFToken)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.HFToken != "hf_saved_token" {
		t.Fatalf("expected saved token in config, got %q", loaded.HFToken)
	}
}

func TestHandlePatchSettings(t *testing.T) {
	configureTestConfigHome(t)

	cfg := config.DefaultConfig()
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	d := &Daemon{cfg: cfg}
	body := bytes.NewBufferString(`{"preferences":{"theme":"midnight","default_vllm_image":"custom/vllm:latest"}}`)
	req := httptest.NewRequest("PATCH", "/settings", body)
	rr := httptest.NewRecorder()

	d.handlePatchSettings(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if d.cfg.Preferences.Theme != "midnight" {
		t.Fatalf("expected updated theme, got %q", d.cfg.Preferences.Theme)
	}
	if d.cfg.Preferences.DefaultVLLMImage != "custom/vllm:latest" {
		t.Fatalf("expected updated vllm image, got %q", d.cfg.Preferences.DefaultVLLMImage)
	}
}

func TestHandlePutDeployHistory(t *testing.T) {
	configureTestConfigHome(t)

	d := &Daemon{cfg: config.DefaultConfig()}
	body := bytes.NewBufferString(`{"images":["img-a","img-a"," ","img-b"],"models":["model-a","model-a","model-b"]}`)
	req := httptest.NewRequest("PUT", "/history/deploy", body)
	rr := httptest.NewRecorder()

	d.handlePutDeployHistory(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	history, err := config.LoadHistory()
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	if len(history.Images) != 2 || history.Images[0] != "img-a" || history.Images[1] != "img-b" {
		t.Fatalf("unexpected saved images: %#v", history.Images)
	}
	if len(history.Models) != 2 || history.Models[0] != "model-a" || history.Models[1] != "model-b" {
		t.Fatalf("unexpected saved models: %#v", history.Models)
	}
}

func configureTestConfigHome(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("HOME", tempDir)
	t.Setenv("HF_TOKEN", "")
	t.Setenv("HUGGING_FACE_HUB_TOKEN", "")
}
