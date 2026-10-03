package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
)

type blockingTensorFoldProcess struct {
	stopOnce   sync.Once
	stopCalled chan struct{}
	release    chan struct{}
}

func (p *blockingTensorFoldProcess) Wait() error {
	<-p.release
	return nil
}

func (p *blockingTensorFoldProcess) Stop() error {
	p.stopOnce.Do(func() { close(p.stopCalled) })
	return nil
}
func (p *blockingTensorFoldProcess) Kill() error { return p.Stop() }

func TestTensorFoldReconcilePreservesLivePreparationPhases(t *testing.T) {
	for _, test := range []struct {
		name       string
		workerOnly bool
	}{
		{name: "before containers"},
		{name: "worker first", workerOnly: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var upstreamStop bool
			runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
				joined := name + " " + strings.Join(args, " ")
				switch {
				case name == "docker" && len(args) > 0 && args[0] == "ps":
					return nil, nil
				case name == "ssh" && strings.Contains(joined, "docker ps"):
					if test.workerOnly {
						return []byte(strings.Repeat("2", 64) + "\n"), nil
					}
					return nil, nil
				case name == "ssh" && strings.Contains(joined, "docker inspect"):
					return tensorFoldInspectFixture("", "1"), nil
				case name == "./stop.sh":
					upstreamStop = true
					return nil, nil
				default:
					return nil, fmt.Errorf("unexpected command: %s", joined)
				}
			})
			manager := newTensorFoldManager(t.TempDir(), runner)
			resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
			if err := manager.writeResource(resource); err != nil {
				t.Fatal(err)
			}
			manager.processes[resource.Name] = &tensorFoldLaunch{id: resource.LaunchID, process: &fakeTensorFoldProcess{done: make(chan error, 1)}, done: make(chan struct{})}

			if err := manager.reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			got, err := manager.inspect(resource.Name)
			if err != nil || got.Status != "starting" || upstreamStop {
				t.Fatalf("live preparation was disrupted: resource=%#v upstream_stop=%v err=%v", got, upstreamStop, err)
			}
			if test.workerOnly && got.WorkerContainerID != strings.Repeat("2", 64) {
				t.Fatalf("worker-first ownership was not recorded: %#v", got)
			}
		})
	}
}

func TestTensorFoldActiveReconcileReportsInspectFailureWithoutCleanup(t *testing.T) {
	var upstreamStop bool
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		if name == "docker" && len(args) > 0 && args[0] == "ps" {
			return nil, errors.New("docker unavailable")
		}
		if name == "ssh" && strings.Contains(joined, "docker ps") {
			return nil, errors.New("ssh unavailable")
		}
		if name == "./stop.sh" {
			upstreamStop = true
		}
		return nil, nil
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	manager.processes[resource.Name] = &tensorFoldLaunch{id: resource.LaunchID, process: &fakeTensorFoldProcess{done: make(chan error, 1)}, done: make(chan struct{})}
	if err := manager.reconcile(context.Background()); err == nil || upstreamStop {
		t.Fatalf("active inspect failure was hidden or triggered cleanup: upstream=%v err=%v", upstreamStop, err)
	}
	got, err := manager.inspect(resource.Name)
	if err != nil || got.Status != "starting" {
		t.Fatalf("active resource was mutated after observation failure: resource=%#v err=%v", got, err)
	}
}

func TestTensorFoldReservationIsSingletonAcrossResourceNames(t *testing.T) {
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		if name == "docker" && len(args) > 0 && args[0] == "ps" {
			return nil, nil
		}
		if name == "docker" && len(args) > 1 && args[0] == "image" && args[1] == "inspect" {
			return []byte(bkc.GLM53FlashEXL3TensorFoldImagePatchHash + "\n"), nil
		}
		return []byte("ok\n"), nil
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	first := validTensorFoldResourceRequest(false)
	if _, err := manager.create(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.DeploymentID = "other"
	second.Name = "yokai-deployment-other-g1-worker"
	if _, err := manager.create(context.Background(), second); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("second singleton reservation was accepted: %v", err)
	}
}

