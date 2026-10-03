package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
)

type tensorFoldRunnerFunc func(context.Context, string, []string, string, ...string) ([]byte, error)

func (f tensorFoldRunnerFunc) Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	return f(ctx, dir, env, name, args...)
}

func (f tensorFoldRunnerFunc) Start(context.Context, string, []string, io.Writer, string, ...string) (tensorFoldProcess, error) {
	return nil, errors.New("unexpected start")
}

type fakeTensorFoldProcess struct {
	done chan error
}

func (p *fakeTensorFoldProcess) Wait() error { return <-p.done }
func (p *fakeTensorFoldProcess) Stop() error {
	select {
	case p.done <- nil:
	default:
	}
	return nil
}
func (p *fakeTensorFoldProcess) Kill() error { return p.Stop() }

type fakeTensorFoldRunner struct {
	mu       sync.Mutex
	commands [][]string
	process  *fakeTensorFoldProcess
	started  bool
	launchID string
}

func (r *fakeTensorFoldRunner) Run(_ context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, append([]string{dir, strings.Join(env, " "), name}, args...))
	if name == "git" && len(args) >= 3 && args[2] == "rev-parse" {
		return []byte(bkc.GLM53FlashEXL3TensorFoldRecipeCommit + "\n"), nil
	}
	joined := strings.Join(args, " ")
	if (name == "docker" || name == "ssh") && strings.Contains(joined, "docker image inspect") || name == "docker" && strings.Contains(joined, "image inspect") {
		return []byte(bkc.GLM53FlashEXL3TensorFoldImagePatchHash + "\n"), nil
	}
	if (name == "docker" && len(args) > 0 && args[0] == "ps") || (name == "ssh" && strings.Contains(joined, "docker ps")) {
		if r.started {
			if name == "ssh" {
				return []byte(strings.Repeat("2", 64) + "\n"), nil
			}
			return []byte(strings.Repeat("1", 64) + "\n"), nil
		}
		return nil, nil
	}
	if name == "docker" && len(args) > 0 && args[0] == "inspect" {
		return tensorFoldInspectFixtureWithLaunchID("192.168.1.191", "0", r.launchID), nil
	}
	if name == "ssh" && strings.Contains(joined, "docker inspect") {
		return tensorFoldInspectFixtureWithLaunchID("", "1", r.launchID), nil
	}
	if name == "ip" {
		return []byte(`[{"dev":"enP2p1s0f0np0","src":"192.168.201.2"}]`), nil
	}
	if name == "ssh" && strings.Contains(joined, "ip -j route get") {
		return []byte(`[{"dev":"enP2p1s0f0np0","src":"192.168.201.1"}]`), nil
	}
	if (name == "cat" || name == "ssh") && strings.Contains(joined, "gid_attrs/types/3") {
		return []byte("RoCE v2\n"), nil
	}
	if (name == "cat" || name == "ssh") && strings.Contains(joined, "gid_attrs/ndevs/3") {
		return []byte("enP2p1s0f0np0\n"), nil
	}
	if name == "./stop.sh" {
		r.started = false
	}
	return nil, nil
}

func (r *fakeTensorFoldRunner) Start(_ context.Context, dir string, env []string, output io.Writer, name string, args ...string) (tensorFoldProcess, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = io.WriteString(output, "recipe supervisor started\n")
	r.commands = append(r.commands, append([]string{dir, strings.Join(env, " "), name}, args...))
	r.process = &fakeTensorFoldProcess{done: make(chan error, 1)}
	r.started = true
	r.launchID = tensorFoldLaunchIDFromEnvironmentFile(dir)
	return r.process, nil
}

