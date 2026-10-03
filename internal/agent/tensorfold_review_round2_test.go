package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func noTensorFoldContainersRunner(t *testing.T) tensorFoldRunnerFunc {
	return func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		if strings.Contains(joined, "docker ps") {
			return nil, nil
		}
		t.Errorf("unexpected command: %s", joined)
		return nil, fmt.Errorf("unexpected command: %s", joined)
	}
}

func TestTensorFoldReconcileFencesSurvivingLauncherBeforeRelease(t *testing.T) {
	manager := newTensorFoldManager(t.TempDir(), noTensorFoldContainersRunner(t))
	// The agent restarted mid-preparation: the record says starting, no
	// launcher is tracked in memory, and no container exists yet.
	head := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	if err := manager.writeResource(head); err != nil {
		t.Fatal(err)
	}
	var fenced []string
	fenceErr := errors.New("launcher still alive")
	manager.fenceProcesses = func(marker string) error {
		fenced = append(fenced, marker)
		return fenceErr
	}
	if err := manager.reconcile(context.Background()); err == nil {
		t.Fatal("reconcile hid a launcher it could not fence")
	}
	if current, err := manager.inspect(head.Name); err != nil || current.Status != "starting" {
		t.Fatalf("generation was released while its launcher may still run: resource=%#v err=%v", current, err)
	}
	fenceErr = nil
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if current, err := manager.inspect(head.Name); err != nil || current.Status != "stopped" {
		t.Fatalf("fenced generation was not released: resource=%#v err=%v", current, err)
	}
	want := manager.tensorFoldLaunchMarker(head.LaunchID)
	if len(fenced) != 2 || fenced[0] != want || fenced[1] != want {
		t.Fatalf("fence did not target this launch: %v", fenced)
	}
}

func TestTensorFoldStopFencesLauncherNotTrackedInMemory(t *testing.T) {
	manager := newTensorFoldManager(t.TempDir(), noTensorFoldContainersRunner(t))
	head := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	stageTensorFoldExecutionFixture(t, manager, head)
	if err := manager.writeResource(head); err != nil {
		t.Fatal(err)
	}
	manager.fenceProcesses = func(string) error { return errors.New("launcher still alive") }
	if err := manager.stop(context.Background(), head.Name); err == nil || !strings.Contains(err.Error(), "fence") {
		t.Fatalf("stop proceeded without fencing a surviving launcher: %v", err)
	}
}

func TestTensorFoldResourceIDSurvivesMetricsMerge(t *testing.T) {
	id := tensorFoldResourceID("yokai-deployment-test-g1-head")
	merged := mergeContainerMetrics(nil, []Container{{ID: id, Name: "yokai-deployment-test-g1-head", Status: "stopped"}})
	if len(merged) != 1 || merged[0].ID != id {
		t.Fatalf("TensorFold resource ID was truncated: %#v", merged)
	}
}

func TestTensorFoldServiceInventoryOmitsWorkerReservation(t *testing.T) {
	manager := newTensorFoldManager(t.TempDir(), noTensorFoldContainersRunner(t))
	if err := manager.writeResource(resourceFromTensorFoldRequest(validTensorFoldResourceRequest(false), "running")); err != nil {
		t.Fatal(err)
	}
	all, err := manager.inventory(context.Background())
	if err != nil || len(all) != 1 || all[0].Status != "reserved" {
		t.Fatalf("worker reservation missing from full inventory: %#v err=%v", all, err)
	}
	services, err := manager.serviceInventory(context.Background())
	if err != nil || len(services) != 0 {
		t.Fatalf("worker reservation reported as an inference service: %#v err=%v", services, err)
	}
	if body := renderPrometheusMetrics(&SystemMetrics{}, services); strings.Contains(body, "yokai_service_up{") {
		t.Fatalf("worker reservation emitted a service-up sample:\n%s", body)
	}
}

