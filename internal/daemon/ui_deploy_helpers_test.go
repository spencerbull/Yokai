package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestHandleDeployBKCReturnsSpeculativeVariants(t *testing.T) {
	t.Parallel()

	d := &Daemon{}
	model := url.QueryEscape("RadixArk/Qwen3.8-27B-NVFP4")
	req := httptest.NewRequest(http.MethodGet, "/deploy/bkc?workload=sglang&model="+model, nil)
	recorder := httptest.NewRecorder()

	d.handleDeployBKC(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response deployBKCResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Config == nil || response.Config.ID != "qwen3-8-27b-nvfp4-sglang-dflash2" {
		t.Fatalf("expected DFlash2 to be the default BKC, got %#v", response.Config)
	}
	if len(response.Configs) != 2 {
		t.Fatalf("expected two selectable BKCs, got %#v", response.Configs)
	}
	if response.Configs[1].ID != "qwen3-8-27b-nvfp4-sglang-dspark" {
		t.Fatalf("expected DSpark rollback BKC, got %#v", response.Configs)
	}
}