func TestTensorFoldManagerOwnsReservationAndHeadRecipeLifecycle(t *testing.T) {
	root := t.TempDir()
	runner := &fakeTensorFoldRunner{}
	manager := newTensorFoldManager(root, runner)
	workerManager := newTensorFoldManager(t.TempDir(), runner)
	recipeDir := manager.recipePath()
	if err := os.MkdirAll(filepath.Join(recipeDir, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	manager.pinnedFiles = make(map[string]string)
	for _, name := range []string{"README.md", "CHANGELOG.md", "scripts/config.sh", "scripts/local.sh.example", "scripts/nodes.sh", "scripts/prepare.sh", "start.sh", "stop.sh"} {
		body := []byte("pinned " + name + "\n")
		if err := os.WriteFile(filepath.Join(recipeDir, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		manager.pinnedFiles[name] = hex.EncodeToString(sum[:])
	}

	worker := validTensorFoldResourceRequest(false)
	workerResource, err := workerManager.create(context.Background(), worker)
	if err != nil {
		t.Fatal(err)
	}
	if workerResource.Status != "running" || workerResource.ID != tensorFoldResourceID(worker.Name) {
		t.Fatalf("worker reservation mismatch: %#v", workerResource)
	}

	head := validTensorFoldResourceRequest(true)
	headResource, err := manager.create(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}
	if headResource.Status != "starting" || headResource.ID != tensorFoldResourceID(head.Name) {
		t.Fatalf("head resource mismatch: %#v", headResource)
	}
	observedHead, err := manager.observe(context.Background(), head.Name)
	if err != nil || observedHead.Status != "running" {
		t.Fatalf("running recipe ranks did not promote resource observation: resource=%#v err=%v", observedHead, err)
	}
	environment, err := os.ReadFile(filepath.Join(recipeDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(environment), "HOST=192.168.1.191\n") || strings.Contains(string(environment), "0.0.0.0") {
		t.Fatalf("unsafe rendered environment: %s", environment)
	}
	if err := manager.stop(context.Background(), head.Name); err != nil {
		t.Fatal(err)
	}
	if got, err := manager.inspect(head.Name); err != nil || got.Status != "stopped" {
		t.Fatalf("head was not durably stopped: resource=%#v err=%v", got, err)
	}
	if err := manager.remove(context.Background(), head.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.inspect(head.Name); !os.IsNotExist(err) {
		t.Fatalf("removed head resource remains inspectable: %v", err)
	}
	if _, err := os.Stat(recipeDir); err != nil {
		t.Fatalf("resource removal deleted pinned recipe or caches: %v", err)
	}

	runner.mu.Lock()
	commands := append([][]string(nil), runner.commands...)
	runner.mu.Unlock()
	if !containsTensorFoldCommand(commands, recipeDir, "./start.sh", "FOREGROUND=1") || !containsTensorFoldCommand(commands, recipeDir, "./stop.sh", "") {
		t.Fatalf("recipe scripts were not lifecycle authority: commands=%#v", commands)
	}
	transportDir := manager.tensorFoldSSHTransportDir(headResource.LaunchID)
	for _, executable := range []string{"./start.sh", "./stop.sh"} {
		if !containsTensorFoldCommand(commands, recipeDir, executable, "PATH="+transportDir+":") ||
			!containsTensorFoldCommand(commands, recipeDir, executable, "RSYNC_RSH="+filepath.Join(transportDir, "ssh")) {
			t.Fatalf("%s did not receive the generated strict SSH/rsync transport: commands=%#v", executable, commands)
		}
	}
}

func TestTensorFoldGeneratedTransportEnforcesStrictIdentityForSSHAndRsync(t *testing.T) {
	root := t.TempDir()
	manager := newTensorFoldManager(root, &fakeTensorFoldRunner{})
	logPath := filepath.Join(root, "ssh-args")
	fakeSSH := filepath.Join(root, "real-ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TF_SSH_TEST_LOG\"\nexit 23\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manager.sshExecutable = fakeSSH
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	if err := manager.ensureTensorFoldSSHTransport(resource); err != nil {
		t.Fatal(err)
	}
	transportDir := manager.tensorFoldSSHTransportDir(resource.LaunchID)
	configPath := filepath.Join(transportDir, "config")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"BatchMode yes", "StrictHostKeyChecking yes", "CheckHostIP yes",
		"UserKnownHostsFile ~/.ssh/known_hosts ~/.ssh/known_hosts2",
		"GlobalKnownHostsFile /etc/ssh/ssh_known_hosts /etc/ssh/ssh_known_hosts2",
	} {
		if !strings.Contains(string(config), required+"\n") {
			t.Fatalf("generated SSH config lacks %q:\n%s", required, config)
		}
	}
	environment := append(manager.commandEnvironment(false, resource.LaunchID), "TF_SSH_TEST_LOG="+logPath)
	sshCommand := exec.Command("/bin/sh", "-c", "ssh -o BatchMode=yes worker.example true")
	sshCommand.Env = environment
	if err := sshCommand.Run(); err == nil {
		t.Fatal("fake SSH unexpectedly succeeded")
	}
	assertTensorFoldTransportInvocation(t, logPath, configPath)

	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := os.WriteFile(source, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	rsyncCommand := exec.Command("/usr/bin/rsync", "-a", "-e", "ssh -o BatchMode=yes", source, "worker.example:/tmp/destination")
	rsyncCommand.Env = environment
	if err := rsyncCommand.Run(); err == nil {
		t.Fatal("rsync through fake SSH unexpectedly succeeded")
	}
	assertTensorFoldTransportInvocation(t, logPath, configPath)
}

func assertTensorFoldTransportInvocation(t *testing.T, logPath, configPath string) {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	arguments := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(arguments) < 2 || arguments[0] != "-F" || arguments[1] != configPath {
		t.Fatalf("generated SSH wrapper did not force its config: %#v", arguments)
	}
}

func TestTensorFoldStopOwnsSupervisorBeforeContainersExist(t *testing.T) {
	var upstreamStop bool
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case name == "docker" && len(args) > 0 && args[0] == "ps":
			return nil, nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return nil, nil
		case name == "./stop.sh":
			upstreamStop = true
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command %s %v", name, args)
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	stageTensorFoldExecutionFixture(t, manager, resource)
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	process := &fakeTensorFoldProcess{done: make(chan error, 1)}
	trackTestTensorFoldProcess(t, manager, resource, process)

	if err := manager.stop(context.Background(), resource.Name); err != nil {
		t.Fatal(err)
	}
	if _, active := manager.processes[resource.Name]; upstreamStop || active {
		t.Fatalf("head stop did not safely own the pre-container launch: upstream=%v active=%v", upstreamStop, active)
	}
}

func TestTensorFoldReadinessProvesEffectivePolicyAndSmoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = io.WriteString(w, `{"data":[{"id":"GLM-5.3-Flash-EXL3"}]}`)
		case "/tokenize":
			_, _ = io.WriteString(w, `{"count":1,"max_model_len":1048576,"tokens":[1]}`)
		case "/health":
			_, _ = io.WriteString(w, `{}`)
		case "/metrics":
			_, _ = io.WriteString(w, "tensorfold:requests_running 0\ntensorfold_health:context_length 1048576\ntensorfold_health:streams_max 4\ntensorfold_health:pool_tokens 2922496\n")
		case "/v1/chat/completions":
			data, _ := io.ReadAll(r.Body)
			if strings.Contains(string(data), `"stream":true`) {
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\ndata: [DONE]\n\n")
				return
			}
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"OK"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if name == "docker" && len(args) > 0 && args[0] == "ps" {
			return []byte(strings.Repeat("1", 64) + "\n"), nil
		}
		if name == "docker" && len(args) == 2 && args[0] == "inspect" {
			return tensorFoldInspectFixture("192.168.1.191", "0"), nil
		}
		if name == "ssh" && strings.Contains(joined, "docker ps") {
			return []byte(strings.Repeat("2", 64) + "\n"), nil
		}
		if name == "ssh" && len(args) > 0 {
			return tensorFoldInspectFixture("", "1"), nil
		}
		return nil, fmt.Errorf("unexpected command %s %v", name, args)
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	manager.httpClient = server.Client()
	manager.serviceBaseURL = func(tensorFoldResource) string { return server.URL }
	request := validTensorFoldResourceRequest(true)
	resource := tensorFoldResource{
		ID: tensorFoldResourceID(request.Name), Name: request.Name, DeploymentID: request.DeploymentID,
		Generation: request.Generation, Role: request.Role, Status: "running", Ownership: OwnershipManaged, Managed: true,
		Repository: request.Repository, Commit: request.Commit, WorkerUser: request.WorkerUser,
		WorkerAddress: request.WorkerAddress, HeadFabricAddress: request.HeadFabricAddress,
		ServiceAddress: request.ServiceAddress, ServicePort: request.ServicePort,
		LaunchID:        strings.Repeat("a", 64),
		LaunchStartedAt: time.Now().Add(-time.Minute).UTC(),
	}
	result, err := manager.testResource(context.Background(), resource)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || !result.MetricsReady || result.Model != bkc.GLM53FlashEXL3TensorFoldServedModel || !strings.EqualFold(result.Response, "ok") {
		t.Fatalf("readiness result mismatch: %#v", result)
	}
}

func TestTensorFoldReadinessRejectsHeadWorkerPolicyDisagreement(t *testing.T) {
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if (name == "docker" && len(args) > 0 && args[0] == "ps") || (name == "ssh" && strings.Contains(joined, "docker ps")) {
			if name == "ssh" {
				return []byte(strings.Repeat("2", 64) + "\n"), nil
			}
			return []byte(strings.Repeat("1", 64) + "\n"), nil
		}
		if name == "docker" {
			return tensorFoldInspectFixture("192.168.1.191", "0"), nil
		}
		if name == "ssh" {
			fixture := tensorFoldInspectFixture("", "1")
			return bytes.Replace(fixture, []byte(`"--context","1048576"`), []byte(`"--context","524288"`), 1), nil
		}
		return nil, fmt.Errorf("unexpected command %s %v", name, args)
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	if _, err := manager.testResource(context.Background(), resource); err == nil || !strings.Contains(err.Error(), "--context") {
		t.Fatalf("head/worker context disagreement passed readiness: %v", err)
	}
}

func TestTensorFoldInventoryExposesManagedHeadAndNativeMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, "tensorfold:requests_running 2\ntensorfold:requests_waiting 1\n")
	}))
	defer server.Close()
	manager := newTensorFoldManager(t.TempDir(), tensorFoldRunnerFunc(func(context.Context, string, []string, string, ...string) ([]byte, error) {
		return nil, errors.New("unexpected command")
	}))
	manager.httpClient = server.Client()
	manager.serviceBaseURL = func(tensorFoldResource) string { return server.URL }
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "running")
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	containers, err := manager.inventory(context.Background())
	if err != nil || len(containers) != 1 {
		t.Fatalf("TensorFold inventory mismatch: containers=%#v err=%v", containers, err)
	}
	container := containers[0]
	if container.Ownership != OwnershipManaged || inferenceBackend(container) != "tensorfold" || container.VLLMMetrics == nil || container.VLLMMetrics.RequestsRunning != 2 || container.VLLMMetrics.RequestsWaiting != 1 {
		t.Fatalf("TensorFold monitoring projection mismatch: %#v", container)
	}
}

