package plugins

import (
	"strings"
	"testing"
)

func TestLookupNemotronParserPlugin(t *testing.T) {
	t.Parallel()

	plugin, ok := Lookup("vllm-reasoning-parser-super-v3")
	if !ok {
		t.Fatal("expected plugin catalog entry")
	}
	if plugin.Workload != "vllm" {
		t.Fatalf("unexpected workload %q", plugin.Workload)
	}
	if len(plugin.Assets) != 1 || !strings.Contains(plugin.Assets[0].URL, "super_v3_reasoning_parser.py") {
		t.Fatalf("unexpected plugin assets %#v", plugin.Assets)
	}
}
