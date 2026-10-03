package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stagingTensorFoldRunner answers every preflight probe like a healthy pair of
// nodes and materializes one pinned file on checkout. fetchFailures makes the
// next N fetches fail as an unreachable remote would.
func stagingTensorFoldRunner(body []byte, fetchFailures *int) tensorFoldRunnerFunc {
	base := &fakeTensorFoldRunner{}
	return func(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if name == "git" && strings.Contains(joined, "fetch") && *fetchFailures > 0 {
			*fetchFailures--
			return nil, errors.New("network unreachable")
		}
		if name == "git" && strings.Contains(joined, "checkout --quiet --detach") {
			if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o700); err != nil {
				return nil, err
			}
			return nil, os.WriteFile(filepath.Join(dir, "start.sh"), body, 0o600)
		}
		return base.Run(ctx, dir, env, name, args...)
	}
}

func newStagingTensorFoldManager(t *testing.T, fetchFailures *int) *tensorFoldManager {
	t.Helper()
	body := []byte("pinned start\n")
	sum := sha256.Sum256(body)
	manager := newTensorFoldManager(t.TempDir(), stagingTensorFoldRunner(body, fetchFailures))
	manager.pinnedFiles = map[string]string{"start.sh": hex.EncodeToString(sum[:])}
	return manager
}

func TestTensorFoldInterruptedRecipeFetchLeavesNoPartialCheckout(t *testing.T) {
	fetchFailures := 1
	manager := newStagingTensorFoldManager(t, &fetchFailures)
	if err := manager.ensurePinnedRecipe(context.Background()); err == nil || !strings.Contains(err.Error(), "stage pinned TensorFold recipe") {
		t.Fatalf("failed fetch was not reported as a staging failure: %v", err)
	}
	if _, err := os.Lstat(manager.recipePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed fetch left a partial recipe checkout: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(manager.recipePath()))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed fetch left staging debris: %v", entries)
	}
	if err := manager.ensurePinnedRecipe(context.Background()); err != nil {
		t.Fatalf("retry after a transient fetch failure did not recover: %v", err)
	}
	if _, err := os.Stat(filepath.Join(manager.recipePath(), "start.sh")); err != nil {
		t.Fatalf("retry did not install the pinned checkout: %v", err)
	}
}

func TestTensorFoldHeadPreflightStagesAndVerifiesPinnedRecipe(t *testing.T) {
	request := validTensorFoldResourceRequest(true)

	fetchFailures := 1
	unreachable := newStagingTensorFoldManager(t, &fetchFailures)
	if err := unreachable.preflight(context.Background(), request); err == nil || !strings.Contains(err.Error(), "stage pinned TensorFold recipe") {
		t.Fatalf("unreachable recipe passed head preflight: %v", err)
	}
	if _, err := os.Lstat(unreachable.resourcePath(request.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed preflight crossed the resource mutation boundary: %v", err)
	}

	fetchFailures = 0
	healthy := newStagingTensorFoldManager(t, &fetchFailures)
	if err := healthy.preflight(context.Background(), request); err != nil {
		t.Fatalf("healthy head preflight failed: %v", err)
	}
	if err := healthy.verifyPinnedRecipeFiles(); err != nil {
		t.Fatalf("head preflight did not leave a verified checkout: %v", err)
	}
}

func TestTensorFoldHeadLogsReadOnlyBoundedTail(t *testing.T) {
	manager := newTensorFoldManager(t.TempDir(), &fakeTensorFoldRunner{})
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	logFile, err := manager.openLog(resource.Name)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("x", 1023) + "\n"
	for written := 0; written < 2*tensorFoldLogReadLimit; written += len(line) {
		if _, err := logFile.WriteString(line); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := logFile.WriteString("final supervisor line\n"); err != nil {
		t.Fatal(err)
	}
	if err := logFile.Close(); err != nil {
		t.Fatal(err)
	}

	data, truncated, err := readTensorFoldLogTail(manager.logPath(resource.Name))
	if err != nil || !truncated || len(data) > tensorFoldLogReadLimit || !strings.HasPrefix(string(data), "xxx") {
		t.Fatalf("supervisor log read was not a bounded line-aligned tail: len=%d truncated=%v err=%v", len(data), truncated, err)
	}
	tail, truncated, err := manager.logs(context.Background(), resource)
	if err != nil || !truncated || !strings.HasSuffix(tail, "final supervisor line\n") {
		t.Fatalf("bounded supervisor log lost its newest lines: truncated=%v err=%v", truncated, err)
	}
}
