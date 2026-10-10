package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func exitedInspectFixture(host, rank, launchID, containerID string) []byte {
	data := tensorFoldInspectFixtureWithIdentity(host, rank, launchID, containerID)
	var records []map[string]any
	if err := json.Unmarshal(data, &records); err != nil {
		panic(err)
	}
	records[0]["State"] = map[string]any{"Running": false}
	out, _ := json.Marshal(records)
	return out
}

func newReconcileManager(t *testing.T, resource tensorFoldResource, runner tensorFoldRunnerFunc) *tensorFoldManager {
	t.Helper()
	manager := newTensorFoldManager(t.TempDir(), runner)
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestReconcileRebootRestartRecovery(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	startCalls := []string{}
	var stopCalled bool

	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "docker" && strings.HasPrefix(joined, "ps -a"):
			return []byte(headID + "\n"), nil
		case name == "docker" && strings.HasPrefix(joined, "inspect"):
			return exitedInspectFixture("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return exitedInspectFixture("", "1", strings.Repeat("a", 64), workerID), nil
		case name == "./stop.sh":
			stopCalled = true
			return nil, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	resource.HeadContainerID = headID
	resource.WorkerContainerID = workerID
	manager := newReconcileManager(t, resource, runner)

	manager.startRecipeContainer = func(_ context.Context, _ tensorFoldResource, worker bool, id string) error {
		role := "head"
		if worker {
			role = "worker"
		}
		startCalls = append(startCalls, role+":"+id)
		return nil
	}

	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if stopCalled {
		t.Fatal("stop.sh was invoked during bounded restart; containers must not be deleted")
	}
	if len(startCalls) != 2 {
		t.Fatalf("expected 2 docker-start calls (worker then head), got %d: %v", len(startCalls), startCalls)
	}
	if !strings.HasPrefix(startCalls[0], "worker:") {
		t.Fatalf("worker must be started first, got: %v", startCalls)
	}
	if !strings.HasPrefix(startCalls[1], "head:") {
		t.Fatalf("head must be started second, got: %v", startCalls)
	}

	got, err := manager.inspect(resource.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "needs-restart" {
		t.Fatalf("status = %q, want needs-restart", got.Status)
	}
	if got.RestartAttempts != 1 {
		t.Fatalf("RestartAttempts = %d, want 1", got.RestartAttempts)
	}
	if got.LastRestartAt.IsZero() {
		t.Fatal("LastRestartAt not set")
	}
}

func TestReconcileRestartFailsAfterBudget(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	var stopCalled bool

	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "docker" && strings.HasPrefix(joined, "ps -a"):
			return []byte(headID + "\n"), nil
		case name == "docker" && strings.HasPrefix(joined, "inspect"):
			return exitedInspectFixture("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return exitedInspectFixture("", "1", strings.Repeat("a", 64), workerID), nil
		case name == "./stop.sh":
			stopCalled = true
			return nil, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	resource.HeadContainerID = headID
	resource.WorkerContainerID = workerID
	manager := newReconcileManager(t, resource, runner)

	manager.startRecipeContainer = func(_ context.Context, _ tensorFoldResource, _ bool, _ string) error {
		return errors.New("docker start failed")
	}

	// Attempt 1
	if err := manager.reconcile(context.Background()); err == nil {
		t.Fatal("expected restart failure error on attempt 1")
	}
	got, _ := manager.inspect(resource.Name)
	if got.RestartAttempts != 1 || got.Status != "needs-restart" {
		t.Fatalf("after attempt 1: attempts=%d status=%q", got.RestartAttempts, got.Status)
	}

	// Attempt 2 (backoff elapsed via LastRestartAt manipulation)
	got.LastRestartAt = time.Now().UTC().Add(-2 * time.Minute)
	manager.mu.Lock()
	if err := manager.writeResource(got); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.mu.Unlock()
	if err := manager.reconcile(context.Background()); err == nil {
		t.Fatal("expected restart failure error on attempt 2")
	}
	got, _ = manager.inspect(resource.Name)
	if got.RestartAttempts != 2 || got.Status != "needs-restart" {
		t.Fatalf("after attempt 2: attempts=%d status=%q", got.RestartAttempts, got.Status)
	}

	// Attempt 3 (final)
	got.LastRestartAt = time.Now().UTC().Add(-2 * time.Minute)
	manager.mu.Lock()
	if err := manager.writeResource(got); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.mu.Unlock()
	if err := manager.reconcile(context.Background()); err == nil {
		t.Fatal("expected restart failure error on attempt 3")
	}
	got, _ = manager.inspect(resource.Name)
	if got.RestartAttempts != 3 {
		t.Fatalf("after attempt 3: attempts=%d, want 3", got.RestartAttempts)
	}

	// A 4th pass: budget exhausted, should mark failed (no start attempted, no error expected)
	got.LastRestartAt = time.Now().UTC().Add(-2 * time.Minute)
	manager.mu.Lock()
	if err := manager.writeResource(got); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.mu.Unlock()
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile after budget exhausted: %v", err)
	}
	got, _ = manager.inspect(resource.Name)
	if got.Status != "failed" {
		t.Fatalf("after budget exhausted: status=%q, want failed", got.Status)
	}
	if stopCalled {
		t.Fatal("stop.sh must not be called after budget exhaustion; containers left in place")
	}
}

func TestReconcileZeroAttemptsWithTimestampNoPanic(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	startCount := 0

	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "docker" && strings.HasPrefix(joined, "ps -a"):
			return []byte(headID + "\n"), nil
		case name == "docker" && strings.HasPrefix(joined, "inspect"):
			return exitedInspectFixture("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return exitedInspectFixture("", "1", strings.Repeat("a", 64), workerID), nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	resource.HeadContainerID = headID
	resource.WorkerContainerID = workerID
	// Legacy/hand-edited record: LastRestartAt set but RestartAttempts == 0.
	resource.LastRestartAt = time.Now().UTC().Add(-time.Hour)
	manager := newReconcileManager(t, resource, runner)

	manager.startRecipeContainer = func(_ context.Context, _ tensorFoldResource, _ bool, _ string) error {
		startCount++
		return nil
	}

	// Must not panic: index clamped to 0 (backoff = 30s), elapsed > 30s so
	// the restart proceeds normally.
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile with zero attempts + timestamp: %v", err)
	}
	if startCount != 2 {
		t.Fatalf("expected 2 start calls after clamp, got %d", startCount)
	}
	got, err := manager.inspect(resource.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.RestartAttempts != 1 {
		t.Fatalf("RestartAttempts = %d, want 1", got.RestartAttempts)
	}
	if got.Status != "needs-restart" {
		t.Fatalf("status = %q, want needs-restart", got.Status)
	}
}

func TestReconcileBackoffWindow(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	startCount := 0

	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "docker" && strings.HasPrefix(joined, "ps -a"):
			return []byte(headID + "\n"), nil
		case name == "docker" && strings.HasPrefix(joined, "inspect"):
			return exitedInspectFixture("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return exitedInspectFixture("", "1", strings.Repeat("a", 64), workerID), nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	resource.HeadContainerID = headID
	resource.WorkerContainerID = workerID
	manager := newReconcileManager(t, resource, runner)

	manager.startRecipeContainer = func(_ context.Context, _ tensorFoldResource, _ bool, _ string) error {
		startCount++
		return nil
	}

	// First pass: worker + head = 2 start calls in one pass.
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if startCount != 2 {
		t.Fatalf("expected 2 start calls after first pass, got %d", startCount)
	}

	// Second pass immediately: backoff window (30s) not elapsed, no retry.
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if startCount != 2 {
		t.Fatalf("expected no second start within backoff window, got %d total starts", startCount)
	}
}

func TestReconcileBudgetExhaustedDoesNotIncrementAttempts(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	startCount := 0

	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "docker" && strings.HasPrefix(joined, "ps -a"):
			return []byte(headID + "\n"), nil
		case name == "docker" && strings.HasPrefix(joined, "inspect"):
			return exitedInspectFixture("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return exitedInspectFixture("", "1", strings.Repeat("a", 64), workerID), nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	resource.HeadContainerID = headID
	resource.WorkerContainerID = workerID
	// Pre-set to max attempts so the budget-exhausted path fires immediately.
	resource.RestartAttempts = tensorFoldRestartMaxAttempts
	resource.LastRestartAt = time.Now().UTC().Add(-2 * time.Hour)
	manager := newReconcileManager(t, resource, runner)

	manager.startRecipeContainer = func(_ context.Context, _ tensorFoldResource, _ bool, _ string) error {
		startCount++
		return errors.New("should not be called")
	}

	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile at budget limit: %v", err)
	}
	if startCount != 0 {
		t.Fatalf("start called %d times after budget exhaustion, want 0", startCount)
	}
	got, err := manager.inspect(resource.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	// m5: attempts must not increment on the budget-exhausted path.
	if got.RestartAttempts != tensorFoldRestartMaxAttempts {
		t.Fatalf("RestartAttempts = %d, want %d (must not increment on budget-exhausted path)",
			got.RestartAttempts, tensorFoldRestartMaxAttempts)
	}
}

func TestReconcileStoppingStatusNoRestart(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	var stopCalled, restartCalled bool

	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "docker" && strings.HasPrefix(joined, "ps -a"):
			return []byte(headID + "\n"), nil
		case name == "docker" && strings.HasPrefix(joined, "inspect"):
			return exitedInspectFixture("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return exitedInspectFixture("", "1", strings.Repeat("a", 64), workerID), nil
		case name == "./stop.sh":
			stopCalled = true
			return nil, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "stopping")
	resource.HeadContainerID = headID
	resource.WorkerContainerID = workerID
	manager := newReconcileManager(t, resource, runner)

	manager.startRecipeContainer = func(_ context.Context, _ tensorFoldResource, _ bool, _ string) error {
		restartCalled = true
		return nil
	}

	// An interrupted operator stop (status "stopping", both containers exited)
	// must be left to the stop path: no docker start, no stop.sh, no error.
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if restartCalled {
		t.Fatal("docker start was attempted for a stopping resource")
	}
	if stopCalled {
		t.Fatal("stop.sh was invoked for a stopping resource")
	}
	got, err := manager.inspect(resource.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopping" || got.RestartAttempts != 0 || !got.LastRestartAt.IsZero() {
		t.Fatalf("stopping resource was mutated: status=%q attempts=%d lastRestart=%v",
			got.Status, got.RestartAttempts, got.LastRestartAt)
	}
}

func TestReconcileCrashLoopExhaustsBudget(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	startCount := 0
	phase := 0 // 0 = running, 1 = exited (both ranks)

	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "docker" && strings.HasPrefix(joined, "ps -a"):
			return []byte(headID + "\n"), nil
		case name == "docker" && strings.HasPrefix(joined, "inspect"):
			if phase == 0 {
				return tensorFoldInspectFixtureWithIdentity("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
			}
			return exitedInspectFixture("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			if phase == 0 {
				return tensorFoldInspectFixtureWithIdentity("", "1", strings.Repeat("a", 64), workerID), nil
			}
			return exitedInspectFixture("", "1", strings.Repeat("a", 64), workerID), nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "exited")
	resource.HeadContainerID = headID
	resource.WorkerContainerID = workerID
	manager := newReconcileManager(t, resource, runner)

	manager.startRecipeContainer = func(_ context.Context, _ tensorFoldResource, _ bool, _ string) error {
		startCount++
		return nil
	}

	setLastRestartAt := func(d time.Duration) {
		got, err := manager.inspect(resource.Name)
		if err != nil {
			t.Fatal(err)
		}
		got.LastRestartAt = time.Now().UTC().Add(d)
		manager.mu.Lock()
		defer manager.mu.Unlock()
		if err := manager.writeResource(got); err != nil {
			t.Fatal(err)
		}
	}

	// Crash 1: exited -> restart (attempts 1)
	phase = 1
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile crash 1: %v", err)
	}
	if startCount != 2 {
		t.Fatalf("after crash 1: start calls = %d, want 2", startCount)
	}
	// Briefly running, then crash 2 — inside the stability window the budget
	// must NOT reset, and backoff must be bypassed for the failed status.
	phase = 0
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile running: %v", err)
	}
	got, _ := manager.inspect(resource.Name)
	if got.RestartAttempts != 1 {
		t.Fatalf("budget reset too early: attempts=%d after 2min running, want 1", got.RestartAttempts)
	}
	phase = 1
	setLastRestartAt(-5 * time.Minute) // backoff elapsed, no stability
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile crash 2: %v", err)
	}
	got, _ = manager.inspect(resource.Name)
	if got.RestartAttempts != 2 {
		t.Fatalf("after crash 2: attempts=%d, want 2", got.RestartAttempts)
	}
	// Briefly running again, then crash 3 — still within the window.
	phase = 0
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile running: %v", err)
	}
	phase = 1
	setLastRestartAt(-5 * time.Minute)
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile crash 3: %v", err)
	}
	got, _ = manager.inspect(resource.Name)
	if got.RestartAttempts != 3 {
		t.Fatalf("after crash 3: attempts=%d, want 3", got.RestartAttempts)
	}
	// Budget exhausted: next pass marks failed without any further start.
	setLastRestartAt(-5 * time.Minute)
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile after budget: %v", err)
	}
	got, _ = manager.inspect(resource.Name)
	if got.Status != "failed" {
		t.Fatalf("crash-loop did not reach failed: status=%q attempts=%d", got.Status, got.RestartAttempts)
	}
	if startCount != 6 {
		t.Fatalf("start calls after crash-loop = %d, want 6 (2 per attempt, none after budget)", startCount)
	}
}