func TestTensorFoldConcurrentSingletonReservationHasOneWinner(t *testing.T) {
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		if name == "docker" && len(args) > 0 && args[0] == "ps" {
			return nil, nil
		}
		if name == "docker" && len(args) > 1 && args[0] == "image" && args[1] == "inspect" {
			return []byte(bkc.GLM53FlashEXL3TensorFoldImagePatchHash + "\n"), nil
		}
		return []byte("ok\n"), nil
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	requests := []tensorFoldResourceRequest{validTensorFoldResourceRequest(false), validTensorFoldResourceRequest(false)}
	requests[1].DeploymentID = "other"
	requests[1].Name = "yokai-deployment-other-g1-worker"
	results := make(chan error, len(requests))
	var start sync.WaitGroup
	start.Add(1)
	for _, request := range requests {
		request := request
		go func() {
			start.Wait()
			_, err := manager.create(context.Background(), request)
			results <- err
		}()
	}
	start.Done()
	successes := 0
	for range requests {
		if err := <-results; err == nil {
			successes++
		} else if !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("unexpected concurrent reservation result: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent singleton successes=%d, want 1", successes)
	}
}

func TestTensorFoldStaleWaiterCannotOverwriteNewLaunch(t *testing.T) {
	manager := newTensorFoldManager(t.TempDir(), tensorFoldRunnerFunc(func(context.Context, string, []string, string, ...string) ([]byte, error) {
		return nil, errors.New("unexpected command")
	}))
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	resource.LaunchID = strings.Repeat("b", 64)
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	newLaunch := &tensorFoldLaunch{id: resource.LaunchID, process: &fakeTensorFoldProcess{done: make(chan error, 1)}, done: make(chan struct{})}
	manager.processes[resource.Name] = newLaunch
	oldProcess := &fakeTensorFoldProcess{done: make(chan error, 1)}
	oldProcess.done <- nil
	oldLaunch := &tensorFoldLaunch{id: strings.Repeat("a", 64), process: oldProcess, done: make(chan struct{})}
	logFile, err := os.CreateTemp(t.TempDir(), "old-launch")
	if err != nil {
		t.Fatal(err)
	}
	go manager.waitForProcess(resource.Name, oldLaunch, logFile)
	select {
	case <-oldLaunch.done:
	case <-time.After(time.Second):
		t.Fatal("old waiter did not finish")
	}
	manager.mu.Lock()
	currentLaunch := manager.processes[resource.Name]
	manager.mu.Unlock()
	got, err := manager.inspect(resource.Name)
	if err != nil || currentLaunch != newLaunch || got.LaunchID != resource.LaunchID || got.Status != "starting" {
		t.Fatalf("stale waiter changed new launch: launch=%p want=%p resource=%#v err=%v", currentLaunch, newLaunch, got, err)
	}
}

func TestTensorFoldWaiterCleanupCannotStopReplacementLaunch(t *testing.T) {
	manager := newTensorFoldManager(t.TempDir(), tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("replacement reached stale cleanup command: %s %s", name, strings.Join(args, " "))
	}))
	oldResource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	if err := manager.writeResource(oldResource); err != nil {
		t.Fatal(err)
	}
	oldProcess := &fakeTensorFoldProcess{done: make(chan error, 1)}
	oldProcess.done <- nil
	oldLaunch := &tensorFoldLaunch{id: oldResource.LaunchID, process: oldProcess, done: make(chan struct{})}
	manager.processes[oldResource.Name] = oldLaunch
	cleanupReady := make(chan struct{})
	releaseCleanup := make(chan struct{})
	manager.beforeWaiterCleanup = func() {
		close(cleanupReady)
		<-releaseCleanup
	}
	logFile, err := os.CreateTemp(t.TempDir(), "old-launch")
	if err != nil {
		t.Fatal(err)
	}
	waiterReturned := make(chan struct{})
	go func() {
		manager.waitForProcess(oldResource.Name, oldLaunch, logFile)
		close(waiterReturned)
	}()
	<-cleanupReady

	replacement := oldResource
	replacement.LaunchID = strings.Repeat("b", 64)
	replacement.Status = "starting"
	newProcess := &fakeTensorFoldProcess{done: make(chan error, 1)}
	newLaunch := &tensorFoldLaunch{id: replacement.LaunchID, process: newProcess, done: make(chan struct{})}
	manager.mu.Lock()
	if err := manager.writeResource(replacement); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.processes[replacement.Name] = newLaunch
	manager.mu.Unlock()
	close(releaseCleanup)
	select {
	case <-waiterReturned:
	case <-time.After(time.Second):
		t.Fatal("old waiter cleanup did not finish")
	}
	got, err := manager.inspect(replacement.Name)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	currentLaunch := manager.processes[replacement.Name]
	manager.mu.Unlock()
	if got.LaunchID != replacement.LaunchID || got.Status != replacement.Status || currentLaunch != newLaunch {
		t.Fatalf("stale waiter cleanup changed replacement: resource=%#v launch=%p want=%p", got, currentLaunch, newLaunch)
	}
}

