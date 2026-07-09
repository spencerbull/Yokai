package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

func TestValidatePassphraseConfirmation(t *testing.T) {
	t.Parallel()
	if err := validatePassphraseConfirmation("matching passphrase", "matching passphrase"); err != nil {
		t.Fatalf("matching passphrases returned error: %v", err)
	}
	if err := validatePassphraseConfirmation("first passphrase", "second passphrase"); err == nil {
		t.Fatal("mismatched passphrases were accepted")
	}
}

func TestReloadDaemonWithClientTimesOut(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.Daemon.Listen = strings.TrimPrefix(server.URL, "http://")
	client := &http.Client{Timeout: 25 * time.Millisecond}
	started := time.Now()
	if reloadDaemonWithClient(context.Background(), cfg, client) {
		t.Fatal("stalled daemon reload unexpectedly succeeded")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("stalled daemon reload took %s", elapsed)
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("YOKAI_TEST_DEFAULT", " override ")
	if got := envOrDefault("YOKAI_TEST_DEFAULT", "fallback"); got != "override" {
		t.Fatalf("envOrDefault() = %q", got)
	}
	t.Setenv("YOKAI_TEST_DEFAULT", "")
	if got := envOrDefault("YOKAI_TEST_DEFAULT", "fallback"); got != "fallback" {
		t.Fatalf("envOrDefault() fallback = %q", got)
	}
}
