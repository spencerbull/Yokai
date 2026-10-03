package daemon

import (
	"encoding/json"
	"testing"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/config"
)

func TestLiveOpenAIEndpointCandidatesIncludeUnmanagedRunningContainers(t *testing.T) {
	t.Parallel()

	devices := []config.Device{
		{ID: "dev-finn", Label: "finn", Host: "finn.trout-rooster.ts.net"},
	}
	claimedPorts := map[string]map[int]struct{}{}
	liveContainers := map[string][]agentContainerRecord{
		"dev-finn": {
			{
				ID:     "container-1",
				Name:   "yokai-vllm-nvidia-NVIDIA-Nemotron-3-Super",
				Image:  "vllm/vllm-openai:v0.17.0-cu130",
				Status: "running",
				Ports:  map[string]string{"8000": "8888"},
			},
			{
				ID:     "container-2",
				Name:   "yokai-mon-grafana",
				Image:  "grafana/grafana:latest",
				Status: "running",
				Ports:  map[string]string{"3000": "3001"},
			},
		},
	}

	candidates := liveOpenAIEndpointCandidates(devices, claimedPorts, liveContainers)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 unmanaged OpenAI candidate, got %d", len(candidates))
	}
	if candidates[0].DeviceLabel != "finn" || candidates[0].Host != "finn.trout-rooster.ts.net" || candidates[0].Port != 8888 {
		t.Fatalf("unexpected candidate: %#v", candidates[0])
	}
	if candidates[0].ServiceType != "vllm" {
		t.Fatalf("expected vllm candidate, got %#v", candidates[0])
	}
}

func TestLiveOpenAIEndpointCandidatesSkipClaimedPortsAndStoppedContainers(t *testing.T) {
	t.Parallel()

	devices := []config.Device{
		{ID: "dev-finn", Label: "finn", Host: "finn.trout-rooster.ts.net"},
	}
	claimedPorts := map[string]map[int]struct{}{
		"dev-finn": {8888: {}},
	}
	liveContainers := map[string][]agentContainerRecord{
		"dev-finn": {
			{
				ID:     "container-1",
				Name:   "yokai-vllm-claimed",
				Image:  "vllm/vllm-openai:latest",
				Status: "running",
				Ports:  map[string]string{"8000": "8888"},
			},
			{
				ID:     "container-2",
				Name:   "yokai-vllm-stopped",
				Image:  "vllm/vllm-openai:latest",
				Status: "stopped",
				Ports:  map[string]string{"8000": "9999"},
			},
		},
	}

	candidates := liveOpenAIEndpointCandidates(devices, claimedPorts, liveContainers)
	if len(candidates) != 0 {
		t.Fatalf("expected no unmanaged OpenAI candidates, got %#v", candidates)
	}
}

func TestYokaiModelNameIncludesDeviceTag(t *testing.T) {
	t.Parallel()

	if got := yokaiModelName("nemotron-3-super-nvfp4", "Finn"); got != "nemotron-3-super-nvfp4 (yokai-finn)" {
		t.Fatalf("unexpected yokai model name: %q", got)
	}
	if got := yokaiModelName("Qwen/Qwen3", "My GPU Box"); got != "Qwen/Qwen3 (yokai-my-gpu-box)" {
		t.Fatalf("unexpected spaced device tag: %q", got)
	}
	if got := yokaiModelName("model", ""); got != "model (yokai)" {
		t.Fatalf("unexpected fallback device tag: %q", got)
	}
}

func TestLiveOpenAIEndpointCandidatesIncludeTensorFoldHead(t *testing.T) {
	t.Parallel()

	devices := []config.Device{{ID: "dev-kyber", Label: "kyber", Host: "kyber.example"}}
	// Decode the agent's /metrics container shape, which discovery reads
	// from the aggregator cache.
	var containers []agentContainerRecord
	if err := json.Unmarshal([]byte(`[{"id":"tensorfold:yokai-deployment-d1-g1-head","name":"yokai-deployment-d1-g1-head","image":"`+bkc.GLM53FlashEXL3TensorFoldImage+`","status":"running","ports":{"8888":"8888"}}]`), &containers); err != nil {
		t.Fatal(err)
	}

	candidates := liveOpenAIEndpointCandidates(devices, map[string]map[int]struct{}{}, map[string][]agentContainerRecord{"dev-kyber": containers})
	if len(candidates) != 1 || candidates[0].ServiceType != "tensorfold" || candidates[0].Port != 8888 {
		t.Fatalf("TensorFold head was not offered as an OpenAI endpoint: %#v", candidates)
	}
}