func TestReconcileMissingContainerIDReports(t *testing.T) {
	workerID := strings.Repeat("2", 64)
	var stopCalled bool

	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "docker" && strings.HasPrefix(joined, "ps -a"):
			// head container exists but head container ID was never recorded
			return []byte(strings.Repeat("1", 64) + "\n"), nil
		case name == "docker" && strings.HasPrefix(joined, "inspect"):
			return exitedInspectFixture("192.168.1.191", "0", strings.Repeat("a", 64), strings.Repeat("1", 64)), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return exitedInspectFixture("", "1", strings.Repeat("a", 64), workerID), nil
		case name == "./stop.sh":
			stopCalled = true
			return nil, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	// Head container ID is missing; worker ID is recorded.
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "exited")
	resource.WorkerContainerID = workerID
	manager := newReconcileManager(t, resource, runner)

	var restartCalled bool
	manager.startRecipeContainer = func(_ context.Context, _ tensorFoldResource, _ bool, _ string) error {
		restartCalled = true
		return nil
	}

	err := manager.reconcile(context.Background())
	if err == nil {
		t.Fatal("reconcile should report the missing head container ID")
	}
	if !strings.Contains(err.Error(), "head container ID") {
		t.Fatalf("error should name the missing head container ID: %v", err)
	}
	if restartCalled {
		t.Fatal("docker start must not be attempted without a recorded ID")
	}
	if stopCalled {
		t.Fatal("stop.sh must not be called when the ID is missing")
	}
	got, inspectErr := manager.inspect(resource.Name)
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	if got.RestartAttempts != 0 {
		t.Fatalf("missing ID must not consume restart budget: attempts=%d", got.RestartAttempts)
	}
}