func TestTensorFoldStopRefusesForeignContainerNameCollision(t *testing.T) {
	var commands []string
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		if name == "docker" && len(args) > 0 && args[0] == "ps" {
			return []byte(strings.Repeat("3", 64) + "\n"), nil
		}
		if name == "docker" && len(args) > 0 && args[0] == "inspect" {
			fixture := tensorFoldInspectFixtureWithIdentity("192.168.1.191", "0", strings.Repeat("a", 64), strings.Repeat("3", 64))
			fixture = bytes.Replace(fixture, []byte(bkc.GLM53FlashEXL3TensorFoldImage), []byte("foreign/image:latest"), 1)
			return fixture, nil
		}
		return nil, fmt.Errorf("unexpected command: %s", command)
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	request := validTensorFoldResourceRequest(true)
	resource := resourceFromTensorFoldRequest(request, "running")
	stageTensorFoldExecutionFixture(t, manager, resource)
	if err := manager.writeResource(resource); err != nil {
		t.Fatal(err)
	}
	if err := manager.stop(context.Background(), resource.Name); err == nil || !strings.Contains(err.Error(), "pinned digest") {
		t.Fatalf("foreign container collision was not refused: %v", err)
	}
	for _, command := range commands {
		if strings.Contains(command, "./stop.sh") {
			t.Fatalf("foreign container was exposed to recipe stop: %v", commands)
		}
	}
}

