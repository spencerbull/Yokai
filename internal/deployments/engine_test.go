package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
)

type fakeOperations struct {
	events         []string
	fail           string
	failErr        error
	observed       map[string]ObservedContainer
	candidates     []Candidate
	testAPIKey     string
	expectedAPIKey string
	testModel      string
	testFailures   int
	testCalls      int
	statuses       map[string]string
	exitRole       string
	failOnce       map[string]int
	logTails       map[string]string
	logCaptureErr  map[string]error
	logRedactions  map[string]string
}

func (f *fakeOperations) record(event string) error {
	f.events = append(f.events, event)
	if f.failOnce[event] > 0 {
		f.failOnce[event]--
		return errors.New("injected once " + event)
	}
	if f.fail == event {
		if f.failErr != nil {
			return f.failErr
		}
		return errors.New("injected " + event)
	}
	return nil
}

func (f *fakeOperations) Preflight(_ context.Context, request PreflightRequest, _ []string, _ []string) error {
	return f.record("preflight:" + request.Binding.Role)
}
func (f *fakeOperations) Pull(_ context.Context, binding Binding, _ string) error {
	return f.record("pull:" + binding.Role)
}
func (f *fakeOperations) Inspect(_ context.Context, deviceID, containerID string) (ObservedContainer, error) {
	if err := f.record("inspect:" + deviceID + ":" + containerID); err != nil {
		return ObservedContainer{}, err
	}
	if observed, ok := f.observed[containerID]; ok {
		return observed, nil
	}
	for _, candidate := range f.candidates {
		if candidate.Name == containerID || "new-"+candidate.Role == containerID {
			status := "running"
			if f.statuses != nil && f.statuses["new-"+candidate.Role] != "" {
				status = f.statuses["new-"+candidate.Role]
			}
			if f.exitRole == candidate.Role {
				status = "stopped"
			}
			generation, _ := strconv.Atoi(candidate.Labels["io.yokai.deployment.generation"])
			return ObservedContainer{ID: "new-" + candidate.Role, Name: candidate.Name, Status: status, Ownership: OwnershipManaged, Managed: true, Generation: generation, DeploymentID: candidate.Labels["io.yokai.deployment.id"], Role: candidate.Role}, nil
		}
	}
	if strings.HasPrefix(containerID, "yokai-deployment-") {
		return ObservedContainer{}, ErrContainerNotFound
	}
	return ObservedContainer{ID: containerID, Status: "running", Ownership: OwnershipObserved}, nil
}
func (f *fakeOperations) Stop(_ context.Context, deviceID, containerID string) error {
	if err := f.record("stop:" + deviceID + ":" + containerID); err != nil {
		return err
	}
	if observed, ok := f.observed[containerID]; ok {
		observed.Status = "stopped"
		f.observed[containerID] = observed
	}
	if f.statuses == nil {
		f.statuses = make(map[string]string)
	}
	f.statuses[containerID] = "stopped"
	return nil
}
func (f *fakeOperations) StopManaged(ctx context.Context, _ Deployment, member Member) error {
	return f.Stop(ctx, member.DeviceID, memberLocator(member))
}
func (f *fakeOperations) Launch(_ context.Context, binding Binding, candidate Candidate) (ObservedContainer, error) {
	f.candidates = append(f.candidates, candidate)
	if err := f.record("launch:" + binding.Role); err != nil {
		return ObservedContainer{}, err
	}
	if f.statuses == nil {
		f.statuses = make(map[string]string)
	}
	f.statuses["new-"+binding.Role] = "running"
	generation, _ := strconv.Atoi(candidate.Labels["io.yokai.deployment.generation"])
	return ObservedContainer{ID: "new-" + binding.Role, Name: candidate.Name, Status: "created", Ownership: OwnershipManaged, Managed: true, Generation: generation, DeploymentID: candidate.Labels["io.yokai.deployment.id"], Role: binding.Role}, nil
}
func (f *fakeOperations) WaitRunning(_ context.Context, deviceID, containerID string) error {
	return f.record("wait:" + deviceID + ":" + containerID)
}
func (f *fakeOperations) Test(_ context.Context, deviceID, containerID, apiKey string) (TestResult, error) {
	f.testAPIKey = apiKey
	f.testCalls++
	if err := f.record("test:" + deviceID + ":" + containerID); err != nil {
		return TestResult{}, err
	}
	if f.expectedAPIKey != "" && apiKey != f.expectedAPIKey {
		return TestResult{}, errors.New("invalid original API key")
	}
	if f.testFailures > 0 {
		f.testFailures--
		return TestResult{}, errors.New("service still loading")
	}
	model := bkc.GLM53FlashNVFP4Model
	for _, candidate := range f.candidates {
		if candidate.Role == bkc.MultiDeviceRoleHead && candidate.Model != "" {
			model = candidate.Model
		}
	}
	if f.testModel != "" {
		model = f.testModel
	}
	return TestResult{OK: true, MetricsReady: true, Message: "model and metrics responded", Model: model, Response: " OK "}, nil
}

func (f *fakeOperations) CaptureManagedLogs(_ context.Context, _ Deployment, member Member, exactRedaction string) (LogTailCapture, error) {
	if f.logRedactions == nil {
		f.logRedactions = make(map[string]string)
	}
	f.logRedactions[member.Role] = exactRedaction
	if err := f.record("logs:" + member.DeviceID + ":" + memberLocator(member)); err != nil {
		return LogTailCapture{}, err
	}
	if err := f.logCaptureErr[member.Role]; err != nil {
		return LogTailCapture{}, err
	}
	return LogTailCapture{Tail: f.logTails[member.Role]}, nil
}

type noMetricsOperations struct{ *fakeOperations }

type cancelingLaunchOperations struct {
	*fakeOperations
	cancel context.CancelFunc
}

type cancelingSuccessfulLaunchOperations struct {
	*fakeOperations
	cancel context.CancelFunc
}

type barrierRemovalOperations struct {
	*fakeOperations
	started chan struct{}
	release chan struct{}
}

type crashAfterRemovalOperations struct {
	*fakeOperations
	candidateName string
	removed       bool
}

type cancelingStartOperations struct {
	*fakeOperations
	cancelOnTest bool
	cancel       context.CancelFunc
}

func (f *cancelingStartOperations) Test(ctx context.Context, deviceID, containerID, apiKey string) (TestResult, error) {
	if !f.cancelOnTest {
		return f.fakeOperations.Test(ctx, deviceID, containerID, apiKey)
	}
	f.testAPIKey = apiKey
	f.testCalls++
	if err := f.record("test:" + deviceID + ":" + containerID); err != nil {
		return TestResult{}, err
	}
	f.cancel()
	return TestResult{}, context.Canceled
}

func (f *cancelingStartOperations) Stop(ctx context.Context, deviceID, containerID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return f.fakeOperations.Stop(ctx, deviceID, containerID)
}

func (f *cancelingLaunchOperations) Launch(_ context.Context, binding Binding, candidate Candidate) (ObservedContainer, error) {
	f.candidates = append(f.candidates, candidate)
	f.cancel()
	_ = f.record("launch:" + binding.Role)
	return ObservedContainer{}, context.Canceled
}

func (f *cancelingLaunchOperations) Inspect(ctx context.Context, deviceID, containerID string) (ObservedContainer, error) {
	if err := ctx.Err(); err != nil {
		return ObservedContainer{}, err
	}
	return f.fakeOperations.Inspect(ctx, deviceID, containerID)
}

func (f *cancelingSuccessfulLaunchOperations) Launch(ctx context.Context, binding Binding, candidate Candidate) (ObservedContainer, error) {
	observed, err := f.fakeOperations.Launch(ctx, binding, candidate)
	f.cancel()
	return observed, err
}