func TestReconcilePeerUnreachableSkips(t *testing.T) {
	headID := strings.Repeat("1", 64)
	var stopCalled bool
	var restartCalled bool

	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "docker" && strings.HasPrefix(joined, "ps -a"):
			return []byte(headID + "\n"), nil
		case name == "docker" && strings.HasPrefix(joined, "inspect"):
			return exitedInspectFixture("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh":
			return nil, errors.New("ssh: connection refused")
		case name == "./stop.sh":
			stopCalled = true
			return nil, nil
		default:
			return nil, nil
		}
	})

	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	resource.HeadContainerID = headID
	resource.WorkerContainerID = strings.Repeat("2", 64)
	manager := newReconcileManager(t, resource, runner)

	manager.startRecipeContainer = func(_ context.Context, _ tensorFoldResource, _ bool, _ string) error {
		restartCalled = true
		return nil
	}

	// reconcile reports the worker inspect error; resource must be left untouched.
	err := manager.reconcile(context.Background())
	if err == nil {
		t.Fatal("reconcile should report the worker inspect error")
	}

	if stopCalled {
		t.Fatal("stop.sh was called when peer is unreachable")
	}
	if restartCalled {
		t.Fatal("docker-start was called when peer is unreachable")
	}
	got, inspectErr := manager.inspect(resource.Name)
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	if got.Status != "running" || got.RestartAttempts != 0 {
		t.Fatalf("resource was mutated while peer unreachable: status=%q attempts=%d", got.Status, got.RestartAttempts)
	}
}