func TestTensorFoldReconcileCleansPartialOwnedStartAndKeepsWorkerReservation(t *testing.T) {
	var stopped bool
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case name == "docker" && len(args) > 0 && args[0] == "ps":
			if stopped {
				return nil, nil
			}
			return []byte(strings.Repeat("1", 64) + "\n"), nil
		case name == "docker" && len(args) > 0 && args[0] == "inspect":
			return tensorFoldInspectFixture("192.168.1.191", "0"), nil
		case name == "docker" && len(args) > 0 && args[0] == "stop":
			return nil, nil
		case name == "docker" && len(args) > 0 && args[0] == "rm":
			stopped = true
			return nil, nil
		case name == "ssh" && strings.Contains(joined, "docker ps"):
			return nil, nil
		case name == "./stop.sh":
			return nil, errors.New("name-based stop must not run for a partial launch")
		default:
			return nil, fmt.Errorf("unexpected command: %s", joined)
		}
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	headRequest := validTensorFoldResourceRequest(true)
	workerRequest := validTensorFoldResourceRequest(false)
	headResource := resourceFromTensorFoldRequest(headRequest, "starting")
	stageTensorFoldExecutionFixture(t, manager, headResource)
	if err := manager.writeResource(headResource); err != nil {
		t.Fatal(err)
	}
	if err := manager.writeResource(resourceFromTensorFoldRequest(workerRequest, "running")); err != nil {
		t.Fatal(err)
	}
	if err := manager.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	head, err := manager.inspect(headRequest.Name)
	if err != nil || head.Status != "stopped" || !stopped {
		t.Fatalf("partial head start was not safely cleaned: resource=%#v stopped=%v err=%v", head, stopped, err)
	}
	worker, err := manager.inspect(workerRequest.Name)
	if err != nil || worker.Status != "running" {
		t.Fatalf("worker reservation was lost during head recovery: resource=%#v err=%v", worker, err)
	}
}

