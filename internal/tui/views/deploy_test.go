package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/components"
)

func TestUpdateConfigAllowsSpaceInExtraArgs(t *testing.T) {
	d := &Deploy{
		cfg:               config.DefaultConfig(),
		extraArgsInput:    components.NewTextField("Extra arguments"),
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
		extraArgsInput:    components.NewTextField("Extra arguments"),
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
		extraArgsInput:     components.NewTextField("Extra arguments"),
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
