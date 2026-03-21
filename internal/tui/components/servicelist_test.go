package components

import (
	"strings"
	"testing"
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