func TestTensorFoldPreflightFailsClosedWhenStrictWorkerSSHIsLost(t *testing.T) {
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		if name == "ssh" {
			return nil, errors.New("worker unreachable")
		}
		if name == "docker" && len(args) > 0 && args[0] == "ps" {
			return nil, nil
		}
		if name == "docker" && len(args) > 1 && args[0] == "image" && args[1] == "inspect" {
			return []byte(bkc.GLM53FlashEXL3TensorFoldImagePatchHash + "\n"), nil
		}
		return []byte("ok\n"), nil
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	request := validTensorFoldResourceRequest(true)
	if err := manager.preflight(context.Background(), request); err == nil || !strings.Contains(err.Error(), "worker SSH") {
		t.Fatalf("lost strict worker SSH passed preflight: %v", err)
	}
	if _, err := os.Stat(manager.resourcePath(request.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed preflight crossed the resource mutation boundary: %v", err)
	}
}

func TestTensorFoldPreflightRejectsExistingSingletonReservation(t *testing.T) {
	runner := &fakeTensorFoldRunner{}
	manager := newTensorFoldManager(t.TempDir(), runner)
	first := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(false), "running")
	if err := manager.writeResource(first); err != nil {
		t.Fatal(err)
	}
	second := validTensorFoldResourceRequest(false)
	second.DeploymentID = "other"
	second.Name = "yokai-deployment-other-g1-worker"
	if err := manager.preflight(context.Background(), second); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("preflight accepted a second node reservation: %v", err)
	}
}

func TestTensorFoldPreflightDoesNotRequireRecipeCheckoutAsWorkingDirectory(t *testing.T) {
	runner := tensorFoldRunnerFunc(func(_ context.Context, dir string, _ []string, name string, args ...string) ([]byte, error) {
		if dir != "" {
			return nil, fmt.Errorf("unexpected preflight working directory %s", dir)
		}
		if name == "docker" && len(args) > 0 && args[0] == "ps" {
			return nil, nil
		}
		if name == "docker" && len(args) > 1 && args[0] == "image" && args[1] == "inspect" {
			return []byte(bkc.GLM53FlashEXL3TensorFoldImagePatchHash + "\n"), nil
		}
		return []byte("ok\n"), nil
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	if err := manager.preflight(context.Background(), validTensorFoldResourceRequest(false)); err != nil {
		t.Fatalf("fresh worker preflight depended on an unstaged recipe checkout: %v", err)
	}
}

func TestTensorFoldImageProvenanceRequiresPinnedPatchLabelOnBothNodes(t *testing.T) {
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	for _, test := range []struct {
		name   string
		worker bool
	}{
		{name: "head"},
		{name: "worker", worker: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
				if name == "docker" || name == "ssh" {
					return []byte("wrong-patch-set\n"), nil
				}
				return nil, fmt.Errorf("unexpected command %s %v", name, args)
			})
			manager := newTensorFoldManager(t.TempDir(), runner)
			if err := manager.verifyTensorFoldImageProvenance(context.Background(), resource, test.worker); err == nil || !strings.Contains(err.Error(), "patch provenance") {
				t.Fatalf("unpinned image patch label was accepted: %v", err)
			}
		})
	}
}

func TestTensorFoldFreshCheckoutNormalizesGroupWritablePinnedFiles(t *testing.T) {
	body := []byte("pinned recipe content\n")
	sum := sha256.Sum256(body)
	var manager *tensorFoldManager
	runner := tensorFoldRunnerFunc(func(_ context.Context, dir string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if name == "git" && strings.Contains(joined, "checkout --quiet --detach") {
			if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o700); err != nil {
				return nil, err
			}
			path := filepath.Join(dir, "CHANGELOG.md")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				return nil, err
			}
			if err := os.Chmod(path, 0o664); err != nil {
				return nil, err
			}
			return nil, nil
		}
		if name == "git" && strings.Contains(joined, "rev-parse HEAD") {
			return []byte(bkc.GLM53FlashEXL3TensorFoldRecipeCommit + "\n"), nil
		}
		if name == "git" && strings.Contains(joined, "status --porcelain") {
			return nil, nil
		}
		return nil, nil
	})
	manager = newTensorFoldManager(t.TempDir(), runner)
	manager.pinnedFiles = map[string]string{"CHANGELOG.md": hex.EncodeToString(sum[:])}
	if err := manager.ensurePinnedRecipe(context.Background()); err != nil {
		t.Fatalf("fresh checkout did not normalize inherited group write permission: %v", err)
	}
	info, err := os.Stat(filepath.Join(manager.recipePath(), "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("fresh checkout mode = %o, want 644", info.Mode().Perm())
	}
}

