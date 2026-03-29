package views

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/components"
)

func newExtraArgsInput() textarea.Model {
	ta := components.NewTextAreaField("Extra arguments")
	ta.SetWidth(30)
	return ta
}

func TestUpdateConfigAllowsSpaceInExtraArgs(t *testing.T) {
	d := &Deploy{
		cfg:               config.DefaultConfig(),
		extraArgsInput:    newExtraArgsInput(),
		portInput:         components.NewPortField("8000"),
		pickerSearchInput: components.NewTextField("Type to filter"),
		activeConfigField: 1,
	}
	d.extraArgsInput.Focus()

	_, _ = d.updateConfig(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	if got := d.extraArgsInput.Value(); got != " " {
		t.Fatalf("expected extra args to contain a space, got %q", got)
	}
}

func TestUpdateConfigSpaceTogglesHelpWhenSelected(t *testing.T) {
	d := &Deploy{
		cfg:               config.DefaultConfig(),
		extraArgsInput:    newExtraArgsInput(),
		portInput:         components.NewPortField("8000"),
		pickerSearchInput: components.NewTextField("Type to filter"),
		activeConfigField: 2,
	}

	_, _ = d.updateConfig(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	if !d.showArgsHelp {
		t.Fatal("expected help toggle to open when help row is selected")
	}
}

func TestFilteredTypeOptionsFuzzySearch(t *testing.T) {
	d := &Deploy{pickerSearchInput: components.NewTextField("Type to filter")}
	d.pickerSearchInput.SetValue("lmc")

	options := d.filteredTypeOptions()
	if len(options) == 0 {
		t.Fatal("expected at least one fuzzy match")
	}
	if options[0].Primary != "llama.cpp" {
		t.Fatalf("expected llama.cpp to be top fuzzy match, got %q", options[0].Primary)
	}
}

func TestFilteredDeviceOptionsMatchesHostAndLabel(t *testing.T) {
	d := &Deploy{
		cfg:               config.DefaultConfig(),
		pickerSearchInput: components.NewTextField("Type to filter"),
	}
	d.cfg.Devices = []config.Device{
		{ID: "rig-a", Label: "Main Rig", Host: "100.64.0.2"},
		{ID: "rig-b", Label: "Backup", Host: "finn"},
	}
	d.pickerSearchInput.SetValue("finn")

	options := d.filteredDeviceOptions()
	if len(options) != 1 {
		t.Fatalf("expected one device match, got %d", len(options))
	}
	if options[0].Index != 1 {
		t.Fatalf("expected device index 1, got %d", options[0].Index)
	}
}

func TestSetOrReplaceArgHandlesSplitAndEqualsForms(t *testing.T) {
	got := setOrReplaceArg("--foo bar --gpu-memory-utilization=0.50", "--gpu-memory-utilization", "0.82")
	if got != "--foo bar --gpu-memory-utilization 0.82" {
		t.Fatalf("unexpected args: %q", got)
	}

	got = setOrReplaceArg("--foo bar", "--gpu-memory-utilization", "0.82")
	if got != "--foo bar --gpu-memory-utilization 0.82" {
		t.Fatalf("unexpected appended args: %q", got)
	}
}

func TestRoundUpHundredth(t *testing.T) {
	if got := roundUpHundredth(0.811); got != 0.82 {
		t.Fatalf("expected 0.82, got %.2f", got)
	}
}

func TestApplyVLLMMemoryEstimateAddsRecommendedFlags(t *testing.T) {
	d := &Deploy{
		cfg:                config.DefaultConfig(),
		extraArgsInput:     newExtraArgsInput(),
		vllmMemoryEstimate: &vllmMemoryEstimate{Utilization: 0.82, ContextLength: 32768, TensorParallelSize: 4},
	}
	d.extraArgsInput.SetValue("--max-model-len 8192")

	d.applyVLLMMemoryEstimate()

	got := d.extraArgsInput.Value()
	if val, ok := argValue(got, "--max-model-len"); !ok || val != "32768" {
		t.Fatalf("expected max model len to be replaced, got %q", got)
	}
	if val, ok := argValue(got, "--gpu-memory-utilization"); !ok || val != "0.82" {
		t.Fatalf("expected gpu memory utilization to be set, got %q", got)
	}
	if val, ok := argValue(got, "--tensor-parallel-size"); !ok || val != "4" {
		t.Fatalf("expected tensor parallel size to be set, got %q", got)
	}
	if d.vllmMemoryError == "" {
		t.Fatal("expected apply status message")
	}
}

func TestApplyCurrentBKCPrefillsVisibleFields(t *testing.T) {
	d := &Deploy{
		cfg:               config.DefaultConfig(),
		workload:          wtVLLM,
		imageInput:        components.NewTextField("Docker image"),
		modelInput:        components.NewTextField("Model"),
		portInput:         components.NewPortField("8000"),
		extraArgsInput:    newExtraArgsInput(),
		vllmContextInput:  components.NewPortField("32768"),
		vllmOverheadInput: components.NewTextField("1.5"),
	}
	d.modelInput.SetValue("nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-NVFP4")

	d.applyCurrentBKC()

	if d.appliedBKCID == "" {
		t.Fatal("expected applied BKC id")
	}
	if got := d.imageInput.Value(); got != "vllm/vllm-openai:v0.17.0-cu130" {
		t.Fatalf("unexpected image %q", got)
	}
	if got := d.portInput.Value(); got != "8888" {
		t.Fatalf("unexpected port %q", got)
	}
	if val, ok := argValue(d.extraArgsInput.Value(), "--max-model-len"); !ok || val != "262144" {
		t.Fatalf("expected BKC max model len, got %q", d.extraArgsInput.Value())
	}
	if !d.hasAppliedBKC() {
		t.Fatal("expected BKC to be marked as applied")
	}
}

func TestBuildDeployRequestIncludesAppliedBKCEnvAndVolumes(t *testing.T) {
	d := &Deploy{
		cfg:            config.DefaultConfig(),
		workload:       wtVLLM,
		imageInput:     components.NewTextField("Docker image"),
		modelInput:     components.NewTextField("Model"),
		portInput:      components.NewPortField("8000"),
		extraArgsInput: newExtraArgsInput(),
	}
	d.modelInput.SetValue("nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-NVFP4")
	d.applyCurrentBKC()

	req := d.buildDeployRequest(config.Service{
		ID:       "svc",
		DeviceID: "dev1",
		Type:     "vllm",
		Image:    d.imageInput.Value(),
		Model:    d.modelInput.Value(),
		Port:     8888,
	})

	if req.Env["MODEL"] != "nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-NVFP4" {
		t.Fatalf("expected MODEL env, got %#v", req.Env)
	}
	if req.Env["VLLM_ATTENTION_BACKEND"] != "FLASHINFER" {
		t.Fatalf("expected BKC env vars, got %#v", req.Env)
	}
	if req.Volumes["/var/lib/yokai/huggingface"] != "/root/.cache/huggingface" {
		t.Fatalf("expected Hugging Face cache mount, got %#v", req.Volumes)
	}
	if len(req.Plugins) != 1 || req.Plugins[0] != "vllm-reasoning-parser-super-v3" {
		t.Fatalf("expected parser plugin, got %#v", req.Plugins)
	}
	if req.Runtime.IPCMode != "host" || req.Runtime.ShmSize != "16g" {
		t.Fatalf("unexpected runtime settings %#v", req.Runtime)
	}
	if req.Ports["8888"] != "8888" {
		t.Fatalf("expected selected port mapping, got %#v", req.Ports)
	}
}

func TestUpdateConfigEnterAddsNewLineInExtraArgs(t *testing.T) {
	d := &Deploy{
		cfg:               config.DefaultConfig(),
		extraArgsInput:    newExtraArgsInput(),
		portInput:         components.NewPortField("8000"),
		pickerSearchInput: components.NewTextField("Type to filter"),
		activeConfigField: 1,
	}
	d.extraArgsInput.SetValue("--foo bar")
	d.extraArgsInput.Focus()

	_, _ = d.updateConfig(tea.KeyMsg{Type: tea.KeyEnter})

	if got := d.extraArgsInput.Value(); got != "--foo bar\n" {
		t.Fatalf("expected newline in extra args, got %q", got)
	}
}

func TestBuildDeployRequestNormalizesMultiLineExtraArgs(t *testing.T) {
	d := &Deploy{
		cfg:            config.DefaultConfig(),
		workload:       wtVLLM,
		imageInput:     components.NewTextField("Docker image"),
		modelInput:     components.NewTextField("Model"),
		portInput:      components.NewPortField("8000"),
		extraArgsInput: newExtraArgsInput(),
	}
	d.extraArgsInput.SetValue("--tensor-parallel-size 2\n--max-model-len 32768\n")

	req := d.buildDeployRequest(config.Service{ID: "svc", DeviceID: "dev1", Type: "vllm", Image: "img", Model: "model", Port: 8000})

	if req.ExtraArgs != "--tensor-parallel-size 2 --max-model-len 32768" {
		t.Fatalf("unexpected normalized extra args %q", req.ExtraArgs)
	}
}