func TestTensorFoldStopJoinsLauncherBeforeDestructiveCleanup(t *testing.T) {
	var mu sync.Mutex
	upstreamStop := false
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case (name == "docker" || name == "ssh") && strings.Contains(joined, "docker ps"):
			return nil, nil
		case name == "./stop.sh":
			mu.Lock()
			upstreamStop = true
			mu.Unlock()
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command: %s", joined)
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	stageTensorFoldExecutionFixture(t, manager, resource)
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	process := &blockingTensorFoldProcess{stopCalled: make(chan struct{}), release: make(chan struct{})}
	launch := &tensorFoldLaunch{id: resource.LaunchID, process: process, done: make(chan struct{})}
	manager.processes[resource.Name] = launch
	logFile, err := os.CreateTemp(t.TempDir(), "launcher-log")
	if err != nil {
		t.Fatal(err)
	}
	go manager.waitForProcess(resource.Name, launch, logFile)
	result := make(chan error, 1)
	go func() { result <- manager.stop(context.Background(), resource.Name) }()

	select {
	case <-process.stopCalled:
	case <-time.After(time.Second):
		t.Fatal("stop did not signal the active launcher")
	}
	mu.Lock()
	ranBeforeJoin := upstreamStop
	mu.Unlock()
	if ranBeforeJoin {
		t.Fatal("upstream name-based cleanup ran before launcher join")
	}
	select {
	case err := <-result:
		t.Fatalf("stop returned before launcher was reaped: %v", err)
	default:
	}
	close(process.release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stop did not complete after launcher reap")
	}
}

func TestTensorFoldHeadResourceRequiresDurableLaunchIdentity(t *testing.T) {
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	resource.LaunchID = ""
	if err := validateStoredTensorFoldResource(resource, resource.Name); err == nil {
		t.Fatal("head resource without an unforgeable launch identity was accepted")
	}
}

func TestTensorFoldContainerRejectsForgedLaunchMarker(t *testing.T) {
	forgedID := strings.Repeat("3", 64)
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		if name == "docker" && len(args) > 0 && args[0] == "ps" {
			return []byte(forgedID + "\n"), nil
		}
		if name == "docker" && len(args) > 0 && args[0] == "inspect" {
			fixture := tensorFoldInspectFixtureWithIdentity("192.168.1.191", "0", strings.Repeat("a", 64), forgedID)
			return mutateTensorFoldInspectEnvironment(fixture, "TENSORFOLD_YOKAI_LAUNCH_ID", strings.Repeat("f", 64)), nil
		}
		return nil, errors.New("unexpected command")
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	if err := manager.verifyRecipeContainer(context.Background(), resource, false); err == nil {
		t.Fatal("matching image and argv with a forged launch marker was accepted")
	}
}

func TestTensorFoldObservePersistsExactContainerIDs(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case name == "docker" && len(args) > 0 && args[0] == "ps":
			return []byte(headID + "\n"), nil
		case name == "docker" && len(args) > 0 && args[0] == "inspect":
			return tensorFoldInspectFixtureWithIdentity("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return tensorFoldInspectFixtureWithIdentity("", "1", strings.Repeat("a", 64), workerID), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s", joined)
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	observed, err := manager.observe(context.Background(), resource.Name)
	if err != nil {
		t.Fatal(err)
	}
	if observed.HeadContainerID != headID || observed.WorkerContainerID != workerID {
		t.Fatalf("exact ownership was not persisted: %#v", observed)
	}
}