func TestTensorFoldPinnedRecipeRejectsIgnoredAdditions(t *testing.T) {
	resource := resourceFromTensorFoldRequest(validTensorFoldResourceRequest(true), "starting")
	var inspectedIgnored bool
	runner := tensorFoldRunnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if name == "git" && strings.Contains(joined, "rev-parse HEAD") {
			return []byte(bkc.GLM53FlashEXL3TensorFoldRecipeCommit + "\n"), nil
		}
		if name == "git" && strings.Contains(joined, "status --porcelain") {
			inspectedIgnored = strings.Contains(joined, "--ignored=matching")
			if inspectedIgnored {
				return []byte("!! patches/injected.patch\n"), nil
			}
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected command %s %v", name, args)
	})
	manager := newTensorFoldManager(t.TempDir(), runner)
	stageTensorFoldExecutionFixture(t, manager, resource)
	if err := manager.ensurePinnedRecipe(context.Background()); err == nil || !strings.Contains(err.Error(), "unowned changes") {
		t.Fatalf("ignored recipe addition was accepted: %v", err)
	}
	if !inspectedIgnored {
		t.Fatal("recipe checkout did not inspect ignored additions")
	}
}

func resourceFromTensorFoldRequest(request tensorFoldResourceRequest, status string) tensorFoldResource {
	resource := tensorFoldResource{
		ID: tensorFoldResourceID(request.Name), Name: request.Name, DeploymentID: request.DeploymentID,
		Generation: request.Generation, Role: request.Role, Status: status, Ownership: OwnershipManaged, Managed: true,
		Repository: request.Repository, Commit: request.Commit, WorkerUser: request.WorkerUser,
		WorkerAddress: request.WorkerAddress, HeadFabricAddress: request.HeadFabricAddress,
		ServiceAddress: request.ServiceAddress, ServicePort: request.ServicePort,
		LaunchStartedAt: time.Now().Add(-time.Minute).UTC(),
	}
	if request.Role == bkc.MultiDeviceRoleHead {
		resource.LaunchID = strings.Repeat("a", 64)
	}
	return resource
}

func tensorFoldInspectFixture(host, rank string) []byte {
	return tensorFoldInspectFixtureWithLaunchID(host, rank, strings.Repeat("a", 64))
}

func tensorFoldInspectFixtureWithLaunchID(host, rank, launchID string) []byte {
	containerID := strings.Repeat("1", 64)
	if rank == "1" {
		containerID = strings.Repeat("2", 64)
	}
	return tensorFoldInspectFixtureWithIdentity(host, rank, launchID, containerID)
}

func tensorFoldInspectFixtureWithIdentity(host, rank, launchID, containerID string) []byte {
	command := []string{
		"tensorfold", "serve", "/root/.cache/huggingface/hub/models--Mia-AiLab--GLM-5.3-Flash-EXL3-4bpw-TensorFold/snapshots/078455ffe6472f9a52fbc1139f58b9db2881b25c",
		"--tp", "2", "--rank", rank, "--master", "192.168.201.2", "--master-port", "29551",
		"--drafter", "/root/.cache/huggingface/hub/models--incoai--GLM-5.3-Flash-DFlash2/snapshots/bf582e4eacc1810f76656d1811693ff6c6737d2a",
		"--context", "1048576", "--parallel", "4", "--vision",
	}
	if rank == "0" {
		command = append(command, "--name", "GLM-5.3-Flash-EXL3", "--host", host, "--port", "8888")
	}
	value := []map[string]any{{
		"Id":      containerID,
		"Name":    "/" + tensorFoldContainerName,
		"Created": time.Now().UTC().Format(time.RFC3339Nano),
		"Config": map[string]any{
			"Image": bkc.GLM53FlashEXL3TensorFoldImage,
			"Env":   []string{"HF_HUB_OFFLINE=1", "TF_GLM_KV=fp8", "TF_GLM_DENSE=q4", "TENSORFOLD_MEMORY_RESERVE_GIB=14.5", "TF_GLM_CACHE_GIB=12.5", "NCCL_SOCKET_IFNAME=enP2p1s0f0np0", "NCCL_IB_HCA=roceP2p1s0f0", "NCCL_IB_GID_INDEX=3", "TENSORFOLD_YOKAI_LAUNCH_ID=" + launchID},
			"Cmd":   command,
		},
		"State": map[string]any{"Running": true},
	}}
	data, _ := json.Marshal(value)
	return data
}

