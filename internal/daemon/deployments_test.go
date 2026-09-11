package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/deployments"
)

func TestDeploymentResponsesRedactInternalHashAndExposeStoppedGroups(t *testing.T) {
	store, err := deployments.OpenStore(filepath.Join(t.TempDir(), deployments.StoreFile))
	if err != nil {
		t.Fatal(err)
	}
	when := time.Unix(10, 0).UTC()
	want := deployments.Deployment{
		ID:             "dep-stopped",
		BKCID:          "recipe",
		IdempotencyKey: "key",
		RequestHash:    "internal-only",
		State:          deployments.StateStopped,
		Members: []deployments.Member{{
			Role: "head", DeviceID: "spark-a", ContainerID: "container-a", Status: "exited", Ownership: deployments.OwnershipManaged,
		}},
		CreatedAt: when,
		UpdatedAt: when,
	}
	if err := store.Put(want); err != nil {
		t.Fatal(err)
	}
	d := &Daemon{deploymentEngine: deployments.NewEngine(store, nil)}
	recorder := httptest.NewRecorder()
	d.handleListDeployments(recorder, httptest.NewRequest(http.MethodGet, "/deployments", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, secret := range []string{"internal-only", "request_hash", "api_key"} {
		if strings.Contains(body, secret) {
			t.Fatalf("deployment response exposed %q: %s", secret, body)
		}
	}
	var response struct {
		Deployments []deployments.Deployment `json:"deployments"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Deployments) != 1 || response.Deployments[0].State != deployments.StateStopped {
		t.Fatalf("stopped deployment was not visible: %#v", response.Deployments)
	}
}

func TestStartDeploymentErrorResponseIncludesPopulatedSafeDeployment(t *testing.T) {
	response := httptest.NewRecorder()
	writeDeploymentStartError(response, deployments.Deployment{
		ID: "dep-start", RequestHash: "internal-only", State: deployments.StateStopped,
		Members: []deployments.Member{{Role: "head", Status: "stopped"}},
	}, deployments.WrapError(deployments.ErrorDependency, "start deployment", errors.New("injected persist failure")))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("start error status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Error      string                 `json:"error"`
		Deployment deployments.Deployment `json:"deployment"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error != "deployment_dependency" || envelope.Deployment.ID != "dep-start" || envelope.Deployment.State != deployments.StateStopped {
		t.Fatalf("start error omitted the populated deployment: %#v", envelope)
	}
	if envelope.Deployment.RequestHash != "" || strings.Contains(response.Body.String(), "internal-only") {
		t.Fatalf("start error exposed internal deployment fields: %s", response.Body.String())
	}
}

func TestLegacyMemberLifecycleFailsClosedForManagedDeploymentMember(t *testing.T) {
	store, err := deployments.OpenStore(filepath.Join(t.TempDir(), deployments.StoreFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(deployments.Deployment{
		ID: "dep-1", State: deployments.StateRunning,
		Members: []deployments.Member{{DeviceID: "spark-a", ContainerID: "1234567890abcdef", Ownership: deployments.OwnershipManaged}},
	}); err != nil {
		t.Fatal(err)
	}
	d := &Daemon{deploymentEngine: deployments.NewEngine(store, nil)}
	request := httptest.NewRequest(http.MethodPost, "/containers/spark-a/1234567890ab/stop", nil)
	request.SetPathValue("deviceID", "spark-a")
	request.SetPathValue("containerID", "1234567890ab")
	recorder := httptest.NewRecorder()
	d.handleStopContainer(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected grouped member conflict, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "/deployments/dep-1") {
		t.Fatalf("missing grouped lifecycle guidance: %s", recorder.Body.String())
	}
}

func TestLegacyMemberLifecycleGuardMatchesManagedDeploymentName(t *testing.T) {
	store, err := deployments.OpenStore(filepath.Join(t.TempDir(), deployments.StoreFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(deployments.Deployment{
		ID: "dep-1", State: deployments.StateRunning,
		Members: []deployments.Member{{DeviceID: "spark-a", ContainerID: "1234567890abcdef", Name: "yokai-deployment-dep-1-g1-head", Ownership: deployments.OwnershipManaged}},
	}); err != nil {
		t.Fatal(err)
	}
	d := &Daemon{deploymentEngine: deployments.NewEngine(store, nil)}
	if err := d.guardGroupedMemberMutation("spark-a", "yokai-deployment-dep-1-g1-head"); err == nil {
		t.Fatal("managed deployment member name bypassed the legacy lifecycle guard")
	}
}

func TestSameContainerIDRequiresBothPrefixesAtLeastTwelveCharacters(t *testing.T) {
	if sameContainerID("1234567890abcdef", "123456") {
		t.Fatal("short compared selector matched a stored container ID prefix")
	}
	if !sameContainerID("1234567890abcdef", "1234567890ab") {
		t.Fatal("valid 12-character prefix did not match")
	}
}

func TestDeploymentMemberDeleteTimeoutOutlivesLaunchCleanupBarrier(t *testing.T) {
	minimum := deployments.DefaultCandidateLaunchCommandTimeout + deployments.DefaultCandidateLaunchSettleTimeout
	if got := deploymentMemberMutationTimeout(http.MethodDelete); got <= minimum {
		t.Fatalf("managed delete timeout %s does not outlive launch cleanup barrier %s", got, minimum)
	}
	if got := deploymentMemberMutationTimeout(http.MethodPost); got != deployments.DefaultMemberMutationRPCTimeout {
		t.Fatalf("ordinary member mutation timeout changed unexpectedly: %s", got)
	}
}

func TestObservedContainerMatchingRequiresExactNameOrUnambiguousLongPrefix(t *testing.T) {
	containers := []agentContainer{
		{ID: "1234567890abaaaaaaaa", Name: "old-head"},
		{ID: "1234567890abbbbbbbbb", Name: "old-worker"},
		{ID: "fedcba0987654321", Name: "another"},
	}
	if got, err := matchObservedContainer(containers, "old-head"); err != nil || got.Name != "old-head" {
		t.Fatalf("exact name did not match: %#v err=%v", got, err)
	}
	if _, err := matchObservedContainer(containers, "1234567890a"); deployments.ErrorKindOf(err) != deployments.ErrorValidation {
		t.Fatalf("short ID prefix was not a validation error: %v", err)
	}
	if _, err := matchObservedContainer(containers, "1234567890ab"); deployments.ErrorKindOf(err) != deployments.ErrorConflict {
		t.Fatalf("ambiguous ID prefix was not a conflict: %v", err)
	}
	if got, err := matchObservedContainer(containers, "fedcba098765"); err != nil || got.Name != "another" {
		t.Fatalf("unambiguous 12-char prefix did not match: %#v err=%v", got, err)
	}
}

func TestDeploymentErrorStatusMapping(t *testing.T) {
	for _, test := range []struct {
		err  error
		want int
	}{
		{deployments.WrapError(deployments.ErrorValidation, "test", errors.New("bad")), http.StatusBadRequest},
		{deployments.WrapError(deployments.ErrorConflict, "test", errors.New("busy")), http.StatusConflict},
		{deployments.WrapError(deployments.ErrorDependency, "test", errors.New("agent")), http.StatusBadGateway},
		{deployments.WrapError(deployments.ErrorUnavailable, "test", errors.New("offline")), http.StatusServiceUnavailable},
	} {
		if got, _ := deploymentErrorResponse(test.err, "fallback"); got != test.want {
			t.Fatalf("status mapping for %v: got %d want %d", test.err, got, test.want)
		}
	}
}

func TestAgentPreflightServiceUnavailableMapsToCoordinator503(t *testing.T) {
	err := classifyAgentPreflightError(&agentHTTPError{Status: http.StatusServiceUnavailable})
	if kind := deployments.ErrorKindOf(err); kind != deployments.ErrorUnavailable {
		t.Fatalf("got kind %q want %q: %v", kind, deployments.ErrorUnavailable, err)
	}
	if status, _ := deploymentErrorResponse(err, "fallback"); status != http.StatusServiceUnavailable {
		t.Fatalf("got HTTP %d want %d", status, http.StatusServiceUnavailable)
	}
	infoErr := classifyAgentPreflightDependency("agent system info", &agentHTTPError{Status: http.StatusServiceUnavailable})
	if kind := deployments.ErrorKindOf(infoErr); kind != deployments.ErrorUnavailable {
		t.Fatalf("system-info 503 got kind %q want %q: %v", kind, deployments.ErrorUnavailable, infoErr)
	}
}

func TestDeviceIsGB10RejectsSpoofedTag(t *testing.T) {
	device := config.Device{Tags: []string{bkc.DeviceGB10}}
	if deviceIsGB10(device, "NVIDIA GeForce RTX 4090") {
		t.Fatal("operator-editable GB10 tag overrode observed non-GB10 GPU")
	}
	if !deviceIsGB10(config.Device{}, "NVIDIA GB10") {
		t.Fatal("observed GB10 GPU was not accepted")
	}
}

func TestDeploymentAgentOperationErrorClassification(t *testing.T) {
	for _, test := range []struct {
		name           string
		err            error
		clientConflict bool
		want           deployments.ErrorKind
		wantAuth       bool
	}{
		{name: "transport", err: errors.New("connection refused"), want: deployments.ErrorUnavailable},
		{name: "inventory client capability", err: &agentHTTPError{Status: http.StatusBadRequest}, clientConflict: true, want: deployments.ErrorConflict},
		{name: "missing member", err: &agentHTTPError{Status: http.StatusNotFound}, want: deployments.ErrorConflict},
		{name: "agent failure", err: &agentHTTPError{Status: http.StatusInternalServerError}, want: deployments.ErrorDependency},
		{name: "service authorization", err: &agentHTTPError{Status: http.StatusBadGateway, Code: "service_unauthorized"}, want: deployments.ErrorDependency, wantAuth: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := deploymentAgentOperationError("test", test.err, test.clientConflict)
			if kind := deployments.ErrorKindOf(got); kind != test.want {
				t.Fatalf("got kind %q want %q: %v", kind, test.want, got)
			}
			if errors.Is(got, deployments.ErrServiceUnauthorized) != test.wantAuth {
				t.Fatalf("authorization sentinel mismatch: want=%v err=%v", test.wantAuth, got)
			}
		})
	}
}

func TestAgentResponseErrorDecodesWorstCaseEscapedPullDiagnostic(t *testing.T) {
	const deviceID = "spark-a"
	diagnostic := strings.Repeat("<\x00", 4*1024)
	if len(diagnostic) != 8*1024 {
		t.Fatalf("diagnostic length = %d, want %d", len(diagnostic), 8*1024)
	}
	message := "docker pull failed: exit status 1 (" + diagnostic + ")"

	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(map[string]string{"error": "pull_failed", "message": message}); err != nil {
		t.Fatal(err)
	}
	if body.Len() <= 32*1024 {
		t.Fatalf("escaped envelope length = %d, want more than the former 32 KiB limit", body.Len())
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/containers" {
			t.Fatalf("unexpected deploy request: %s %s", request.Method, request.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write(body.Bytes()); err != nil {
			t.Errorf("write agent error: %v", err)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.Devices = []config.Device{{ID: deviceID}}
	tunnels := NewTunnelPool(cfg)
	tunnels.tunnels[deviceID] = &tunnel{deviceID: deviceID, localPort: server.Listener.Addr().(*net.TCPAddr).Port, connected: true}
	aggregator := NewAggregator(cfg, tunnels)
	_, err := aggregator.Deploy(DeployRequest{DeviceID: deviceID})
	var responseErr *agentHTTPError
	if !errors.As(err, &responseErr) {
		t.Fatalf("error type = %T, want *agentHTTPError", err)
	}
	if responseErr.Code != "pull_failed" || responseErr.Message != message {
		t.Fatalf("agent error was not decoded: code=%q message length=%d", responseErr.Code, len(responseErr.Message))
	}
}

func TestWaitRunningFailsPromptlyForTerminalContainerStates(t *testing.T) {
	for _, status := range []string{"dead", "exited"} {
		t.Run(status, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requests++
				if request.Method != http.MethodGet || request.URL.String() != "/containers?scope=all" {
					t.Fatalf("unexpected inventory request: %s %s", request.Method, request.URL.String())
				}
				writeJSON(w, http.StatusOK, map[string]any{"containers": []map[string]any{{
					"id": "1234567890ab", "name": "candidate-head", "status": status, "ownership": "managed",
				}}})
			}))
			defer server.Close()

			const deviceID = "spark-a"
			port := server.Listener.Addr().(*net.TCPAddr).Port
			cfg := config.DefaultConfig()
			cfg.Devices = []config.Device{{ID: deviceID}}
			tunnels := NewTunnelPool(cfg)
			tunnels.tunnels[deviceID] = &tunnel{deviceID: deviceID, localPort: port, connected: true}
			d := &Daemon{cfg: cfg, tunnels: tunnels, aggregator: NewAggregator(cfg, tunnels)}
			ops := &daemonDeploymentOperations{daemon: d, waitInterval: 10 * time.Millisecond, waitTimeout: time.Second}

			started := time.Now()
			err := ops.WaitRunning(context.Background(), deviceID, "1234567890ab")
			if err == nil || !strings.Contains(err.Error(), status) {
				t.Fatalf("terminal %s was not reported: %v", status, err)
			}
			if elapsed := time.Since(started); elapsed > 250*time.Millisecond || requests != 1 {
				t.Fatalf("terminal %s was retried: elapsed=%s requests=%d", status, elapsed, requests)
			}
		})
	}
}

func TestDaemonDeploymentTestMapsAgentServiceUnauthorized(t *testing.T) {
	const (
		deviceID = "spark-a"
		secret   = "request-only-secret"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.String() != "/containers/container-head/test?require_metrics=true" {
			t.Fatalf("unexpected service-test request: %s %s", request.Method, request.URL.String())
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body["api_key"] != secret {
			t.Fatalf("request key was not forwarded transiently: body=%#v err=%v", body, err)
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "service_unauthorized", "message": "model service rejected the API key",
		})
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port
	cfg := config.DefaultConfig()
	cfg.Devices = []config.Device{{ID: deviceID}}
	tunnels := NewTunnelPool(cfg)
	tunnels.tunnels[deviceID] = &tunnel{deviceID: deviceID, localPort: port, connected: true}
	d := &Daemon{cfg: cfg, tunnels: tunnels, aggregator: NewAggregator(cfg, tunnels)}
	ops := &daemonDeploymentOperations{daemon: d}

	_, err := ops.Test(context.Background(), deviceID, "container-head", secret)
	if err == nil || !errors.Is(err, deployments.ErrServiceUnauthorized) || deployments.ErrorKindOf(err) != deployments.ErrorDependency {
		t.Fatalf("agent authorization code did not reach the deployment sentinel: %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("daemon deployment error retained request key: %v", err)
	}
}

func TestDeploymentLaunchPayloadKeepsSecretInOneStructuredRankZeroArg(t *testing.T) {
	const secret = "-sentinel-secret"
	payload := deploymentLaunchPayload{
		Image: "image", Name: "head", ExtraArgs: "sglang serve --node-rank 0", Args: []string{"--api-key=" + secret},
		Env: map[string]string{"NCCL_NET": "IB"},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), secret) != 1 || strings.Contains(payload.ExtraArgs, secret) || payload.Env["API_KEY"] != "" {
		t.Fatalf("launch payload secret boundary drifted: %s", data)
	}
	if len(payload.Args) != 1 || payload.Args[0] != "--api-key="+secret {
		t.Fatalf("secret was not one safe argv token: %#v", payload.Args)
	}
	if strings.Contains(string(data), "hf_token") {
		t.Fatalf("coordinated local-snapshot launch transported an unnecessary HF token: %s", data)
	}
}

func TestDeploymentLaunchRPCOutlivesCallerCancellation(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	launchCtx, cancelLaunch := detachedDeploymentLaunchContext(requestCtx)
	defer cancelLaunch()
	cancelRequest()
	select {
	case <-launchCtx.Done():
		t.Fatalf("deployment launch RPC inherited caller cancellation: %v", launchCtx.Err())
	case <-time.After(10 * time.Millisecond):
	}
}

func TestDaemonCapturesManagedLogTailWithTransientRedaction(t *testing.T) {
	const (
		deviceID = "spark-a"
		token    = "agent-token"
		sentinel = "request-sentinel-key"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/deployments/dep-test/members/container-head/logs/tail" {
			t.Fatalf("unexpected log capture request: %s %s", request.Method, request.URL.String())
		}
		if request.Header.Get("Authorization") != "Bearer "+token {
			t.Fatal("missing agent authorization")
		}
		if request.URL.Query().Get("generation") != "3" || request.URL.Query().Get("role") != "head" || request.URL.Query().Get("name") != "candidate-head" {
			t.Fatalf("missing member provenance query: %s", request.URL.RawQuery)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body["redact"] != sentinel {
			t.Fatalf("transient redaction was not sent in the request body: body=%#v err=%v", body, err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"tail": "scheduler exception " + sentinel + " --api-key=generic-secret",
		})
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port
	cfg := config.DefaultConfig()
	cfg.Devices = []config.Device{{ID: deviceID, AgentToken: token}}
	tunnels := NewTunnelPool(cfg)
	tunnels.tunnels[deviceID] = &tunnel{deviceID: deviceID, localPort: port, connected: true}
	d := &Daemon{cfg: cfg, tunnels: tunnels, aggregator: NewAggregator(cfg, tunnels)}
	ops := &daemonDeploymentOperations{daemon: d}
	capture, err := ops.CaptureManagedLogs(context.Background(), deployments.Deployment{ID: "dep-test"}, deployments.Member{
		Role: "head", Rank: 0, DeviceID: deviceID, ContainerID: "container-head", Name: "candidate-head", Generation: 3,
	}, sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(capture.Tail, "scheduler exception") || strings.Contains(capture.Tail, sentinel) || strings.Contains(capture.Tail, "generic-secret") {
		t.Fatalf("daemon returned unsafe or incomplete log tail: %q", capture.Tail)
	}
}