func (f *barrierRemovalOperations) RemoveManaged(ctx context.Context, deployment Deployment, member Member) error {
	select {
	case <-f.started:
	default:
		close(f.started)
	}
	select {
	case <-f.release:
		return ErrContainerNotFound
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *crashAfterRemovalOperations) Inspect(ctx context.Context, deviceID, containerID string) (ObservedContainer, error) {
	if f.removed && containerID == f.candidateName {
		if err := f.record("inspect:" + deviceID + ":" + containerID); err != nil {
			return ObservedContainer{}, err
		}
		return ObservedContainer{}, ErrContainerNotFound
	}
	return f.fakeOperations.Inspect(ctx, deviceID, containerID)
}

func (f *crashAfterRemovalOperations) CaptureManagedLogs(ctx context.Context, deployment Deployment, member Member, exactRedaction string) (LogTailCapture, error) {
	if f.removed {
		if err := f.record("logs:" + member.DeviceID + ":" + memberLocator(member)); err != nil {
			return LogTailCapture{}, err
		}
		return LogTailCapture{}, errors.New("candidate already absent")
	}
	return f.fakeOperations.CaptureManagedLogs(ctx, deployment, member, exactRedaction)
}

func (f *crashAfterRemovalOperations) RemoveManaged(ctx context.Context, deployment Deployment, member Member) error {
	if err := f.fakeOperations.RemoveManaged(ctx, deployment, member); err != nil {
		return err
	}
	f.removed = true
	return nil
}

func (f *noMetricsOperations) Test(_ context.Context, deviceID, containerID, _ string) (TestResult, error) {
	if err := f.record("test:" + deviceID + ":" + containerID); err != nil {
		return TestResult{}, err
	}
	return TestResult{OK: true, Message: "model responded but metrics did not", Model: bkc.GLM53FlashNVFP4Model, Response: "ok"}, nil
}
func (f *fakeOperations) Remove(_ context.Context, deviceID, containerID string) error {
	return f.record("remove:" + deviceID + ":" + containerID)
}
func (f *fakeOperations) RemoveManaged(ctx context.Context, _ Deployment, member Member) error {
	return f.Remove(ctx, member.DeviceID, memberLocator(member))
}
func (f *fakeOperations) Restart(_ context.Context, deviceID, containerID string) error {
	if err := f.record("restart:" + deviceID + ":" + containerID); err != nil {
		return err
	}
	if observed, ok := f.observed[containerID]; ok {
		observed.Status = "running"
		f.observed[containerID] = observed
	}
	if f.statuses == nil {
		f.statuses = make(map[string]string)
	}
	f.statuses[containerID] = "running"
	return nil
}
func (f *fakeOperations) RestartManaged(ctx context.Context, _ Deployment, member Member) error {
	return f.Restart(ctx, member.DeviceID, memberLocator(member))
}

func newTestEngine(t *testing.T, ops Operations) (*Engine, *Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), StoreFile)
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	engine := NewEngine(store, ops)
	engine.NewID = func() string { return "dep-test" }
	engine.Now = func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }
	engine.ReadinessTimeout = 30 * time.Millisecond
	engine.ReadinessInterval = time.Millisecond
	return engine, store, path
}