func tensorFoldLaunchIDFromEnvironmentFile(recipeDir string) string {
	data, _ := os.ReadFile(filepath.Join(recipeDir, ".env"))
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "TENSORFOLD_YOKAI_LAUNCH_ID="); ok {
			return value
		}
	}
	return strings.Repeat("a", 64)
}

func trackTestTensorFoldProcess(t *testing.T, manager *tensorFoldManager, resource tensorFoldResource, process tensorFoldProcess) {
	t.Helper()
	logFile, err := os.CreateTemp(t.TempDir(), "tensorfold-launch")
	if err != nil {
		t.Fatal(err)
	}
	launch := &tensorFoldLaunch{id: resource.LaunchID, process: process, done: make(chan struct{})}
	manager.processes[resource.Name] = launch
	go manager.waitForProcess(resource.Name, launch, logFile)
}

func containsTensorFoldCommand(commands [][]string, directory, executable, requiredEnvironment string) bool {
	for _, command := range commands {
		if len(command) >= 3 && command[0] == directory && command[2] == executable && (requiredEnvironment == "" || strings.Contains(command[1], requiredEnvironment)) {
			return true
		}
	}
	return false
}

func validTensorFoldResourceRequest(head bool) tensorFoldResourceRequest {
	role := bkc.MultiDeviceRoleWorker
	name := "yokai-deployment-test-g1-worker"
	if head {
		role = bkc.MultiDeviceRoleHead
		name = "yokai-deployment-test-g1-head"
	}
	request := tensorFoldResourceRequest{
		Name: name, DeploymentID: "test", Generation: 1, Role: role,
		Repository: bkc.GLM53FlashEXL3TensorFoldRecipeRepository, Commit: bkc.GLM53FlashEXL3TensorFoldRecipeCommit,
	}
	if head {
		request.WorkerUser = "dell"
		request.WorkerAddress = "192.168.201.1"
		request.HeadFabricAddress = "192.168.201.2"
		request.ServiceAddress = "192.168.1.191"
		request.ServicePort = bkc.GLM53FlashEXL3TensorFoldServicePort
	}
	return request
}

