package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

func TestHealthIndicatorCoversCreatedAndUnknownStates(t *testing.T) {
	list := ServiceList{}

	created := list.healthIndicator(ServiceRow{Status: "created"})
	if !strings.Contains(created, "◌") {
		t.Fatalf("expected created state to use pending indicator, got %q", created)
	}

	unknown := list.healthIndicator(ServiceRow{Status: "unknown"})
	if !strings.Contains(unknown, "!") {
		t.Fatalf("expected unknown state to use warning indicator, got %q", unknown)
	}
}

func TestStatusTextIncludesRestartingState(t *testing.T) {
	list := ServiceList{}

	if got := list.statusText(ServiceRow{Status: "restarting"}); got != "restarting" {
		t.Fatalf("expected restarting status text, got %q", got)
	}
	if got := list.statusText(ServiceRow{Status: "created"}); got != "created" {
		t.Fatalf("expected created status text, got %q", got)
	}
}

func TestServiceListHidesDeviceColumnForSingleDevice(t *testing.T) {
	list := NewServiceList([]ServiceRow{{Name: "svc-a", Device: "alpha"}, {Name: "svc-b", Device: "alpha"}}, 100)
	for _, col := range list.activeColumns() {
		if col.id == "device" {
			t.Fatal("expected device column to be hidden for a single-device service list")
		}
	}
}

func TestServiceListKeepsDeviceColumnForFleetView(t *testing.T) {
	list := NewServiceList([]ServiceRow{{Name: "svc-a", Device: "alpha"}, {Name: "svc-b", Device: "beta"}}, 100)
	found := false
	for _, col := range list.activeColumns() {
		if col.id == "device" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected device column to remain visible for multi-device service lists")
	}
}

func TestMarqueeKeepsFixedVisibleWidth(t *testing.T) {
	got := marquee("very-long-service-name", 8, 5, 3)
	if ansi.StringWidth(got) != 8 {
		t.Fatalf("expected marquee width 8, got %d (%q)", ansi.StringWidth(got), got)
	}
}

func TestSelectedServiceUsesMarqueeInRenderedList(t *testing.T) {
	zone.NewGlobal()
	list := NewServiceList([]ServiceRow{{Name: "very-long-service-name", Selected: true}}, 24)
	list.Cursor = 0
	list.MarqueeOffset = 4
	output := list.Render()
	if !strings.Contains(output, "long") {
		t.Fatalf("expected rendered selected row to include marquee window, got %q", output)
	}
}