func TestReconcileSingleContainerStillCleansUp(t *testing.T) {
	headID := strings.Repeat("1", 64)
	var stopCalled bool
	var restartCalled bool
	headRemoved := false
	var rmCalled bool

	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "docker" && strings.HasPrefix(joined, "ps -a"):
			if headRemoved {
				return []byte(""), nil
			}
			return []byte(headID + "\n"), nil
		case name == "docker" && strings.HasPrefix(joined, "inspect"):
			if headRemoved {
				return nil, errors.New("No such object")
			}
			return tensorFoldInspectFixtureWithIdentity("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "docker" && strings.HasPrefix(joined, "stop "):
			return nil, nil
		case name == "docker" && strings.HasPrefix(joined, "rm "):
			headRemoved = true
			rmCalled = true
			return []byte(headID + "\n"), nil
		case name == "ssh":
			// Worker never existed; docker ps via ssh returns nothing.
			return []byte(""), nil
		case name == "./stop.sh":
			stopCalled = true
			return nil, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	resource.HeadContainerID = headID
	manager := newReconcileManager(t, resource, runner)

	// Stage the pinned recipe so stopGeneration can run.
	stageTensorFoldExecutionFixture(t, manager, resource)
	manager.fenceProcesses = func(marker string) error { return nil }

	manager.startRecipeContainer = func(_ context.Context, _ tensorFoldResource, _ bool, _ string) error {
		restartCalled = true
		return nil
	}

	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if restartCalled {
		t.Fatal("restart was attempted for a single-container resource; should have cleaned up")
	}
	if stopCalled {
		t.Fatal("stop.sh should NOT be called when only one container exists (it's for both-rank cleanup)")
	}
	if !rmCalled {
		t.Fatal("docker rm was not called — the single-container cleanup did not run")
	}
	got, _ := manager.inspect(resource.Name)
	if got.Status != "stopped" {
		t.Fatalf("resource status = %q, want stopped after partial-start cleanup", got.Status)
	}
}