func stageTensorFoldExecutionFixture(t *testing.T, manager *tensorFoldManager, resource tensorFoldResource) {
	t.Helper()
	recipeDir := manager.recipePath()
	if err := os.MkdirAll(filepath.Join(recipeDir, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("#!/bin/sh\n")
	if err := os.WriteFile(filepath.Join(recipeDir, "stop.sh"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	manager.pinnedFiles = map[string]string{"stop.sh": hex.EncodeToString(sum[:])}
	request := tensorFoldResourceRequest{
		Name: resource.Name, DeploymentID: resource.DeploymentID, Generation: resource.Generation, Role: resource.Role,
		WorkerUser: resource.WorkerUser, WorkerAddress: resource.WorkerAddress, HeadFabricAddress: resource.HeadFabricAddress,
		ServiceAddress: resource.ServiceAddress, ServicePort: resource.ServicePort, Repository: resource.Repository, Commit: resource.Commit,
	}
	environment, err := renderTensorFoldRecipeEnvironment(request, resource.LaunchID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recipeDir, ".env"), []byte(environment), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.ensureTensorFoldSSHTransport(resource); err != nil {
		t.Fatal(err)
	}
}

func TestTensorFoldRecipeEnvironmentPinsPolicyAndPrivateBind(t *testing.T) {
	req := tensorFoldResourceRequest{
		Name: "yokai-deployment-dep-123-g1-head", DeploymentID: "dep-123", Generation: 1, Role: bkc.MultiDeviceRoleHead,
		WorkerUser: "dell", WorkerAddress: "192.168.201.1", HeadFabricAddress: "192.168.201.2", ServiceAddress: "192.168.1.191", ServicePort: 8888,
		Repository: bkc.GLM53FlashEXL3TensorFoldRecipeRepository, Commit: bkc.GLM53FlashEXL3TensorFoldRecipeCommit,
	}
	env, err := renderTensorFoldRecipeEnvironment(req)
	if err != nil {
		t.Fatal(err)
	}
	wants := []string{
		"WORKER=dell@192.168.201.1", "FABRIC_PEER=192.168.201.1", "MASTER_ADDR=192.168.201.2", "MASTER_PORT=29551",
		"HOST=192.168.1.191", "PORT=8888", "TP=2", "NCCL_RAILS=1", "CONTEXT=1048576", "PARALLEL=4", "KV=fp8",
		"DRAFTER=dflash2", "DENSE=q4", "VISION=1", "MEMORY_RESERVE_GIB=14.5", "KV_POOL_GIB=12.5", "HF_HUB_OFFLINE=1",
		"MODEL_ID=Mia-AiLab/GLM-5.3-Flash-EXL3-4bpw-TensorFold",
		"MODEL_REVISION=078455ffe6472f9a52fbc1139f58b9db2881b25c",
		"DFLASH2_ID=incoai/GLM-5.3-Flash-DFlash2",
		"DFLASH2_REVISION=bf582e4eacc1810f76656d1811693ff6c6737d2a",
		"TF_GLM_MULTI_WATCHDOG_EXIT=0",
		"IMAGE=ghcr.io/miaai-lab/glm-5.3-flash-exl3-2x-dgx-sparks-tensorfold@sha256:14f15591eae5d6a540f09218d3852068962fe5381371bbfefe0e9194cd834529",
		"PULL=1",
	}
	for _, want := range wants {
		if !strings.Contains(env, want+"\n") {
			t.Fatalf("generated recipe environment missing %q:\n%s", want, env)
		}
	}
	for _, forbidden := range []string{"HOST=0.0.0.0", "HOST=::", "CONTEXT=0", "PREPARE=0", "PULL=0"} {
		if strings.Contains(env, forbidden) {
			t.Fatalf("generated recipe environment contains forbidden setting %q", forbidden)
		}
	}
}

func TestTensorFoldRecipeRequestRejectsInjectionAndProvenanceDrift(t *testing.T) {
	valid := tensorFoldResourceRequest{
		Name: "yokai-deployment-dep-123-g1-head", DeploymentID: "dep-123", Generation: 1, Role: bkc.MultiDeviceRoleHead,
		WorkerUser: "dell", WorkerAddress: "192.168.201.1", HeadFabricAddress: "192.168.201.2", ServiceAddress: "192.168.1.191", ServicePort: 8888,
		Repository: bkc.GLM53FlashEXL3TensorFoldRecipeRepository, Commit: bkc.GLM53FlashEXL3TensorFoldRecipeCommit,
	}
	tests := map[string]func(*tensorFoldResourceRequest){
		"worker user shell": func(r *tensorFoldResourceRequest) { r.WorkerUser = "dell;id" },
		"worker host shell": func(r *tensorFoldResourceRequest) { r.WorkerAddress = "192.168.201.1$(id)" },
		"wildcard bind":     func(r *tensorFoldResourceRequest) { r.ServiceAddress = "0.0.0.0" },
		"wrong port":        func(r *tensorFoldResourceRequest) { r.ServicePort = 8000 },
		"mutable source":    func(r *tensorFoldResourceRequest) { r.Commit = "main" },
		"other repository":  func(r *tensorFoldResourceRequest) { r.Repository = "https://example.invalid/recipe.git" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			req := valid
			mutate(&req)
			if _, err := renderTensorFoldRecipeEnvironment(req); err == nil {
				t.Fatal("unsafe or drifted request was accepted")
			}
		})
	}
}

func TestTensorFoldPinnedRecipeFilesAreComplete(t *testing.T) {
	want := map[string]string{
		"README.md":                "e852819f6634e770426fdc6aee2d4adf5a929a4ee9a1b2fa5f559df63505895d",
		"CHANGELOG.md":             "9c40743b270bdb6688ff0a24c2e23e924d6e0d2552c33d9a3e73a60b3376a746",
		"scripts/config.sh":        "9cfc3a5bac37c48b48db5d2a96c1e775d5be8e18842dc5d17b71db2032282fc4",
		"scripts/banner.sh":        "d05fef4009bceba9764097edc259590a18c48bc72b290681cab398ace95ab0c8",
		"scripts/local.sh.example": "c407b41e0bf3121256e442b6142a04603b808ff49f960c59097b0e5b42eba502",
		"scripts/nodes.sh":         "21f41546fb61e381fbc0cb3432dd2e74b74d7ea9bc36b18ee617eb0692ab21a5",
		"scripts/prepare.sh":       "03842903bbf8d879540c588e1507f706a35239b12101151505eb4f9236f01d2c",
		"start.sh":                 "72f3f96c90373e5a1f24898c78b539f943b418b7f0be52b1a1b0ef8c30a88118",
		"stop.sh":                  "84168a2f184a6e72cf848c5db5f8c2d585e1481671af6a21e3227583966ea5ef",
	}
	if len(tensorFoldPinnedRecipeFiles) != len(want) {
		t.Fatalf("pinned critical file count = %d, want %d", len(tensorFoldPinnedRecipeFiles), len(want))
	}
	for path, hash := range want {
		if tensorFoldPinnedRecipeFiles[path] != hash {
			t.Fatalf("pinned hash for %s = %q, want %q", path, tensorFoldPinnedRecipeFiles[path], hash)
		}
	}
}