func validRequest() CreateRequest {
	return CreateRequest{
		BKCID: bkc.GLM53FlashNVFP4DualGB10ID, IdempotencyKey: "create-1", APIKey: "test-api-key",
		Bindings: []Binding{
			{Role: bkc.MultiDeviceRoleWorker, DeviceID: "spark-b", FabricAddress: "192.168.201.2"},
			{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", FabricAddress: "192.168.201.1", ServiceAddress: "100.96.0.20", ServicePort: 8000},
		},
	}
}

func TestCreateRejectsDuplicateDeviceBeforeMutation(t *testing.T) {
	ops := &fakeOperations{}
	engine, store, _ := newTestEngine(t, ops)
	request := validRequest()
	request.Bindings[0].DeviceID = request.Bindings[1].DeviceID
	if _, err := engine.Create(context.Background(), request); err == nil || !strings.Contains(err.Error(), "distinct devices") {
		t.Fatalf("expected duplicate-device rejection, got %v", err)
	}
	if len(ops.events) != 0 || len(store.List()) != 0 {
		t.Fatalf("validation mutated state: events=%v store=%v", ops.events, store.List())
	}
}

func TestCreateFailsOldAgentCapabilityBeforeMutation(t *testing.T) {
	ops := &fakeOperations{fail: "preflight:head", failErr: errors.New("agent lacks required capability deployments.v1")}
	engine, store, _ := newTestEngine(t, ops)
	if _, err := engine.Create(context.Background(), validRequest()); err == nil || !strings.Contains(err.Error(), "lacks required capability deployments.v1") {
		t.Fatalf("expected old-agent capability failure, got %v", err)
	}
	if !reflect.DeepEqual(ops.events, []string{"preflight:head"}) || len(store.List()) != 0 {
		t.Fatalf("preflight failure crossed mutation boundary: events=%v store=%v", ops.events, store.List())
	}
}

func TestCreateRequiresExplicitDistinctHeadServiceEndpointBeforeMutation(t *testing.T) {
	for name, mutate := range map[string]func(*CreateRequest){
		"missing address": func(request *CreateRequest) { request.Bindings[1].ServiceAddress = "" },
		"broad address":   func(request *CreateRequest) { request.Bindings[1].ServiceAddress = "0.0.0.0" },
		"fabric address":  func(request *CreateRequest) { request.Bindings[1].ServiceAddress = request.Bindings[1].FabricAddress },
		"wrong port":      func(request *CreateRequest) { request.Bindings[1].ServicePort = 30000 },
	} {
		t.Run(name, func(t *testing.T) {
			ops := &fakeOperations{}
			engine, store, _ := newTestEngine(t, ops)
			request := validRequest()
			mutate(&request)
			if _, err := engine.Create(context.Background(), request); err == nil {
				t.Fatal("expected head service endpoint rejection")
			}
			if len(ops.events) != 0 || len(store.List()) != 0 {
				t.Fatalf("invalid endpoint mutated state: events=%v store=%v", ops.events, store.List())
			}
		})
	}
}

func TestCreateTransactionOrderAndIdempotency(t *testing.T) {
	ops := &fakeOperations{}
	engine, store, _ := newTestEngine(t, ops)
	first, err := engine.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	wantEvents := []string{
		"preflight:head", "preflight:worker", "pull:head", "pull:worker",
		"launch:head", "launch:worker", "inspect:spark-a:new-head", "inspect:spark-b:new-worker", "test:spark-a:new-head",
	}
	if !reflect.DeepEqual(ops.events, wantEvents) {
		t.Fatalf("transaction order mismatch\n got: %v\nwant: %v", ops.events, wantEvents)
	}
	if first.State != StateRunning || len(first.Members) != 2 || first.Members[0].Role != bkc.MultiDeviceRoleHead {
		t.Fatalf("unexpected promoted deployment: %#v", first)
	}
	eventCount := len(ops.events)
	second, err := engine.Create(context.Background(), validRequest())
	if err != nil || second.ID != first.ID || len(ops.events) != eventCount {
		t.Fatalf("idempotent replay reran transaction: deployment=%#v err=%v events=%v", second, err, ops.events)
	}
	conflict := validRequest()
	conflict.Bindings[0].FabricAddress = "192.168.201.3"
	if _, err := engine.Create(context.Background(), conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	if len(store.List()) != 1 {
		t.Fatalf("expected one durable deployment, got %d", len(store.List()))
	}
}

func TestTerminalIdempotentReplaysReturnWithoutWork(t *testing.T) {
	ops := &fakeOperations{}
	engine, _, _ := newTestEngine(t, ops)
	request := validRequest()
	created, err := engine.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Stop(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	eventCount := len(ops.events)
	stopped, err := engine.Create(context.Background(), request)
	if err != nil || stopped.State != StateStopped || len(ops.events) != eventCount {
		t.Fatalf("stopped replay performed work: deployment=%#v err=%v events=%v", stopped, err, ops.events[eventCount:])
	}
	rolledBack, err := engine.Rollback(context.Background(), created.ID)
	if err != nil || rolledBack.State != StateRolledBack {
		t.Fatalf("rollback: deployment=%#v err=%v", rolledBack, err)
	}
	eventCount = len(ops.events)
	replayed, err := engine.Create(context.Background(), request)
	if err != nil || replayed.State != StateRolledBack || len(ops.events) != eventCount {
		t.Fatalf("rolled-back replay performed work: deployment=%#v err=%v events=%v", replayed, err, ops.events[eventCount:])
	}
}

func TestCreateRollbackRemovesCandidatesReverseAndRestartsOldIDs(t *testing.T) {
	ops := &fakeOperations{
		fail: "launch:worker",
		observed: map[string]ObservedContainer{
			"old-head":   {ID: "old-head", Status: "running", Ownership: OwnershipObserved, Generation: 7},
			"old-worker": {ID: "old-worker", Status: "running", Ownership: OwnershipObserved, Generation: 7},
		},
	}
	engine, store, _ := newTestEngine(t, ops)
	request := validRequest()
	request.Bindings[0].ObservedContainerID = "old-worker"
	request.Bindings[1].ObservedContainerID = "old-head"
	deployment, err := engine.Create(context.Background(), request)
	if err == nil {
		t.Fatal("expected injected launch failure")
	}
	wantSuffix := []string{
		"launch:head", "launch:worker", "inspect:spark-b:yokai-deployment-dep-test-g8-worker", "logs:spark-b:new-worker", "remove:spark-b:new-worker",
		"inspect:spark-a:yokai-deployment-dep-test-g8-head", "logs:spark-a:new-head", "remove:spark-a:new-head",
		"inspect:spark-a:old-head", "restart:spark-a:old-head", "wait:spark-a:old-head",
		"inspect:spark-b:old-worker", "restart:spark-b:old-worker", "wait:spark-b:old-worker",
	}
	if len(ops.events) < len(wantSuffix) || !reflect.DeepEqual(ops.events[len(ops.events)-len(wantSuffix):], wantSuffix) {
		t.Fatalf("rollback order mismatch: %v", ops.events)
	}
	if deployment.State != StateRolledBack || deployment.PreviousGeneration != 7 || deployment.Rollback == nil {
		t.Fatalf("rollback was not persisted: %#v", deployment)
	}
	stored, getErr := store.Get(deployment.ID)
	if getErr != nil || stored.State != StateRolledBack {
		t.Fatalf("stored rollback mismatch: %#v err=%v", stored, getErr)
	}
}

func TestCreateRequiresMetricsBeforePromotion(t *testing.T) {
	ops := &noMetricsOperations{fakeOperations: &fakeOperations{}}
	engine, store, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err == nil || !strings.Contains(err.Error(), "metrics check did not report ready") {
		t.Fatalf("expected metrics promotion failure, got deployment=%#v err=%v", deployment, err)
	}
	if deployment.State != StateRolledBack || deployment.Rollback == nil {
		t.Fatalf("metrics failure did not roll back candidates: %#v", deployment)
	}
	stored, getErr := store.Get(deployment.ID)
	if getErr != nil || stored.State != StateRolledBack {
		t.Fatalf("metrics rollback was not persisted: %#v err=%v", stored, getErr)
	}
}

func TestFailedCreatePersistsRedactedRankOneLogTail(t *testing.T) {
	const sentinel = "exact-sentinel-api-key"
	ops := &fakeOperations{
		testModel: "unexpected/model",
		logTails: map[string]string{
			bkc.MultiDeviceRoleWorker: "rank 1 scheduler exception " + sentinel + " --api-key=generic-equals --api-key generic-space\x00",
			bkc.MultiDeviceRoleHead:   "rank 0 waiting",
		},
	}
	engine, store, path := newTestEngine(t, ops)
	request := validRequest()
	request.APIKey = sentinel
	deployment, err := engine.Create(context.Background(), request)
	if err == nil || deployment.State != StateRolledBack || deployment.Rollback == nil {
		t.Fatalf("expected failed deployment rollback, got deployment=%#v err=%v", deployment, err)
	}
	var rankOne RankLogTail
	for _, capture := range deployment.Rollback.LogTails {
		if capture.Rank == 1 {
			rankOne = capture
		}
	}
	if rankOne.Status != "captured" || !strings.Contains(rankOne.Tail, "rank 1 scheduler exception") {
		t.Fatalf("rank-1 scheduler log was not persisted: %#v", rankOne)
	}
	serialized, marshalErr := json.Marshal(deployment)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	storedBytes, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, unsafe := range []string{sentinel, "generic-equals", "generic-space", "\x00"} {
		if strings.Contains(string(serialized), unsafe) || strings.Contains(string(storedBytes), unsafe) {
			t.Fatalf("unsafe value %q survived deployment persistence", unsafe)
		}
	}
	stored, getErr := store.Get(deployment.ID)
	if getErr != nil || stored.Rollback == nil || len(stored.Rollback.LogTails) != 2 {
		t.Fatalf("stored log tails missing: deployment=%#v err=%v", stored, getErr)
	}
	if ops.logRedactions[bkc.MultiDeviceRoleHead] != sentinel || ops.logRedactions[bkc.MultiDeviceRoleWorker] != "" {
		t.Fatalf("transient create API key crossed the rank boundary: %#v", ops.logRedactions)
	}
}

func TestLogCaptureFailureStillRemovesCandidatesAndRestoresPrevious(t *testing.T) {
	const sentinel = "capture-sentinel-key"
	ops := &fakeOperations{
		testModel: "unexpected/model",
		logCaptureErr: map[string]error{
			bkc.MultiDeviceRoleHead:   errors.New("head capture failed " + sentinel),
			bkc.MultiDeviceRoleWorker: errors.New("worker capture failed " + sentinel),
		},
		observed: map[string]ObservedContainer{
			"old-head":   {ID: "old-head", Status: "running", Ownership: OwnershipObserved, Generation: 4},
			"old-worker": {ID: "old-worker", Status: "running", Ownership: OwnershipObserved, Generation: 4},
		},
	}
	engine, _, _ := newTestEngine(t, ops)
	request := validRequest()
	request.APIKey = sentinel
	request.Bindings[0].ObservedContainerID = "old-worker"
	request.Bindings[1].ObservedContainerID = "old-head"
	deployment, err := engine.Create(context.Background(), request)
	if err == nil || deployment.State != StateRolledBack || deployment.Rollback == nil {
		t.Fatalf("capture failure blocked rollback: deployment=%#v err=%v", deployment, err)
	}
	if len(deployment.Rollback.LogTails) != 2 {
		t.Fatalf("capture failures were not represented honestly: %#v", deployment.Rollback)
	}
	for _, capture := range deployment.Rollback.LogTails {
		if capture.Status != "failed" || capture.Error == "" || strings.Contains(capture.Error, sentinel) {
			t.Fatalf("unsafe or dishonest capture failure: %#v", capture)
		}
	}
	for _, event := range []string{
		"remove:spark-b:new-worker", "remove:spark-a:new-head",
		"restart:spark-a:old-head", "restart:spark-b:old-worker",
	} {
		if !containsString(ops.events, event) {
			t.Fatalf("capture failure blocked rollback event %q: %v", event, ops.events)
		}
	}
}

func TestManualRollbackRedactsGenericAPIKeyFormsWithoutOriginalKey(t *testing.T) {
	ops := &fakeOperations{logTails: map[string]string{
		bkc.MultiDeviceRoleHead:   "argv --api-key=manual-equals api_key='manual-sglang'",
		bkc.MultiDeviceRoleWorker: `argv --api-key manual-space {"api_key":"manual-json"}`,
	}}
	engine, _, _ := newTestEngine(t, ops)
	created, err := engine.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	rolledBack, err := engine.Rollback(context.Background(), created.ID)
	if err != nil || rolledBack.State != StateRolledBack || rolledBack.Rollback == nil {
		t.Fatalf("manual rollback failed: deployment=%#v err=%v", rolledBack, err)
	}
	serialized, _ := json.Marshal(rolledBack.Rollback.LogTails)
	for _, secret := range []string{"manual-equals", "manual-space", "manual-sglang", "manual-json"} {
		if strings.Contains(string(serialized), secret) {
			t.Fatalf("manual rollback persisted generic API key %q: %s", secret, serialized)
		}
	}
	if ops.logRedactions[bkc.MultiDeviceRoleHead] != "" || ops.logRedactions[bkc.MultiDeviceRoleWorker] != "" {
		t.Fatal("manual rollback unexpectedly recovered a request-time API key")
	}
}

func TestCreateSecretsAndLocalPathNeverPersistOrSerialize(t *testing.T) {
	ops := &fakeOperations{}
	engine, _, path := newTestEngine(t, ops)
	request := validRequest()
	request.APIKey = "-sentinel-api-secret"
	request.LocalModelPath = "/srv/models/glm53-snapshot"
	deployment, err := engine.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(ops.candidates) != 2 || ops.candidates[0].Volumes[request.LocalModelPath] != FixedLocalModelPath+":ro" || ops.candidates[0].Model != FixedLocalModelPath {
		t.Fatalf("local snapshot was not mounted read-only at fixed path: %#v", ops.candidates)
	}
	if ops.testAPIKey != request.APIKey {
		t.Fatalf("promotion probe did not receive the request-time API key")
	}
	head, worker := ops.candidates[0], ops.candidates[1]
	cfg, _ := bkc.LookupID(bkc.GLM53FlashNVFP4DualGB10ID)
	wantPatches := runtimePatchProvenance(cfg)
	if !reflect.DeepEqual(deployment.RuntimePatches, wantPatches) || head.Labels["io.yokai.runtime.patch"] != bkc.GLM53FlashRuntimePatchSetLabel() || worker.Labels["io.yokai.runtime.patch"] != bkc.GLM53FlashRuntimePatchSetLabel() {
		t.Fatalf("runtime patch provenance was not persisted and labeled exactly: deployment=%#v head=%#v worker=%#v", deployment.RuntimePatches, head.Labels, worker.Labels)
	}
	if !reflect.DeepEqual(head.Env, cfg.Env) || !reflect.DeepEqual(worker.Env, cfg.Env) {
		t.Fatalf("candidate environment did not come exactly from the pinned recipe: head=%#v worker=%#v", head.Env, worker.Env)
	}
	if !reflect.DeepEqual(head.Args, []string{"--api-key=-sentinel-api-secret"}) || len(worker.Args) != 0 {
		t.Fatalf("API key was not a single rank-0-only argv token: head=%#v worker=%#v", head.Args, worker.Args)
	}
	if head.Runtime.RestartPolicy != "no" || worker.Runtime.RestartPolicy != "no" {
		t.Fatalf("coordinated ranks must not auto-restart: head=%q worker=%q", head.Runtime.RestartPolicy, worker.Runtime.RestartPolicy)
	}
	if !strings.Contains(head.ExtraArgs, "--host 100.96.0.20 --port 8000") || strings.Contains(head.ExtraArgs, "0.0.0.0") || head.Labels["io.yokai.service.address"] != "100.96.0.20" || head.Labels["io.yokai.service.port"] != "8000" {
		t.Fatalf("head endpoint was not rendered and labeled exactly: %#v", head)
	}
	if !strings.Contains(worker.ExtraArgs, "--host 192.168.201.2 --port 8000") || strings.Contains(worker.ExtraArgs, "0.0.0.0") {
		t.Fatalf("worker bind was not constrained to its private fabric address: %#v", worker)
	}
	storeData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	responseData, err := json.Marshal(deployment)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	for surface, data := range map[string][]byte{"store": storeData, "response": responseData} {
		lower := strings.ToLower(string(data))
		for _, forbidden := range []string{"api_key", "sentinel-api-secret", request.LocalModelPath, "\"env\"", "--api-key"} {
			if strings.Contains(lower, strings.ToLower(forbidden)) {
				t.Fatalf("%s exposed %q: %s", surface, forbidden, data)
			}
		}
		if !strings.Contains(string(data), `"runtime_patches"`) || strings.Contains(string(data), `"runtime_patch":`) {
			t.Fatalf("%s did not expose ordered runtime patch provenance: %s", surface, data)
		}
		for _, patch := range wantPatches {
			for _, exact := range []string{patch.Label, patch.SourcePath, patch.OriginalSHA256, patch.PatchedSHA256} {
				if !strings.Contains(string(data), exact) {
					t.Fatalf("%s omitted runtime patch provenance %q: %s", surface, exact, data)
				}
			}
		}
	}
}

func TestCreateRetriesSemanticReadinessUntilSuccess(t *testing.T) {
	ops := &fakeOperations{testFailures: 2}
	engine, _, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err != nil || deployment.State != StateRunning {
		t.Fatalf("patient readiness did not promote: deployment=%#v err=%v", deployment, err)
	}
	if ops.testCalls != 3 {
		t.Fatalf("expected three readiness attempts, got %d", ops.testCalls)
	}
}

func TestCreateFailsFastWhenWorkerExitsDuringReadiness(t *testing.T) {
	ops := &fakeOperations{exitRole: bkc.MultiDeviceRoleWorker}
	engine, _, _ := newTestEngine(t, ops)
	// Create includes durable rollback after readiness detects the exited rank.
	// Give that cleanup enough headroom under the race detector; the exact
	// inspection counts below prove readiness stopped after its first pass.
	engine.ReadinessTimeout = 2 * time.Second
	deployment, err := engine.Create(context.Background(), validRequest())
	if err == nil || !strings.Contains(err.Error(), "worker rank exited") {
		t.Fatalf("expected worker-exit failure, got deployment=%#v err=%v", deployment, err)
	}
	for _, event := range []string{"inspect:spark-a:new-head", "inspect:spark-b:new-worker"} {
		if count := countString(ops.events, event); count != 1 {
			t.Fatalf("readiness did not fail after one inspection pass: event=%q count=%d events=%v", event, count, ops.events)
		}
	}
}

func TestCreateRejectsUnexpectedServedModel(t *testing.T) {
	ops := &fakeOperations{testModel: "unexpected/model"}
	engine, _, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err == nil || deployment.State != StateRolledBack || !strings.Contains(err.Error(), "unexpected model") {
		t.Fatalf("unexpected served model was promoted: deployment=%#v err=%v", deployment, err)
	}
}

func TestCreateRollbackContinuesAfterRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ops := &cancelingLaunchOperations{fakeOperations: &fakeOperations{}, cancel: cancel}
	engine, _, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(ctx, validRequest())
	if err == nil || deployment.State != StateRolledBack {
		t.Fatalf("canceled launch did not get bounded rollback: deployment=%#v err=%v", deployment, err)
	}
	wantRemove := "remove:spark-a:new-head"
	if !containsString(ops.events, wantRemove) {
		t.Fatalf("rollback did not try deterministic candidate after cancellation: events=%v", ops.events)
	}
}

func TestCreateCancellationAfterSuccessfulLaunchDoesNotStartNextRank(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ops := &cancelingSuccessfulLaunchOperations{fakeOperations: &fakeOperations{}, cancel: cancel}
	engine, _, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(ctx, validRequest())
	if err == nil || deployment.State != StateRolledBack {
		t.Fatalf("post-launch cancellation did not get bounded rollback: deployment=%#v err=%v", deployment, err)
	}
	if containsString(ops.events, "launch:worker") {
		t.Fatalf("worker launched after the request ended: %v", ops.events)
	}
	if !containsString(ops.events, "remove:spark-a:new-head") {
		t.Fatalf("completed head launch was not rolled back: %v", ops.events)
	}
}

func TestRollbackWaitsForAbsentCandidateRemovalBarrier(t *testing.T) {
	ops := &barrierRemovalOperations{fakeOperations: &fakeOperations{}, started: make(chan struct{}), release: make(chan struct{})}
	engine, store, _ := newTestEngine(t, ops)
	deployment := Deployment{ID: "dep-test", State: StatePending, Phase: PhaseLaunching, Generation: 1,
		Members: []Member{{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: "yokai-deployment-dep-test-g1-head", Name: "yokai-deployment-dep-test-g1-head", Status: "launching", Ownership: OwnershipManaged, Generation: 1}}}
	if err := store.Put(deployment); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := engine.Rollback(context.Background(), deployment.ID)
		done <- err
	}()
	<-ops.started
	select {
	case err := <-done:
		t.Fatalf("rollback certified absence before the agent removal barrier completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(ops.release)
	if err := <-done; err != nil {
		t.Fatalf("rollback failed after the absence barrier completed: %v", err)
	}
}

func TestRollbackDoesNotRestorePreviousBeforeLaunchBarrier(t *testing.T) {
	fake := &fakeOperations{observed: map[string]ObservedContainer{
		"old-head": {ID: "old-head", Name: "old-head", Status: "stopped", Ownership: OwnershipObserved},
	}}
	ops := &barrierRemovalOperations{fakeOperations: fake, started: make(chan struct{}), release: make(chan struct{})}
	engine, store, _ := newTestEngine(t, ops)
	deployment := Deployment{ID: "dep-test", State: StatePending, Phase: PhaseLaunching, Generation: 1,
		Members:         []Member{{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: "yokai-deployment-dep-test-g1-head", Name: "yokai-deployment-dep-test-g1-head", Status: "launching", Ownership: OwnershipManaged, Generation: 1}},
		PreviousMembers: []Member{{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: "old-head", Name: "old-head", Status: "stopped", Ownership: OwnershipObserved}}}
	if err := store.Put(deployment); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result, err := engine.Rollback(ctx, deployment.ID)
	if err == nil || result.State != StateRollbackFailed {
		t.Fatalf("timed-out launch barrier was certified safe: deployment=%#v err=%v", result, err)
	}
	for _, event := range ops.events {
		if strings.HasPrefix(event, "restart:") {
			t.Fatalf("previous member restarted before candidate cleanup completed: %v", ops.events)
		}
	}
}

func TestPendingIdempotentReplayReconcilesAndRequiresNewKey(t *testing.T) {
	ops := &fakeOperations{observed: map[string]ObservedContainer{
		"old-head-1234567890": {ID: "old-head-1234567890", Status: "stopped", Ownership: OwnershipObserved},
	}}
	engine, store, _ := newTestEngine(t, ops)
	request := validRequest()
	hash, err := requestHash(normalizeRequest(request))
	if err != nil {
		t.Fatal(err)
	}
	pending := Deployment{
		ID: "dep-test", BKCID: request.BKCID, IdempotencyKey: request.IdempotencyKey, RequestHash: hash,
		State: StatePending, Phase: PhaseLaunching, Generation: 1,
		Bindings:        request.Bindings,
		Members:         []Member{{Role: "head", DeviceID: "spark-a", Name: "yokai-deployment-dep-test-g1-head", ContainerID: "yokai-deployment-dep-test-g1-head", Status: "launching", Ownership: OwnershipManaged}},
		PreviousMembers: []Member{{Role: "head", DeviceID: "spark-a", ContainerID: "old-head-1234567890", Status: "stopped", Ownership: OwnershipObserved}},
		CreatedAt:       time.Now(), UpdatedAt: time.Now(),
	}
	if err := store.Put(pending); err != nil {
		t.Fatal(err)
	}
	deployment, err := engine.Create(context.Background(), request)
	if !errors.Is(err, ErrRecoveryPerformed) || deployment.State != StateRolledBack {
		t.Fatalf("pending replay was not explicitly reconciled: deployment=%#v err=%v", deployment, err)
	}
	for _, event := range ops.events {
		if strings.HasPrefix(event, "pull:") || strings.HasPrefix(event, "launch:") {
			t.Fatalf("recovery started new work: %v", ops.events)
		}
	}
}

func TestFailedIdempotentReplayReconcilesAndNeverReturnsSuccess(t *testing.T) {
	ops := &fakeOperations{}
	engine, store, _ := newTestEngine(t, ops)
	request := validRequest()
	hash, err := requestHash(normalizeRequest(request))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(Deployment{
		ID: "dep-test", BKCID: request.BKCID, IdempotencyKey: request.IdempotencyKey, RequestHash: hash,
		State: StateFailed, Phase: PhaseComplete, Generation: 1, Bindings: request.Bindings,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	outcome, err := engine.CreateWithOutcome(context.Background(), request)
	if !errors.Is(err, ErrRecoveryPerformed) || outcome.Created || outcome.Deployment.State != StateRolledBack {
		t.Fatalf("failed replay returned success: outcome=%#v err=%v", outcome, err)
	}
}

func TestLifecycleIdempotentReplayNeverRollsBackPromotedGroup(t *testing.T) {
	tests := []struct {
		name   string
		state  State
		phase  Phase
		action string
		status string
	}{
		{name: "starting", state: StateStarting, phase: PhaseStarting, action: "start_group", status: "started"},
		{name: "starting readiness snapshot", state: StateStarting, phase: PhaseReadiness, action: "readiness", status: "started"},
		{name: "stopping", state: StateStopping, phase: PhaseStopping, action: "stop_group", status: "started"},
		{name: "failed start", state: StateFailed, phase: PhaseComplete, action: "start_group", status: "failed"},
		{name: "failed stop", state: StateFailed, phase: PhaseComplete, action: "stop_group", status: "failed"},
		{name: "failed running verification", state: StateFailed, phase: PhaseComplete, action: "verify_running", status: "failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ops := &fakeOperations{observed: map[string]ObservedContainer{
				"old-head":   {ID: "old-head", Name: "old-head", Status: "stopped", Ownership: OwnershipObserved},
				"old-worker": {ID: "old-worker", Name: "old-worker", Status: "stopped", Ownership: OwnershipObserved},
			}}
			engine, store, _ := newTestEngine(t, ops)
			request := validRequest()
			deployment, err := engine.Create(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			deployment.PreviousMembers = []Member{
				{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: "old-head", Status: "stopped", Ownership: OwnershipObserved},
				{Role: bkc.MultiDeviceRoleWorker, DeviceID: "spark-b", ContainerID: "old-worker", Status: "stopped", Ownership: OwnershipObserved},
			}
			deployment.State = test.state
			deployment.Phase = test.phase
			deployment.Progress = append(deployment.Progress, Progress{Phase: test.phase, Action: test.action, Status: test.status})
			if err := store.Put(deployment); err != nil {
				t.Fatal(err)
			}
			ops.events = nil

			outcome, err := engine.CreateWithOutcome(context.Background(), request)
			if err == nil || !errors.Is(err, ErrRecoveryPerformed) || outcome.Created || outcome.Deployment.State != StateStopped {
				t.Fatalf("lifecycle replay did not reconcile to an explicit stopped conflict: outcome=%#v err=%v", outcome, err)
			}
			for _, event := range ops.events {
				if strings.HasPrefix(event, "remove:") || strings.HasPrefix(event, "restart:") {
					t.Fatalf("lifecycle replay performed create rollback effect %q: %v", event, ops.events)
				}
			}
		})
	}
}

func TestRollbackFailedRetryCompletesPartialCleanup(t *testing.T) {
	ops := &fakeOperations{fail: "test:spark-a:new-head", failOnce: map[string]int{"remove:spark-b:new-worker": 1}}
	engine, _, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err == nil || deployment.State != StateRollbackFailed {
		t.Fatalf("expected partial rollback failure, got deployment=%#v err=%v", deployment, err)
	}
	ops.fail = ""
	retried, err := engine.Rollback(context.Background(), deployment.ID)
	if err != nil || retried.State != StateRolledBack {
		t.Fatalf("partial rollback was not retryable: deployment=%#v err=%v", retried, err)
	}
	for _, member := range retried.Members {
		if member.Status != "removed" && member.Status != "stopped" {
			t.Fatalf("rolled-back candidate retained active status: %#v", member)
		}
	}
	for _, name := range []string{"yokai-deployment-dep-test-g1-head", "yokai-deployment-dep-test-g1-worker"} {
		if countString(retried.Rollback.RemovedCandidates, name) != 1 {
			t.Fatalf("retry did not preserve/dedupe removed-candidate history for %s: %#v", name, retried.Rollback)
		}
	}
}

func TestRollbackRetryPreservesDurableCapturedTailAfterRemovalWriteFailure(t *testing.T) {
	const candidateName = "yokai-deployment-dep-test-g1-head"
	const schedulerFailure = "rank 0 scheduler exception survived"
	ops := &crashAfterRemovalOperations{
		fakeOperations: &fakeOperations{
			observed: map[string]ObservedContainer{candidateName: {
				ID: "candidate-head", Name: candidateName, Status: "running",
				Ownership: OwnershipManaged, Managed: true, Generation: 1,
				DeploymentID: "dep-test", Role: bkc.MultiDeviceRoleHead,
			}},
			logTails: map[string]string{bkc.MultiDeviceRoleHead: schedulerFailure},
		},
		candidateName: candidateName,
	}
	engine, store, path := newTestEngine(t, ops)
	deployment := Deployment{
		ID: "dep-test", State: StateRollbackFailed, Phase: PhaseRollback, Generation: 1,
		Members: []Member{{
			Role: bkc.MultiDeviceRoleHead, Rank: 0, DeviceID: "spark-a",
			ContainerID: candidateName, Name: candidateName, Status: "running",
			Ownership: OwnershipManaged, Generation: 1,
		}},
	}
	if err := store.Put(deployment); err != nil {
		t.Fatal(err)
	}
	originalWrite := store.write
	store.write = func(path string, document storeDocument) (bool, error) {
		if ops.removed {
			return false, errors.New("injected post-removal store failure")
		}
		return originalWrite(path, document)
	}

	first, err := engine.Rollback(context.Background(), deployment.ID)
	if err == nil || !ops.removed || first.Rollback == nil {
		t.Fatalf("first rollback did not reach the crash window: deployment=%#v removed=%v err=%v", first, ops.removed, err)
	}
	durable, err := store.Get(deployment.ID)
	if err != nil || durable.Rollback == nil || len(durable.Rollback.LogTails) != 1 {
		t.Fatalf("capture was not durable before removal: deployment=%#v err=%v", durable, err)
	}
	if tail := durable.Rollback.LogTails[0]; tail.Status != "captured" || tail.Tail != schedulerFailure || durable.Members[0].Status != "removing" {
		t.Fatalf("unexpected durable crash state: deployment=%#v", durable)
	}

	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	retryEngine := NewEngine(reopened, ops)
	retryEngine.Now = engine.Now
	retried, err := retryEngine.Rollback(context.Background(), deployment.ID)
	if err != nil || retried.State != StateRolledBack || retried.Rollback == nil || len(retried.Rollback.LogTails) != 1 {
		t.Fatalf("retry did not complete: deployment=%#v err=%v", retried, err)
	}
	if tail := retried.Rollback.LogTails[0]; tail.Status != "captured" || tail.Tail != schedulerFailure || tail.Error != "" {
		t.Fatalf("retry capture failure overwrote durable evidence: %#v", tail)
	}
	stored, err := reopened.Get(deployment.ID)
	if err != nil || stored.Rollback == nil || stored.Rollback.LogTails[0].Tail != schedulerFailure {
		t.Fatalf("preserved evidence was not durable after retry: deployment=%#v err=%v", stored, err)
	}
}

func TestUpsertRankLogTailAllowsLaterSuccessButNotLaterFailureToReplaceCapture(t *testing.T) {
	oldCapture := RankLogTail{Role: bkc.MultiDeviceRoleHead, Rank: 0, Status: "captured", Tail: "old evidence"}
	newCapture := RankLogTail{Role: bkc.MultiDeviceRoleHead, Rank: 0, Status: "captured", Tail: "new evidence"}
	newFailure := RankLogTail{Role: bkc.MultiDeviceRoleHead, Rank: 0, Status: "failed", Error: "container absent"}

	if got := upsertRankLogTail([]RankLogTail{oldCapture}, newFailure); !reflect.DeepEqual(got, []RankLogTail{oldCapture}) {
		t.Fatalf("later failure replaced successful capture: %#v", got)
	}
	if got := upsertRankLogTail([]RankLogTail{oldCapture}, newCapture); !reflect.DeepEqual(got, []RankLogTail{newCapture}) {
		t.Fatalf("later successful capture did not replace earlier capture: %#v", got)
	}
	if got := upsertRankLogTail([]RankLogTail{newFailure}, newCapture); !reflect.DeepEqual(got, []RankLogTail{newCapture}) {
		t.Fatalf("later successful capture did not replace earlier failure: %#v", got)
	}
}

func TestRollbackRefusesDeterministicNameOccupiedByUnownedContainer(t *testing.T) {
	deterministicName := "yokai-deployment-dep-test-g1-head"
	ops := &fakeOperations{observed: map[string]ObservedContainer{
		deterministicName: {ID: "external-container", Name: deterministicName, Status: "running", Ownership: OwnershipObserved},
	}}
	engine, store, _ := newTestEngine(t, ops)
	when := time.Now().UTC()
	deployment := Deployment{
		ID: "dep-test", State: StatePending, Phase: PhaseLaunching, Generation: 1,
		Members:   []Member{{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: deterministicName, Name: deterministicName, Status: "launching", Ownership: OwnershipManaged}},
		CreatedAt: when, UpdatedAt: when,
	}
	if err := store.Put(deployment); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Rollback(context.Background(), deployment.ID)
	if err == nil || result.State != StateRollbackFailed {
		t.Fatalf("unowned collision was not preserved: deployment=%#v err=%v", result, err)
	}
	for _, event := range ops.events {
		if strings.HasPrefix(event, "remove:") {
			t.Fatalf("rollback removed an unowned collision: %v", ops.events)
		}
	}
}

func TestRollbackDoesNotClaimSuccessUntilPreviousMembersAreRunning(t *testing.T) {
	ops := &fakeOperations{observed: map[string]ObservedContainer{
		"old-head":   {ID: "old-head", Status: "running", Ownership: OwnershipObserved},
		"old-worker": {ID: "old-worker", Status: "running", Ownership: OwnershipObserved},
	}}
	engine, _, _ := newTestEngine(t, ops)
	request := validRequest()
	request.Bindings[0].ObservedContainerID = "old-worker"
	request.Bindings[1].ObservedContainerID = "old-head"
	deployment, err := engine.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	ops.fail = "wait:spark-a:old-head"
	partial, err := engine.Rollback(context.Background(), deployment.ID)
	if err == nil || partial.State != StateRollbackFailed {
		t.Fatalf("rollback claimed success without restored head: deployment=%#v err=%v", partial, err)
	}
	ops.fail = ""
	retried, err := engine.Rollback(context.Background(), deployment.ID)
	if err != nil || retried.State != StateRolledBack {
		t.Fatalf("partial restoration was not retryable: deployment=%#v err=%v", retried, err)
	}
	for _, id := range []string{"old-head", "old-worker"} {
		if countString(retried.Rollback.RestartedContainers, id) != 1 {
			t.Fatalf("retry did not preserve/dedupe restart history for %s: %#v", id, retried.Rollback)
		}
	}
}

func TestRollbackRestartsDurablyStoppingPreviousMemberEvenWhenInspectSeesRunning(t *testing.T) {
	ops := &fakeOperations{observed: map[string]ObservedContainer{
		"old-head": {ID: "old-head", Name: "old-head", Status: "running", Ownership: OwnershipObserved},
	}}
	engine, store, _ := newTestEngine(t, ops)
	when := time.Now().UTC()
	deployment := Deployment{
		ID: "dep-test", State: StatePending, Phase: PhaseCutover,
		PreviousMembers: []Member{{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: "old-head", Status: "stopping", Ownership: OwnershipObserved}},
		CreatedAt:       when, UpdatedAt: when,
	}
	if err := store.Put(deployment); err != nil {
		t.Fatal(err)
	}

	result, err := engine.Rollback(context.Background(), deployment.ID)
	if err != nil || result.State != StateRolledBack {
		t.Fatalf("rollback did not restore stopping previous member: deployment=%#v err=%v", result, err)
	}
	if !containsString(ops.events, "restart:spark-a:old-head") {
		t.Fatalf("rollback trusted a transient running observation despite durable stopping intent: %v", ops.events)
	}
	if countString(result.Rollback.RestartedContainers, "old-head") != 1 {
		t.Fatalf("rollback audit did not record the forced restart: %#v", result.Rollback)
	}
}

func TestStartRestartsDurablyStoppingManagedMemberEvenWhenInspectSeesRunning(t *testing.T) {
	ops := &fakeOperations{}
	engine, store, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	deployment.State = StateStopped
	for index := range deployment.Members {
		if deployment.Members[index].Role == bkc.MultiDeviceRoleHead {
			deployment.Members[index].Status = "stopping"
		} else {
			deployment.Members[index].Status = "running"
		}
	}
	if err := store.Put(deployment); err != nil {
		t.Fatal(err)
	}
	ops.events = nil

	result, err := engine.Start(context.Background(), deployment.ID, "test-api-key")
	if err != nil || result.State != StateRunning {
		t.Fatalf("start did not recover stopping managed member: deployment=%#v err=%v", result, err)
	}
	if !containsString(ops.events, "restart:spark-a:new-head") {
		t.Fatalf("start trusted a transient running observation despite durable stopping intent: %v", ops.events)
	}
}

func countString(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}

func containsString(values []string, want string) bool {
	return countString(values, want) > 0
}

func TestStopDoesNotClaimAbsentManagedMemberStopped(t *testing.T) {
	ops := &fakeOperations{}
	engine, store, _ := newTestEngine(t, ops)
	when := time.Now().UTC()
	deployment := Deployment{
		ID: "dep-test", State: StateRunning, Phase: PhaseRunning,
		Members:   []Member{{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: "yokai-deployment-dep-test-g1-head", Name: "yokai-deployment-dep-test-g1-head", Status: "running", Ownership: OwnershipManaged}},
		CreatedAt: when, UpdatedAt: when,
	}
	if err := store.Put(deployment); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Stop(context.Background(), deployment.ID)
	if deploymentsKind := ErrorKindOf(err); err == nil || deploymentsKind != ErrorConflict {
		t.Fatalf("absent member was treated as stopped: deployment=%#v err=%v kind=%s", result, err, deploymentsKind)
	}
	stored, getErr := store.Get(deployment.ID)
	if getErr != nil || stored.State != StateFailed || stored.Members[0].Status != "stopping" {
		t.Fatalf("durable stop state was not truthful: deployment=%#v err=%v", stored, getErr)
	}
}

func TestStoppedDeploymentStartsHeadThenWorkerAndRunsReadiness(t *testing.T) {
	ops := &fakeOperations{}
	engine, _, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Stop(context.Background(), deployment.ID); err != nil {
		t.Fatal(err)
	}
	ops.events = nil
	started, err := engine.Start(context.Background(), deployment.ID, "start-api-key")
	if err != nil || started.State != StateRunning {
		t.Fatalf("start failed: deployment=%#v err=%v", started, err)
	}
	wantPrefix := []string{"inspect:spark-a:new-head", "restart:spark-a:new-head", "inspect:spark-b:new-worker", "restart:spark-b:new-worker"}
	if len(ops.events) < len(wantPrefix) || !reflect.DeepEqual(ops.events[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("start order mismatch: got %v want prefix %v", ops.events, wantPrefix)
	}
}

func TestStoppedDeploymentRequiresOriginalLaunchKey(t *testing.T) {
	ops := &fakeOperations{}
	engine, _, _ := newTestEngine(t, ops)
	request := validRequest()
	request.APIKey = "original-launch-key"
	deployment, err := engine.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Stop(context.Background(), deployment.ID); err != nil {
		t.Fatal(err)
	}
	ops.expectedAPIKey = request.APIKey

	failed, err := engine.Start(context.Background(), deployment.ID, "different-key")
	if err == nil || failed.State != StateStopped {
		t.Fatalf("wrong key did not fail readiness back to stopped: deployment=%#v err=%v", failed, err)
	}
	for _, member := range failed.Members {
		if member.Status != "stopped" {
			t.Fatalf("wrong-key start left member active: %#v", member)
		}
	}

	started, err := engine.Start(context.Background(), deployment.ID, request.APIKey)
	if err != nil || started.State != StateRunning {
		t.Fatalf("original key did not restart group: deployment=%#v err=%v", started, err)
	}
}

func TestFailedStartCleanupContinuesAfterRequestCancellation(t *testing.T) {
	startCtx, cancel := context.WithCancel(context.Background())
	ops := &cancelingStartOperations{fakeOperations: &fakeOperations{}, cancel: cancel}
	engine, _, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Stop(context.Background(), deployment.ID); err != nil {
		t.Fatal(err)
	}
	ops.events = nil
	ops.cancelOnTest = true
	failed, err := engine.Start(startCtx, deployment.ID, "test-api-key")
	if err == nil || failed.State != StateStopped {
		t.Fatalf("canceled Start did not clean back to stopped: deployment=%#v err=%v", failed, err)
	}
	wantStops := []string{"stop:spark-b:new-worker", "stop:spark-a:new-head"}
	var gotStops []string
	for _, event := range ops.events {
		if strings.HasPrefix(event, "stop:") {
			gotStops = append(gotStops, event)
		}
	}
	if !reflect.DeepEqual(gotStops, wantStops) {
		t.Fatalf("canceled Start cleanup did not stop worker then head: events=%v", ops.events)
	}
}

func TestStartRunningInspectsBothRanksAndRecoversExitedMember(t *testing.T) {
	ops := &fakeOperations{}
	engine, _, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	ops.events = nil
	ops.statuses["new-worker"] = "exited"
	started, err := engine.Start(context.Background(), deployment.ID, "recovery-key")
	if err != nil || started.State != StateRunning {
		t.Fatalf("authenticated recovery failed: deployment=%#v err=%v", started, err)
	}
	if ops.testAPIKey != "recovery-key" {
		t.Fatalf("recovery readiness did not use supplied key")
	}
	for _, want := range []string{"inspect:spark-a:new-head", "inspect:spark-b:new-worker", "stop:spark-a:new-head", "restart:spark-a:new-head", "restart:spark-b:new-worker"} {
		if !containsString(ops.events, want) {
			t.Fatalf("recovery missed %q: %v", want, ops.events)
		}
	}
}

func TestStartRunningFailsClosedOnWrongManagedIdentity(t *testing.T) {
	ops := &fakeOperations{}
	engine, store, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	ops.observed = map[string]ObservedContainer{"new-worker": {ID: "new-worker", Name: deployment.Members[1].Name, Status: "running", Ownership: OwnershipObserved, Managed: false, Generation: 1, DeploymentID: deployment.ID, Role: bkc.MultiDeviceRoleWorker}}
	result, err := engine.Start(context.Background(), deployment.ID, "recovery-key")
	if err == nil || ErrorKindOf(err) != ErrorConflict || result.State != StateFailed {
		t.Fatalf("wrong ownership did not fail closed: deployment=%#v err=%v", result, err)
	}
	stored, _ := store.Get(deployment.ID)
	if stored.State != StateFailed {
		t.Fatalf("unsafe running identity remained durably green: %#v", stored)
	}
	for _, event := range ops.events {
		if strings.HasPrefix(event, "stop:") || strings.HasPrefix(event, "restart:") {
			t.Fatalf("identity mismatch caused mutation: %v", ops.events)
		}
	}
}

func TestReconcileStartingOneRankActiveStopsItWithDetachedCleanup(t *testing.T) {
	ops := &fakeOperations{}
	engine, store, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	deployment.State = StateStarting
	deployment.Phase = PhaseReadiness
	deployment.Progress = append(deployment.Progress,
		Progress{Phase: PhaseStarting, Action: "start_group", Status: "started"},
		Progress{Phase: PhaseReadiness, Action: "readiness", Status: "started"},
	)
	ops.statuses["new-worker"] = "stopped"
	if err := store.Put(deployment); err != nil {
		t.Fatal(err)
	}
	ops.events = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := engine.Reconcile(ctx); err != nil {
		t.Fatalf("starting reconciliation failed: %v", err)
	}
	stored, _ := store.Get(deployment.ID)
	if stored.State != StateStopped || !containsString(ops.events, "stop:spark-a:new-head") {
		t.Fatalf("one-rank-active start snapshot was not made safely stopped: deployment=%#v events=%v", stored, ops.events)
	}
}

func TestReconcileStoppingFinishesWorkerThenHead(t *testing.T) {
	ops := &fakeOperations{}
	engine, store, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	deployment.State = StateStopping
	deployment.Phase = PhaseStopping
	deployment.Progress = append(deployment.Progress, Progress{Action: "stop_group", Status: "started"})
	if err := store.Put(deployment); err != nil {
		t.Fatal(err)
	}
	ops.events = nil
	if err := engine.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	var stops []string
	for _, event := range ops.events {
		if strings.HasPrefix(event, "stop:") {
			stops = append(stops, event)
		}
	}
	if !reflect.DeepEqual(stops, []string{"stop:spark-b:new-worker", "stop:spark-a:new-head"}) {
		t.Fatalf("stopping reconciliation order mismatch: %v", ops.events)
	}
}

func TestReconcileFailedStopCompletesRemainingActiveRank(t *testing.T) {
	ops := &fakeOperations{}
	engine, store, _ := newTestEngine(t, ops)
	deployment, err := engine.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	deployment.State = StateFailed
	deployment.Phase = PhaseComplete
	deployment.Progress = append(deployment.Progress, Progress{Action: "stop_group", Status: "failed"})
	ops.statuses["new-worker"] = "stopped"
	if err := store.Put(deployment); err != nil {
		t.Fatal(err)
	}
	ops.events = nil
	if err := engine.Reconcile(context.Background()); err != nil {
		t.Fatalf("failed stop reconciliation failed: %v", err)
	}
	stored, _ := store.Get(deployment.ID)
	if stored.State != StateStopped || !containsString(ops.events, "stop:spark-a:new-head") {
		t.Fatalf("failed stop did not finish active head cleanup: deployment=%#v events=%v", stored, ops.events)
	}
}

func TestReconcileFailedRunningVerificationFailsClosedWithoutCreateRollback(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*fakeOperations, Deployment)
	}{
		{
			name: "worker unavailable",
			configure: func(ops *fakeOperations, _ Deployment) {
				ops.fail = "inspect:spark-b:new-worker"
			},
		},
		{
			name: "worker provenance mismatch",
			configure: func(ops *fakeOperations, deployment Deployment) {
				worker, _ := memberByRole(deployment.Members, bkc.MultiDeviceRoleWorker)
				ops.observed["new-worker"] = ObservedContainer{
					ID: "new-worker", Name: worker.Name, Status: "running", Ownership: OwnershipManaged,
					Managed: true, DeploymentID: deployment.ID, Generation: deployment.Generation + 1, Role: worker.Role,
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ops := &fakeOperations{observed: map[string]ObservedContainer{
				"old-head":   {ID: "old-head", Name: "old-head", Status: "stopped", Ownership: OwnershipObserved},
				"old-worker": {ID: "old-worker", Name: "old-worker", Status: "stopped", Ownership: OwnershipObserved},
			}}
			engine, store, _ := newTestEngine(t, ops)
			deployment, err := engine.Create(context.Background(), validRequest())
			if err != nil {
				t.Fatal(err)
			}
			deployment.PreviousMembers = []Member{
				{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: "old-head", Status: "stopped", Ownership: OwnershipObserved},
				{Role: bkc.MultiDeviceRoleWorker, DeviceID: "spark-b", ContainerID: "old-worker", Status: "stopped", Ownership: OwnershipObserved},
			}
			deployment.State = StateFailed
			deployment.Phase = PhaseComplete
			deployment.Progress = append(deployment.Progress, Progress{Phase: PhaseComplete, Action: "verify_running", Status: "failed"})
			test.configure(ops, deployment)
			if err := store.Put(deployment); err != nil {
				t.Fatal(err)
			}
			ops.events = nil

			if err := engine.Reconcile(context.Background()); err == nil {
				t.Fatal("unsafe running verification reconciliation unexpectedly succeeded")
			}
			stored, getErr := store.Get(deployment.ID)
			if getErr != nil || stored.State != StateFailed {
				t.Fatalf("unsafe member did not remain durably failed: deployment=%#v err=%v", stored, getErr)
			}
			if !containsString(ops.events, "stop:spark-a:new-head") {
				t.Fatalf("verified active head was not safely stopped: %v", ops.events)
			}
			for _, event := range ops.events {
				if strings.HasPrefix(event, "remove:") || strings.HasPrefix(event, "restart:") {
					t.Fatalf("running verification recovery performed create rollback effect %q: %v", event, ops.events)
				}
			}
		})
	}
}

func TestReconcilePendingAndRollbackFailedRetryRollback(t *testing.T) {
	for _, state := range []State{StatePending, StateRollbackFailed} {
		t.Run(string(state), func(t *testing.T) {
			ops := &fakeOperations{}
			engine, store, _ := newTestEngine(t, ops)
			deployment := Deployment{ID: "dep-test", State: state, Phase: PhaseRollback, Generation: 1,
				Members: []Member{{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: "yokai-deployment-dep-test-g1-head", Name: "yokai-deployment-dep-test-g1-head", Status: "removing", Ownership: OwnershipManaged, Generation: 1}}}
			if err := store.Put(deployment); err != nil {
				t.Fatal(err)
			}
			if err := engine.Reconcile(context.Background()); err != nil {
				t.Fatalf("reconcile %s: %v", state, err)
			}
			stored, _ := store.Get(deployment.ID)
			if stored.State != StateRolledBack {
				t.Fatalf("%s was not rolled back: %#v", state, stored)
			}
		})
	}
}

func TestReconcileSerializesWithLifecycleOperations(t *testing.T) {
	engine, _, _ := newTestEngine(t, &fakeOperations{})
	engine.mu.Lock()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- engine.Reconcile(context.Background())
	}()
	<-started
	select {
	case err := <-done:
		engine.mu.Unlock()
		t.Fatalf("reconcile bypassed engine transaction mutex: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	engine.mu.Unlock()
	if err := <-done; err != nil {
		t.Fatalf("serialized reconcile failed: %v", err)
	}
}

func TestCreateRejectsObservedGroupedMemberBeforeMutation(t *testing.T) {
	ops := &fakeOperations{observed: map[string]ObservedContainer{"old-head": {ID: "old-head", Name: "old-head", Status: "running", Ownership: OwnershipManaged, Managed: true, DeploymentID: "another-deployment", Generation: 2, Role: bkc.MultiDeviceRoleHead}}}
	engine, store, _ := newTestEngine(t, ops)
	request := validRequest()
	request.Bindings[1].ObservedContainerID = "old-head"
	if _, err := engine.Create(context.Background(), request); err == nil || ErrorKindOf(err) != ErrorConflict {
		t.Fatalf("grouped observed member was accepted: %v", err)
	}
	if len(store.List()) != 0 || len(ops.events) != 1 || !strings.HasPrefix(ops.events[0], "inspect:") {
		t.Fatalf("grouped-member rejection crossed mutation boundary: store=%v events=%v", store.List(), ops.events)
	}
}

func TestRollbackRejectsEveryManagedProvenanceMismatch(t *testing.T) {
	base := ObservedContainer{ID: "candidate-id", Name: "candidate-name", Status: "running", Ownership: OwnershipManaged, Managed: true, DeploymentID: "dep-test", Generation: 1, Role: bkc.MultiDeviceRoleHead}
	tests := map[string]func(*ObservedContainer){
		"managed":    func(value *ObservedContainer) { value.Managed = false },
		"ownership":  func(value *ObservedContainer) { value.Ownership = OwnershipObserved },
		"deployment": func(value *ObservedContainer) { value.DeploymentID = "other" },
		"generation": func(value *ObservedContainer) { value.Generation = 2 },
		"role":       func(value *ObservedContainer) { value.Role = bkc.MultiDeviceRoleWorker },
		"name":       func(value *ObservedContainer) { value.Name = "other-name" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			observed := base
			mutate(&observed)
			ops := &fakeOperations{observed: map[string]ObservedContainer{"candidate-name": observed}}
			engine, store, _ := newTestEngine(t, ops)
			deployment := Deployment{ID: "dep-test", State: StatePending, Generation: 1, Members: []Member{{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: "candidate-name", Name: "candidate-name", Status: "launching", Ownership: OwnershipManaged, Generation: 1}}}
			if err := store.Put(deployment); err != nil {
				t.Fatal(err)
			}
			if result, err := engine.Rollback(context.Background(), deployment.ID); err == nil || result.State != StateRollbackFailed {
				t.Fatalf("%s mismatch was not rejected: deployment=%#v err=%v", name, result, err)
			}
			for _, event := range ops.events {
				if strings.HasPrefix(event, "remove:") {
					t.Fatalf("%s mismatch removed candidate: %v", name, ops.events)
				}
			}
		})
	}
}

func TestRollbackReconstructsCrashLostMemberAudit(t *testing.T) {
	ops := &fakeOperations{observed: map[string]ObservedContainer{"old-head": {ID: "old-head", Name: "old-head", Status: "running", Ownership: OwnershipObserved}}}
	engine, store, _ := newTestEngine(t, ops)
	deployment := Deployment{ID: "dep-test", State: StateRollbackFailed, Phase: PhaseRollback, Generation: 1,
		Members:         []Member{{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: "yokai-deployment-dep-test-g1-head", Name: "yokai-deployment-dep-test-g1-head", Status: "removing", Ownership: OwnershipManaged, Generation: 1}},
		PreviousMembers: []Member{{Role: bkc.MultiDeviceRoleHead, DeviceID: "spark-a", ContainerID: "old-head", Name: "old-head", Status: "restarting", Ownership: OwnershipObserved}}}
	if err := store.Put(deployment); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Rollback(context.Background(), deployment.ID)
	if err != nil || result.State != StateRolledBack {
		t.Fatalf("crash audit recovery failed: deployment=%#v err=%v", result, err)
	}
	if !containsString(result.Rollback.RemovedCandidates, "yokai-deployment-dep-test-g1-head") || !containsString(result.Rollback.RestartedContainers, "old-head") {
		t.Fatalf("crash-lost audit was not reconstructed: %#v", result.Rollback)
	}
}