func TestTensorFoldWorkerOwnershipToleratesCrossHostClockSkew(t *testing.T) {
	head := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	skewed := func(fixture []byte) []byte {
		var records []map[string]any
		if err := json.Unmarshal(fixture, &records); err != nil {
			t.Fatal(err)
		}
		records[0]["Created"] = head.LaunchStartedAt.Add(-30 * time.Second).Format(time.RFC3339Nano)
		data, _ := json.Marshal(records)
		return data
	}
	workerID, headID := strings.Repeat("2", 64), strings.Repeat("1", 64)
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return []byte(workerID + "\n"), nil
		case name == "ssh" && strings.Contains(joined, "docker inspect"):
			return skewed(tensorFoldInspectFixtureWithIdentity("", "1", head.LaunchID, workerID)), nil
		case name == "docker" && args[0] == "ps":
			return []byte(headID + "\n"), nil
		case name == "docker" && args[0] == "inspect":
			return skewed(tensorFoldInspectFixtureWithIdentity(head.ServiceAddress, "0", head.LaunchID, headID)), nil
		}
		return nil, fmt.Errorf("unexpected command: %s", joined)
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	if exists, _, _, err := manager.inspectRecipeContainerIdentity(context.Background(), head, true); err != nil || !exists {
		t.Fatalf("worker clock skew rejected a container carrying this launch ID: exists=%v err=%v", exists, err)
	}
	if _, _, _, err := manager.inspectRecipeContainerIdentity(context.Background(), head, false); err == nil || !strings.Contains(err.Error(), "launch generation") {
		t.Fatalf("head container predating its own launch was accepted: %v", err)
	}
}

func TestTensorFoldFabricRequiresGIDToEncodeEachNodeAddress(t *testing.T) {
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "planned")
	base := &fakeTensorFoldRunner{}
	runner := tensorFoldRunnerFunc(func(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
		// The worker's GID 3 carries a secondary address instead of its fabric IP.
		if name == "ssh" && strings.Contains(strings.Join(args, " "), "ports/1/gids/3") {
			return []byte("0000:0000:0000:0000:0000:ffff:c0a8:c909\n"), nil
		}
		return base.Run(ctx, dir, env, name, args...)
	})
	if err := newTensorFoldManager(t.TempDir(), base).verifyTensorFoldFabric(context.Background(), resource); err != nil {
		t.Fatalf("matching GIDs were rejected: %v", err)
	}
	if err := newTensorFoldManager(t.TempDir(), runner).verifyTensorFoldFabric(context.Background(), resource); err == nil || !strings.Contains(err.Error(), resource.WorkerAddress) {
		t.Fatalf("GID 3 for another address passed fabric preflight: %v", err)
	}
}

func TestTensorFoldRemoveDeletesSupervisorLogAndTransport(t *testing.T) {
	manager := newTensorFoldManager(t.TempDir(), noTensorFoldContainersRunner(t))
	manager.fenceProcesses = func(string) error { return nil }
	head := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "stopped")
	if err := manager.writeResource(head); err != nil {
		t.Fatal(err)
	}
	if err := manager.ensureTensorFoldSSHTransport(head); err != nil {
		t.Fatal(err)
	}
	logFile, err := manager.openLog(head.Name)
	if err != nil {
		t.Fatal(err)
	}
	_ = logFile.Close()
	if err := manager.remove(context.Background(), head.Name); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{manager.logPath(head.Name), manager.tensorFoldSSHTransportDir(head.LaunchID)} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s outlived its released reservation: %v", path, err)
		}
	}
}

func TestTensorFoldPreparationDeadlineStopsUnservedLaunch(t *testing.T) {
	manager := newTensorFoldManager(t.TempDir(), noTensorFoldContainersRunner(t))
	manager.fenceProcesses = func(string) error { return nil }
	head := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	head.LaunchStartedAt = time.Now().Add(-tensorFoldPreparationDeadline - time.Minute).UTC()
	stageTensorFoldExecutionFixture(t, manager, head)
	if err := manager.writeResource(head); err != nil {
		t.Fatal(err)
	}
	process := &fakeTensorFoldProcess{done: make(chan error, 1)}
	trackTestTensorFoldProcess(t, manager, head, process)
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if current, err := manager.inspect(head.Name); err != nil || current.Status != "stopped" {
		t.Fatalf("hung preparation kept the node past its deadline: resource=%#v err=%v", current, err)
	}

	if tensorFoldPreparationDeadline <= 14400*time.Second {
		t.Fatal("agent deadline must trail the coordinator's readiness timeout")
	}
}