func TestTensorFoldObserveCannotOverwriteConcurrentRestart(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	observationStarted := make(chan struct{})
	releaseObservation := make(chan struct{})
	var startOnce sync.Once
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case name == "docker" && len(args) > 0 && args[0] == "ps":
			startOnce.Do(func() { close(observationStarted) })
			<-releaseObservation
			return []byte(headID + "\n"), nil
		case name == "docker" && len(args) > 0 && args[0] == "inspect":
			return tensorFoldInspectFixtureWithIdentity("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return tensorFoldInspectFixtureWithIdentity("", "1", strings.Repeat("a", 64), workerID), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s", joined)
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	stale := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	if err := manager.writeResource(stale); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := manager.observe(context.Background(), stale.Name)
		result <- err
	}()
	<-observationStarted

	restarted := stale
	restarted.LaunchID = strings.Repeat("b", 64)
	restarted.Status = "starting"
	manager.mu.Lock()
	if err := manager.writeResource(restarted); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.mu.Unlock()
	close(releaseObservation)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	got, err := manager.inspect(stale.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.LaunchID != restarted.LaunchID || got.Status != restarted.Status || got.HeadContainerID != "" || got.WorkerContainerID != "" {
		t.Fatalf("stale observation overwrote restarted launch: %#v", got)
	}
}

func TestTensorFoldReconcileCannotOverwriteConcurrentRestart(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	observationStarted := make(chan struct{})
	releaseObservation := make(chan struct{})
	var startOnce sync.Once
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case name == "docker" && len(args) > 0 && args[0] == "ps":
			startOnce.Do(func() { close(observationStarted) })
			<-releaseObservation
			return []byte(headID + "\n"), nil
		case name == "docker" && len(args) > 0 && args[0] == "inspect":
			return tensorFoldInspectFixtureWithIdentity("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return tensorFoldInspectFixtureWithIdentity("", "1", strings.Repeat("a", 64), workerID), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s", joined)
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	stale := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	if err := manager.writeResource(stale); err != nil {
		t.Fatal(err)
	}
	oldLaunch := &tensorFoldLaunch{id: stale.LaunchID, process: &fakeTensorFoldProcess{done: make(chan error, 1)}, done: make(chan struct{})}
	manager.processes[stale.Name] = oldLaunch
	result := make(chan error, 1)
	go func() { result <- manager.reconcile(context.Background()) }()
	<-observationStarted

	restarted := stale
	restarted.LaunchID = strings.Repeat("b", 64)
	restarted.Status = "starting"
	newLaunch := &tensorFoldLaunch{id: restarted.LaunchID, process: &fakeTensorFoldProcess{done: make(chan error, 1)}, done: make(chan struct{})}
	manager.mu.Lock()
	if err := manager.writeResource(restarted); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.processes[stale.Name] = newLaunch
	manager.mu.Unlock()
	close(releaseObservation)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	got, err := manager.inspect(stale.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.LaunchID != restarted.LaunchID || got.Status != restarted.Status || got.HeadContainerID != "" || got.WorkerContainerID != "" {
		t.Fatalf("stale reconciliation overwrote restarted launch: %#v", got)
	}
}

func TestTensorFoldPartialReconcileCannotStopConcurrentRestart(t *testing.T) {
	headID := strings.Repeat("1", 64)
	observationStarted := make(chan struct{})
	releaseObservation := make(chan struct{})
	var startOnce sync.Once
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case name == "docker" && len(args) > 0 && args[0] == "ps":
			startOnce.Do(func() { close(observationStarted) })
			<-releaseObservation
			return []byte(headID + "\n"), nil
		case name == "docker" && len(args) > 0 && args[0] == "inspect":
			return tensorFoldInspectFixtureWithIdentity("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return nil, nil
		default:
			return nil, fmt.Errorf("replacement reached stale cleanup command: %s", joined)
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	stale := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	if err := manager.writeResource(stale); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- manager.reconcile(context.Background()) }()
	<-observationStarted

	restarted := stale
	restarted.LaunchID = strings.Repeat("b", 64)
	restarted.Status = "starting"
	replacementProcess := &blockingTensorFoldProcess{stopCalled: make(chan struct{}), release: make(chan struct{})}
	replacementLaunch := &tensorFoldLaunch{id: restarted.LaunchID, process: replacementProcess, done: make(chan struct{})}
	close(replacementLaunch.done)
	manager.mu.Lock()
	if err := manager.writeResource(restarted); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.processes[restarted.Name] = replacementLaunch
	manager.mu.Unlock()
	close(releaseObservation)
	reconcileErr := <-result

	select {
	case <-replacementProcess.stopCalled:
		t.Fatalf("stale partial reconciliation killed the replacement launch: %v", reconcileErr)
	default:
	}
	if reconcileErr != nil {
		t.Fatalf("stale partial reconciliation failed after concurrent restart: %v", reconcileErr)
	}
	got, err := manager.inspect(restarted.Name)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	currentLaunch := manager.processes[restarted.Name]
	manager.mu.Unlock()
	if got.LaunchID != restarted.LaunchID || got.Status != restarted.Status || currentLaunch != replacementLaunch {
		t.Fatalf("stale partial reconciliation stopped replacement: resource=%#v launch=%p want=%p", got, currentLaunch, replacementLaunch)
	}
}

func TestTensorFoldStopRefusesSameNameReplacementAfterOwnershipCapture(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	foreignID := strings.Repeat("3", 64)
	var upstreamStop bool
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case name == "docker" && len(args) > 0 && args[0] == "ps":
			return []byte(foreignID + "\n"), nil
		case name == "docker" && len(args) > 0 && args[0] == "inspect":
			return tensorFoldInspectFixtureWithIdentity("192.168.1.191", "0", strings.Repeat("a", 64), foreignID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return tensorFoldInspectFixtureWithIdentity("", "1", strings.Repeat("a", 64), workerID), nil
		case name == "./stop.sh":
			upstreamStop = true
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command: %s", joined)
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	resource.HeadContainerID = headID
	resource.WorkerContainerID = workerID
	stageTensorFoldExecutionFixture(t, manager, resource)
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	if err := manager.stop(context.Background(), resource.Name); err == nil || upstreamStop {
		t.Fatalf("same-name replacement reached destructive cleanup: upstream=%v err=%v", upstreamStop, err)
	}
}

func TestTensorFoldPartialStopUsesExactIDWithoutNameBasedScript(t *testing.T) {
	workerID := strings.Repeat("2", 64)
	workerPresent := true
	var upstreamStop bool
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case name == "docker" && len(args) > 0 && args[0] == "ps":
			return nil, nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			if workerPresent {
				return []byte(workerID + "\n"), nil
			}
			return nil, nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return tensorFoldInspectFixtureWithIdentity("", "1", strings.Repeat("a", 64), workerID), nil
		case name == "ssh" && strings.Contains(joined, "docker stop "+workerID):
			return nil, nil
		case name == "ssh" && strings.Contains(joined, "docker rm "+workerID):
			workerPresent = false
			return nil, nil
		case name == "./stop.sh":
			upstreamStop = true
			workerPresent = false
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command: %s", joined)
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	stageTensorFoldExecutionFixture(t, manager, resource)
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	if err := manager.stop(context.Background(), resource.Name); err != nil {
		t.Fatal(err)
	}
	if upstreamStop || workerPresent {
		t.Fatalf("partial cleanup used unsafe name script or left worker: upstream=%v present=%v", upstreamStop, workerPresent)
	}
}

func TestTensorFoldStopChecksDurableIDAbsenceAfterUpstreamSuccess(t *testing.T) {
	headID := strings.Repeat("1", 64)
	workerID := strings.Repeat("2", 64)
	beforeStop := true
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		isExactWorkerQuery := strings.Contains(joined, "filter id="+workerID)
		switch {
		case name == "docker" && len(args) > 0 && args[0] == "ps":
			if beforeStop {
				return []byte(headID + "\n"), nil
			}
			return nil, nil
		case name == "docker" && len(args) > 0 && args[0] == "inspect":
			return tensorFoldInspectFixtureWithIdentity("192.168.1.191", "0", strings.Repeat("a", 64), headID), nil
		case name == "ssh" && strings.Contains(joined, "docker ps") && isExactWorkerQuery:
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			if beforeStop {
				return []byte(workerID + "\n"), nil
			}
			return nil, nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return tensorFoldInspectFixtureWithIdentity("", "1", strings.Repeat("a", 64), workerID), nil
		case name == "./stop.sh":
			beforeStop = false
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command: %s", joined)
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	resource.HeadContainerID = headID
	resource.WorkerContainerID = workerID
	stageTensorFoldExecutionFixture(t, manager, resource)
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	if err := manager.stop(context.Background(), resource.Name); err == nil {
		t.Fatal("upstream zero exit released reservation while durable worker ID remained")
	}
	got, err := manager.inspect(resource.Name)
	if err != nil || got.Status == "stopped" {
		t.Fatalf("durable ID uncertainty released reservation: resource=%#v err=%v", got, err)
	}
}

func TestTensorFoldStopVerifiesBothNodesAbsent(t *testing.T) {
	stopRan := false
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case name == "docker" && len(args) > 0 && args[0] == "ps":
			return nil, nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(strings.Repeat("2", 64) + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return tensorFoldInspectFixture("", "1"), nil
		case name == "ssh" && strings.Contains(joined, "docker stop "):
			return nil, nil
		case name == "ssh" && strings.Contains(joined, "docker rm "):
			return nil, nil
		case name == "./stop.sh":
			stopRan = true
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command: %s", joined)
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	stageTensorFoldExecutionFixture(t, manager, resource)
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	if err := manager.stop(context.Background(), resource.Name); err == nil || stopRan {
		t.Fatalf("stop accepted a worker that remained present: stop_ran=%v err=%v", stopRan, err)
	}
	got, err := manager.inspect(resource.Name)
	if err != nil || got.Status == "stopped" {
		t.Fatalf("uncertain worker state released the reservation: resource=%#v err=%v", got, err)
	}
}

func TestTensorFoldStopRevalidatesExecutableSources(t *testing.T) {
	var stopRan bool
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case (name == "docker" || name == "ssh") && strings.Contains(joined, "docker ps"):
			return nil, nil
		case name == "./stop.sh":
			stopRan = true
			return nil, nil
		default:
			return nil, nil
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	recipeDir := manager.recipePath()
	if err := os.MkdirAll(filepath.Join(recipeDir, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("#!/bin/sh\n")
	if err := os.WriteFile(filepath.Join(recipeDir, "stop.sh"), body, 0o770); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	manager.pinnedFiles = map[string]string{"stop.sh": hex.EncodeToString(sum[:])}
	request := validTensorFoldResourceRequest(true)
	environment, err := renderTensorFoldRecipeEnvironment(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recipeDir, ".env"), []byte(environment), 0o600); err != nil {
		t.Fatal(err)
	}
	resource := resourceFromTensorFoldRequest(request, "running")
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}

	if err := manager.stop(context.Background(), resource.Name); err == nil || stopRan {
		t.Fatalf("group-writable stop source reached execution: stop_ran=%v err=%v", stopRan, err)
	}
}

func TestTensorFoldStateRejectsSymlinkedParent(t *testing.T) {
	base := t.TempDir()
	realParent := filepath.Join(base, "real")
	if err := os.Mkdir(realParent, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "linked")
	if err := os.Symlink(realParent, link); err != nil {
		t.Fatal(err)
	}
	if err := ensureOwnedTensorFoldDirectory(filepath.Join(link, "resources")); err == nil {
		t.Fatal("symlinked state parent was accepted")
	}
}

func TestTensorFoldResourceReadRejectsSymlinkedStateParent(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real")
	realManager := newTensorFoldManager(realRoot, &fakeTensorFoldRunner{})
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(false), "running")
	if err := realManager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	linkedRoot := filepath.Join(base, "linked")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Fatal(err)
	}
	manager := newTensorFoldManager(linkedRoot, &fakeTensorFoldRunner{})
	if _, err := manager.inspect(resource.Name); err == nil {
		t.Fatal("resource record was read through a symlinked state parent")
	}
}

func TestTensorFoldEnvironmentWriteRejectsSymlinkedRecipeParent(t *testing.T) {
	base := t.TempDir()
	manager := newTensorFoldManager(filepath.Join(base, "state"), &fakeTensorFoldRunner{})
	realRecipes := filepath.Join(base, "real-recipes")
	if err := os.MkdirAll(filepath.Join(realRecipes, bkc.GLM53FlashEXL3TensorFoldRecipeCommit), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(manager.root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRecipes, filepath.Join(manager.root, "recipes")); err != nil {
		t.Fatal(err)
	}
	if err := manager.writeRecipeEnvironment("owned=value\n"); err == nil {
		t.Fatal("recipe environment was written through a symlinked parent")
	}
}

func TestTensorFoldLogsRejectSymlinkTarget(t *testing.T) {
	manager := newTensorFoldManager(t.TempDir(), &fakeTensorFoldRunner{})
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	if err := os.MkdirAll(filepath.Dir(manager.logPath(resource.Name)), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "foreign.log")
	if err := os.WriteFile(target, []byte("foreign\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, manager.logPath(resource.Name)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.logs(context.Background(), resource); err == nil {
		t.Fatal("head logs followed a symlink target")
	}
}

func TestTensorFoldContainerPolicyRejectsConflictingFlagsAndForgedModelPosition(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{
			name: "conflicting duplicate",
			mutate: func(data []byte) []byte {
				return mutateTensorFoldInspectCommand(data, func(command []string) []string {
					return append(command, "--context=524288")
				})
			},
		},
		{
			name: "model revision outside model position",
			mutate: func(data []byte) []byte {
				return mutateTensorFoldInspectCommand(data, func(command []string) []string {
					command[1] = "/tmp/forged-model"
					return append(command, "--note", "/snapshots/"+bkc.GLM53FlashEXL3TensorFoldModelRevision)
				})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
				if name == "docker" && len(args) > 0 && args[0] == "ps" {
					return []byte("head-id\n"), nil
				}
				if name == "docker" && len(args) > 0 && args[0] == "inspect" {
					return test.mutate(tensorFoldInspectFixture("192.168.1.191", "0")), nil
				}
				return nil, errors.New("unexpected command")
			})
			manager := newTensorFoldManager(t.TempDir(), runner)
			resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
			if err := manager.verifyRecipeContainer(context.Background(), resource, false); err == nil {
				t.Fatal("conflicting or position-forged container command passed policy validation")
			}
		})
	}
}

func mutateTensorFoldInspectCommand(data []byte, mutate func([]string) []string) []byte {
	var records []map[string]any
	if err := json.Unmarshal(data, &records); err != nil {
		panic(err)
	}
	config := records[0]["Config"].(map[string]any)
	rawCommand := config["Cmd"].([]any)
	command := make([]string, len(rawCommand))
	for index := range rawCommand {
		command[index] = rawCommand[index].(string)
	}
	config["Cmd"] = mutate(command)
	result, err := json.Marshal(records)
	if err != nil {
		panic(err)
	}
	return bytes.Clone(result)
}

func mutateTensorFoldInspectEnvironment(data []byte, key, value string) []byte {
	var records []map[string]any
	if err := json.Unmarshal(data, &records); err != nil {
		panic(err)
	}
	config := records[0]["Config"].(map[string]any)
	rawEnvironment := config["Env"].([]any)
	environment := make([]string, 0, len(rawEnvironment)+1)
	for _, raw := range rawEnvironment {
		item := raw.(string)
		if !strings.HasPrefix(item, key+"=") {
			environment = append(environment, item)
		}
	}
	environment = append(environment, key+"="+value)
	config["Env"] = environment
	result, err := json.Marshal(records)
	if err != nil {
		panic(err)
	}
	return result
}
