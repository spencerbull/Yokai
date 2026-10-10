package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
)

const tensorFoldContainerName = "glm53-flash-tf"

// Statuses for a head resource whose containers exist but are not both running:
// "needs-restart" means the agent is attempting a bounded docker-start restart;
// "failed" means the restart budget is exhausted and the containers are left
// in place for operator inspection (never auto-deleted by reconcile).
const (
	tensorFoldStatusNeedsRestart = "needs-restart"
	tensorFoldStatusFailed       = "failed"
)

// tensorFoldRestartBackoff delays between restart attempts (1st, 2nd, 3rd).
var tensorFoldRestartBackoff = []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second}

const tensorFoldRestartMaxAttempts = 3

// tensorFoldRestartStableWindow is how long a pair must stay running before a
// restart crash-loop count is forgiven. A crash/watchdog loop that keeps
// flapping within this window exhausts its budget instead of restarting
// forever.
const tensorFoldRestartStableWindow = 10 * time.Minute

// tensorFoldStatusAutoRestartable lists the head statuses for which reconcile
// may auto-restart a complete-but-dead container pair. "stopping" (an
// in-progress operator stop) and other transitional states are excluded so the
// agent never overrides an operator action or re-launches a half-done
// generation.
var tensorFoldStatusAutoRestartable = map[string]bool{
	"running":                    true,
	"exited":                     true,
	tensorFoldStatusNeedsRestart: true,
}

// tensorFoldRankLabel is the "head"/"worker" label for a rank flag in logs and
// errors.
func tensorFoldRankLabel(worker bool) string {
	if worker {
		return "worker"
	}
	return "head"
}

// Pinned recipe file hashes, keyed by recipe commit. The agent verifies the
// set matching the resource's commit; unknown commits fail closed.
var tensorFoldPinnedRecipeFilesByCommit = map[string]map[string]string{
	bkc.GLM53FlashEXL3TensorFoldRecipeCommit: {
		"README.md":                "32364e8760e687431457dfb972c6e85d9229607c8bc5dab5a4420742272b5de8",
		"CHANGELOG.md":             "2fd5120dafd0e2098eb8fa37457bfdc327bc53c9b37d458754d599967325a2d1",
		"scripts/config.sh":        "ccf093bc53c9023ca0b7a87e0cad835cf7c33673726cfb01599bbf3b3667a7c5",
		"scripts/banner.sh":        "d05fef4009bceba9764097edc259590a18c48bc72b290681cab398ace95ab0c8",
		"scripts/local.sh.example": "49f63656725e0c9d21b019eda9507b240776978fe137505fe0cbf5d2d23332df",
		"scripts/nodes.sh":         "5a5b8e1425cfea152caf1ea762923ed3926d30bea691b2241f17e6ae5904bc7a",
		"scripts/prepare.sh":       "0934ef783a596c3eb5bff8cede4d578e438223e15effc8a894b70549fae9ad24",
		"start.sh":                 "7c65a1698481e4da879bd6f1e4093fc098482d9e32e69f93c73d11599fa0cf5b",
		"stop.sh":                  "84168a2f184a6e72cf848c5db5f8c2d585e1481671af6a21e3227583966ea5ef",
	},
	bkc.GLM53FlashEXL3TensorFoldRecipeCommitV14: {
		"README.md":                "e852819f6634e770426fdc6aee2d4adf5a929a4ee9a1b2fa5f559df63505895d",
		"CHANGELOG.md":             "9c40743b270bdb6688ff0a24c2e23e924d6e0d2552c33d9a3e73a60b3376a746",
		"scripts/config.sh":        "9cfc3a5bac37c48b48db5d2a96c1e775d5be8e18842dc5d17b71db2032282fc4",
		"scripts/banner.sh":        "d05fef4009bceba9764097edc259590a18c48bc72b290681cab398ace95ab0c8",
		"scripts/local.sh.example": "c407b41e0bf3121256e442b6142a04603b808ff49f960c59097b0e5b42eba502",
		"scripts/nodes.sh":         "21f41546fb61e381fbc0cb3432dd2e74b74d7ea9bc36b18ee617eb0692ab21a5",
		"scripts/prepare.sh":       "03842903bbf8d879540c588e1507f706a35239b12101151505eb4f9236f01d2c",
		"start.sh":                 "72f3f96c90373e5a1f24898c78b539f943b418b7f0be52b1a1b0ef8c30a88118",
		"stop.sh":                  "84168a2f184a6e72cf848c5db5f8c2d585e1481671af6a21e3227583966ea5ef",
	},
}

type tensorFoldResourceRequest struct {
	Name              string `json:"name"`
	DeploymentID      string `json:"deployment_id"`
	Generation        int    `json:"generation"`
	Role              string `json:"role"`
	WorkerUser        string `json:"worker_user,omitempty"`
	WorkerAddress     string `json:"worker_address,omitempty"`
	HeadFabricAddress string `json:"head_fabric_address,omitempty"`
	ServiceAddress    string `json:"service_address,omitempty"`
	ServicePort       int    `json:"service_port,omitempty"`
	Repository        string `json:"repository"`
	Commit            string `json:"commit"`
}

type tensorFoldResource struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	DeploymentID      string    `json:"deployment_id"`
	Generation        int       `json:"generation"`
	Role              string    `json:"role"`
	Status            string    `json:"status"`
	Ownership         string    `json:"ownership"`
	Managed           bool      `json:"managed"`
	Repository        string    `json:"repository"`
	Commit            string    `json:"commit"`
	WorkerUser        string    `json:"worker_user,omitempty"`
	WorkerAddress     string    `json:"worker_address,omitempty"`
	HeadFabricAddress string    `json:"head_fabric_address,omitempty"`
	ServiceAddress    string    `json:"service_address,omitempty"`
	ServicePort       int       `json:"service_port,omitempty"`
	LaunchID          string    `json:"launch_id,omitempty"`
	HeadContainerID   string    `json:"head_container_id,omitempty"`
	WorkerContainerID string    `json:"worker_container_id,omitempty"`
	LaunchStartedAt   time.Time `json:"launch_started_at,omitempty"`
	// ReadinessDeadline is when the agent stops a launch that has not proven
	// semantic readiness. Zero once ready, and on records predating it.
	ReadinessDeadline time.Time `json:"readiness_deadline,omitempty"`
	// RestartAttempts/LastRestartAt track the bounded docker-start restarts
	// the agent has attempted for a not-launch-active resource whose
	// containers exist but are not both running (e.g. after a reboot).
	RestartAttempts int       `json:"restartAttempts,omitempty"`
	LastRestartAt   time.Time `json:"lastRestartAt,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type tensorFoldProcess interface {
	Wait() error
	Stop() error
	Kill() error
}

type tensorFoldLaunch struct {
	id      string
	process tensorFoldProcess
	done    chan struct{}
	waitErr error
}

type tensorFoldCommandRunner interface {
	Run(context.Context, string, []string, string, ...string) ([]byte, error)
	Start(context.Context, string, []string, io.Writer, string, ...string) (tensorFoldProcess, error)
}

type execTensorFoldProcess struct{ command *exec.Cmd }

func (p *execTensorFoldProcess) Wait() error { return p.command.Wait() }
func (p *execTensorFoldProcess) Stop() error {
	if p.command.Process == nil {
		return nil
	}
	return tensorFoldSignalProcessGroup(p.command.Process.Pid, false)
}
func (p *execTensorFoldProcess) Kill() error {
	if p.command.Process == nil {
		return nil
	}
	return tensorFoldSignalProcessGroup(p.command.Process.Pid, true)
}

type execTensorFoldRunner struct{}

func (execTensorFoldRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = env
	output, err := command.CombinedOutput()
	if len(output) > 256<<10 {
		output = output[len(output)-(256<<10):]
	}
	if err != nil {
		return output, fmt.Errorf("%s failed: %w", filepath.Base(name), err)
	}
	return output, nil
}

func (execTensorFoldRunner) Start(ctx context.Context, dir string, env []string, output io.Writer, name string, args ...string) (tensorFoldProcess, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = env
	command.Stdout = output
	command.Stderr = output
	if err := tensorFoldConfigureCommand(command); err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &execTensorFoldProcess{command: command}, nil
}

type tensorFoldManager struct {
	root                 string
	runner               tensorFoldCommandRunner
	pinnedFilesOverride  map[string]string
	mu                   sync.Mutex
	processes            map[string]*tensorFoldLaunch
	httpClient           *http.Client
	metricsHTTPClient    *http.Client
	serviceBaseURL       func(tensorFoldResource) string
	beforeWaiterCleanup  func()
	sshExecutable        string
	fenceProcesses       func(marker string) error
	startRecipeContainer func(ctx context.Context, resource tensorFoldResource, worker bool, identifier string) error
}

func newTensorFoldManager(root string, runner tensorFoldCommandRunner) *tensorFoldManager {
	sshExecutable, _ := exec.LookPath("ssh")
	m := &tensorFoldManager{
		root: root, runner: runner,
		processes:         make(map[string]*tensorFoldLaunch),
		sshExecutable:     sshExecutable,
		httpClient:        &http.Client{Timeout: 3 * time.Minute},
		metricsHTTPClient: inventoryMetricsHTTPClient,
		fenceProcesses:    fenceTensorFoldProcesses,
		serviceBaseURL: func(resource tensorFoldResource) string {
			return "http://" + net.JoinHostPort(resource.ServiceAddress, strconv.Itoa(resource.ServicePort))
		},
	}
	m.startRecipeContainer = m.startRecipeContainerDefault
	return m
}

// startRecipeContainerDefault docker-starts one recipe rank by its recorded
// container ID through the same node-command path the recipe inspection uses
// (local exec for the head, SSH for the worker).
func (m *tensorFoldManager) startRecipeContainerDefault(ctx context.Context, resource tensorFoldResource, worker bool, identifier string) error {
	if _, err := m.runTensorFoldNodeCommand(ctx, resource, worker, "docker", "start", identifier); err != nil {
		return fmt.Errorf("start %s TensorFold container %s: %w", tensorFoldRankLabel(worker), identifier, err)
	}
	return nil
}

func tensorFoldResourceID(name string) string { return "tensorfold:" + name }

func newTensorFoldLaunchID() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate TensorFold launch identity: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func (m *tensorFoldManager) recipePath(commit string) string {
	return filepath.Join(m.root, "recipes", commit)
}

func (m *tensorFoldManager) resourcePath(name string) string {
	return filepath.Join(m.root, "resources", name+".json")
}

func (m *tensorFoldManager) logPath(name string) string {
	return filepath.Join(m.root, "logs", name+".log")
}

func (m *tensorFoldManager) create(ctx context.Context, request tensorFoldResourceRequest) (tensorFoldResource, error) {
	head := request.Role == bkc.MultiDeviceRoleHead
	if err := validateTensorFoldResourceRequest(request, head); err != nil {
		return tensorFoldResource{}, err
	}
	if err := m.preflight(ctx, request); err != nil {
		return tensorFoldResource{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := os.Lstat(m.resourcePath(request.Name)); err == nil {
		return tensorFoldResource{}, fmt.Errorf("TensorFold resource already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return tensorFoldResource{}, err
	}
	if err := m.ensureSingletonReservation(request.Name); err != nil {
		return tensorFoldResource{}, err
	}
	resource := tensorFoldResource{
		ID: tensorFoldResourceID(request.Name), Name: request.Name, DeploymentID: request.DeploymentID,
		Generation: request.Generation, Role: request.Role, Status: "running", Ownership: OwnershipManaged, Managed: true,
		Repository: request.Repository, Commit: request.Commit, WorkerUser: request.WorkerUser,
		WorkerAddress: request.WorkerAddress, HeadFabricAddress: request.HeadFabricAddress,
		ServiceAddress: request.ServiceAddress, ServicePort: request.ServicePort,
		UpdatedAt: time.Now().UTC(),
	}
	if !head {
		if err := m.writeResource(resource); err != nil {
			return tensorFoldResource{}, err
		}
		return resource, nil
	}
	if err := m.ensurePinnedRecipe(ctx, request.Commit); err != nil {
		return tensorFoldResource{}, err
	}
	if err := m.verifyHeadLaunchBoundary(ctx, tensorFoldResourceFromRequest(request)); err != nil {
		return tensorFoldResource{}, err
	}
	launchID, err := newTensorFoldLaunchID()
	if err != nil {
		return tensorFoldResource{}, err
	}
	resource.LaunchID = launchID
	environment, err := renderTensorFoldRecipeEnvironment(request, launchID)
	if err != nil {
		return tensorFoldResource{}, err
	}
	if err := m.writeRecipeEnvironment(environment, request.Commit); err != nil {
		return tensorFoldResource{}, fmt.Errorf("write TensorFold environment: %w", err)
	}
	if err := m.ensureTensorFoldSSHTransport(resource); err != nil {
		return tensorFoldResource{}, fmt.Errorf("write TensorFold SSH transport: %w", err)
	}
	resource.Status = "starting"
	resource.LaunchStartedAt = time.Now().UTC()
	resource.ReadinessDeadline = resource.LaunchStartedAt.Add(tensorFoldPreparationDeadline)
	if err := m.writeResource(resource); err != nil {
		return tensorFoldResource{}, err
	}
	logFile, err := m.openLog(request.Name)
	if err != nil {
		_ = os.Remove(m.resourcePath(request.Name))
		return tensorFoldResource{}, err
	}
	process, err := m.runner.Start(context.Background(), m.recipePath(request.Commit), m.commandEnvironment(true, launchID), logFile, "./start.sh")
	if err != nil {
		_ = logFile.Close()
		resource.Status = "failed"
		_ = m.writeResource(resource)
		return tensorFoldResource{}, fmt.Errorf("start pinned TensorFold recipe: %w", err)
	}
	launch := &tensorFoldLaunch{id: launchID, process: process, done: make(chan struct{})}
	m.processes[request.Name] = launch
	go m.waitForProcess(request.Name, launch, logFile)
	return resource, nil
}

func (m *tensorFoldManager) stop(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	resource, err := m.readResource(name)
	if err != nil {
		return err
	}
	return m.stopResourceLocked(ctx, resource)
}

func (m *tensorFoldManager) stopGeneration(ctx context.Context, expected tensorFoldResource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.readResource(expected.Name)
	if err != nil {
		return err
	}
	if !sameTensorFoldGeneration(expected, current) {
		return nil
	}
	return m.stopResourceLocked(ctx, current)
}

func (m *tensorFoldManager) stopResourceLocked(ctx context.Context, resource tensorFoldResource) error {
	if resource.Status == "stopped" {
		return nil
	}
	resource.Status = "stopping"
	if err := m.writeResource(resource); err != nil {
		return err
	}
	if resource.Role == bkc.MultiDeviceRoleHead {
		if err := m.joinTensorFoldLaunchLocked(ctx, resource); err != nil {
			resource.Status = "failed"
			_ = m.writeResource(resource)
			return fmt.Errorf("join TensorFold launcher before cleanup: %w", err)
		}
		if err := m.verifyPinnedRecipeForExecution(resource); err != nil {
			resource.Status = "failed"
			_ = m.writeResource(resource)
			return fmt.Errorf("refuse recipe stop with unverified source or environment: %w", err)
		}
		headExists, headRunning, headID, err := m.inspectRecipeContainerIdentity(ctx, resource, false)
		if err != nil {
			resource.Status = "failed"
			_ = m.writeResource(resource)
			return fmt.Errorf("refuse recipe stop without exact head ownership: %w", err)
		}
		workerExists, workerRunning, workerID, err := m.inspectRecipeContainerIdentity(ctx, resource, true)
		if err != nil {
			resource.Status = "failed"
			_ = m.writeResource(resource)
			return fmt.Errorf("refuse recipe stop without exact worker ownership: %w", err)
		}
		if headExists {
			resource.HeadContainerID = headID
		}
		if workerExists {
			resource.WorkerContainerID = workerID
		}
		if err := m.writeResource(resource); err != nil {
			return err
		}
		var stopErr error
		switch {
		case headExists && workerExists:
			_, stopErr = m.runner.Run(ctx, m.recipePath(resource.Commit), m.commandEnvironment(false, resource.LaunchID), "./stop.sh")
		case headExists:
			stopErr = m.removeExactRecipeContainer(ctx, resource, false, headID, headRunning)
		case workerExists:
			stopErr = m.removeExactRecipeContainer(ctx, resource, true, workerID, workerRunning)
		}
		if stopErr != nil {
			resource.Status = "failed"
			_ = m.writeResource(resource)
			return fmt.Errorf("stop pinned TensorFold recipe: %w", stopErr)
		}
		if err := m.verifyRecipeContainerAbsent(ctx, resource, false, resource.HeadContainerID); err != nil {
			resource.Status = "failed"
			_ = m.writeResource(resource)
			return fmt.Errorf("verify head TensorFold container absent after stop: %w", err)
		}
		if err := m.verifyRecipeContainerAbsent(ctx, resource, true, resource.WorkerContainerID); err != nil {
			resource.Status = "failed"
			_ = m.writeResource(resource)
			return fmt.Errorf("verify worker TensorFold container absent after stop: %w", err)
		}
	}
	current, err := m.readResource(resource.Name)
	if err != nil {
		return fmt.Errorf("re-read TensorFold resource after stop: %w", err)
	}
	if !sameTensorFoldGeneration(resource, current) {
		return fmt.Errorf("TensorFold resource generation changed during stop; reservation retained")
	}
	current.Status = "stopped"
	return m.writeResource(current)
}

func (m *tensorFoldManager) joinTensorFoldLaunchLocked(ctx context.Context, resource tensorFoldResource) error {
	if err := m.joinInMemoryLaunchLocked(ctx, resource); err != nil {
		return err
	}
	return m.fenceLauncherLocked(resource.LaunchID)
}

// fenceLauncherLocked kills every local process carrying this launch's
// marker. A launcher that outlived an agent restart is absent from
// m.processes, and descendants can escape the process-group kill; either
// could otherwise create fixed-name containers after cleanup.
func (m *tensorFoldManager) fenceLauncherLocked(launchID string) error {
	if !tensorFoldLaunchIDPattern.MatchString(launchID) {
		return nil
	}
	if err := m.fenceProcesses(m.tensorFoldLaunchMarker(launchID)); err != nil {
		return fmt.Errorf("fence TensorFold launcher: %w", err)
	}
	return nil
}

func (m *tensorFoldManager) joinInMemoryLaunchLocked(ctx context.Context, resource tensorFoldResource) error {
	launch := m.processes[resource.Name]
	if launch == nil || launch.id != resource.LaunchID {
		return nil
	}
	killErr := launch.process.Kill()
	select {
	case <-launch.done:
		if m.processes[resource.Name] == launch {
			delete(m.processes, resource.Name)
		}
		return errors.Join(killErr, tensorFoldExpectedKillResult(launch.waitErr))
	case <-ctx.Done():
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-launch.done:
			if m.processes[resource.Name] == launch {
				delete(m.processes, resource.Name)
			}
			return errors.Join(killErr, tensorFoldExpectedKillResult(launch.waitErr), ctx.Err())
		case <-timer.C:
			return errors.Join(killErr, ctx.Err(), fmt.Errorf("TensorFold launcher did not reap after forced process-group termination"))
		}
	}
}

// SIGKILL is the expected result of our deliberate launcher cancellation,
// not a cleanup failure. Preserve all other exit and cancellation errors.
func tensorFoldExpectedKillResult(err error) error {
	if tensorFoldExitWasIntentionalKill(err) {
		return nil
	}
	return err
}

func (m *tensorFoldManager) removeExactRecipeContainer(ctx context.Context, resource tensorFoldResource, worker bool, identifier string, running bool) error {
	if identifier == "" {
		return fmt.Errorf("refuse exact cleanup without a durable container ID")
	}
	if running {
		if _, err := m.runTensorFoldNodeCommand(ctx, resource, worker, "docker", "stop", identifier); err != nil {
			return fmt.Errorf("stop exact partial TensorFold container %s: %w", identifier, err)
		}
	}
	if _, err := m.runTensorFoldNodeCommand(ctx, resource, worker, "docker", "rm", identifier); err != nil {
		return fmt.Errorf("remove exact partial TensorFold container %s: %w", identifier, err)
	}
	return nil
}

func (m *tensorFoldManager) verifyRecipeContainerAbsent(ctx context.Context, resource tensorFoldResource, worker bool, identifier string) error {
	exists, err := m.recipeContainerNameExists(ctx, resource, worker)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("fixed container name remains present")
	}
	if identifier == "" {
		return nil
	}
	exists, err = m.recipeContainerIDExists(ctx, resource, worker, identifier)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("durable container ID %s remains present", identifier)
	}
	return nil
}

func (m *tensorFoldManager) recipeContainerIDExists(ctx context.Context, resource tensorFoldResource, worker bool, identifier string) (bool, error) {
	output, err := m.runTensorFoldNodeCommand(ctx, resource, worker, "docker", "ps", "-a", "--no-trunc", "--filter", "id="+identifier, "--format", "{{.ID}}")
	if err != nil {
		return false, err
	}
	identifiers := strings.Fields(string(output))
	for _, observed := range identifiers {
		if observed == identifier {
			return true, nil
		}
	}
	if len(identifiers) != 0 {
		return false, fmt.Errorf("exact container ID query returned a different identifier")
	}
	return false, nil
}

func (m *tensorFoldManager) reconcile(ctx context.Context) error {
	directory := filepath.Join(m.root, "resources")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var joined []error
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		resource, readErr := m.inspect(name)
		if readErr != nil {
			joined = append(joined, fmt.Errorf("resource %s: %w", name, readErr))
			continue
		}
		if resource.Role != bkc.MultiDeviceRoleHead || resource.Status == "stopped" {
			continue
		}
		if !resource.ReadinessDeadline.IsZero() && time.Now().After(resource.ReadinessDeadline) {
			// Running containers are not readiness. The coordinator owns the
			// readiness gate and normally rolls back first; this frees the
			// node only when no coordinator is left to do so.
			if err := m.stopGeneration(ctx, resource); err != nil {
				joined = append(joined, fmt.Errorf("resource %s stop after readiness deadline: %w", name, err))
			}
			continue
		}
		m.mu.Lock()
		launch := m.processes[name]
		launchActive := launch != nil && launch.id == resource.LaunchID
		if launchActive {
			select {
			case <-launch.done:
				launchActive = false
			default:
			}
		}
		m.mu.Unlock()
		if launchActive {
			localExists, localRunning, localID, localErr := m.inspectRecipeContainerIdentity(ctx, resource, false)
			workerExists, workerRunning, workerID, workerErr := m.inspectRecipeContainerIdentity(ctx, resource, true)
			if localErr != nil || workerErr != nil {
				joined = append(joined, fmt.Errorf("resource %s active preparation observation: %w", name, errors.Join(localErr, workerErr)))
				continue
			}
			expected := resource
			if localExists {
				resource.HeadContainerID = localID
			}
			if workerExists {
				resource.WorkerContainerID = workerID
			}
			if localExists && workerExists && localRunning && workerRunning {
				resource.Status = "running"
			}
			_, _, writeErr := m.compareAndWriteResource(expected, resource)
			if writeErr != nil {
				joined = append(joined, writeErr)
			}
			continue
		}
		localExists, localRunning, localErr := m.inspectRecipeContainer(ctx, resource, false)
		workerExists, workerRunning, workerErr := m.inspectRecipeContainer(ctx, resource, true)
		if localErr != nil || workerErr != nil {
			// An unreachable node means an unknown state: skip this resource
			// without any destructive action, so we never run cleanup we
			// could not verify.
			joined = append(joined, fmt.Errorf("resource %s ownership reconciliation: %w", name, errors.Join(localErr, workerErr)))
			continue
		}
		if localExists && workerExists && localRunning && workerRunning {
			expected := resource
			// Forgive the crash-loop budget only after the pair has been
			// running for longer than the stability window; a repeated
			// crash/watchdog-exit loop must still be able to reach "failed".
			if resource.RestartAttempts != 0 &&
				!resource.LastRestartAt.IsZero() &&
				time.Since(resource.LastRestartAt) >= tensorFoldRestartStableWindow {
				resource.RestartAttempts = 0
				resource.LastRestartAt = time.Time{}
			}
			resource.Status = "running"
			_, _, writeErr := m.compareAndWriteResource(expected, resource)
			if writeErr != nil {
				joined = append(joined, writeErr)
			}
			continue
		}
		if localExists && workerExists && (!localRunning || !workerRunning) {
			// Oct 9 2026: both hosts rebooted and both ranks existed but were
			// EXITED (restart policy "no"); the old partial-start branch ran
			// stop.sh (docker rm -f on both ranks) and deleted the deployment.
			// A complete-but-dead pair is recoverable: bounded restart, never
			// delete.
			if tensorFoldStatusAutoRestartable[resource.Status] {
				if err := m.attemptBoundedRestart(ctx, resource); err != nil {
					joined = append(joined, fmt.Errorf("resource %s bounded restart: %w", name, err))
				}
			}
			// Any other head status (stopping, failed, planned, starting,
			// needs-restart already parked by backoff, ...) is left to the
			// operator/coordinator path: no start, no delete, no error.
			continue
		}
		if localExists || workerExists {
			// Exactly one rank exists: a genuine partial start (the other was
			// never created), so the existing cleanup is the right call.
			if stopErr := m.stopGeneration(ctx, resource); stopErr != nil {
				joined = append(joined, fmt.Errorf("resource %s partial-start cleanup: %w", name, stopErr))
			}
			continue
		}
		if err := m.fenceAndMarkStopped(resource); err != nil {
			joined = append(joined, fmt.Errorf("resource %s: %w", name, err))
		}
	}
	return errors.Join(joined...)
}

// attemptBoundedRestart recovers a head resource whose launch is not active
// but whose containers exist and are not both running (reboot, crash,
// watchdog exit). It docker-starts the worker first, then the head, using
// the recorded container IDs, with a bounded attempt count and exponential
// backoff. After the budget is exhausted the resource is marked failed and
// the containers are left in place for operator inspection; reconcile never
// deletes them.
func (m *tensorFoldManager) attemptBoundedRestart(ctx context.Context, resource tensorFoldResource) error {
	if !resource.LastRestartAt.IsZero() {
		backoff := tensorFoldRestartBackoff[0]
		idx := resource.RestartAttempts - 1
		if idx < 0 {
			idx = 0
		}
		if idx < len(tensorFoldRestartBackoff) {
			backoff = tensorFoldRestartBackoff[idx]
		}
		if since := time.Since(resource.LastRestartAt); since < backoff {
			// Backoff window not elapsed: hold this pass, retry later.
			return nil
		}
	}
	if resource.RestartAttempts >= tensorFoldRestartMaxAttempts {
		if resource.Status != tensorFoldStatusFailed {
			return m.recordRestartState(resource, tensorFoldStatusFailed, nil)
		}
		return nil
	}
	workerID := resource.WorkerContainerID
	headID := resource.HeadContainerID
	if workerID == "" || headID == "" {
		// No durable ID to target: surface it, but do not guess or clean up.
		return fmt.Errorf("resource %s has no %s container ID recorded; cannot attempt bounded restart",
			resource.Name, tensorFoldRankLabel(workerID == ""))
	}
	return m.recordRestartState(resource, tensorFoldStatusNeedsRestart, func() error {
		if err := m.startRecipeContainer(ctx, resource, true, workerID); err != nil {
			return err
		}
		return m.startRecipeContainer(ctx, resource, false, headID)
	})
}

// recordRestartState applies one bounded-restart outcome: set the status and
// persist via compareAndWriteResource. When start is non-nil it increments
// RestartAttempts, stamps LastRestartAt, and invokes the start function.
// When start is nil (budget-exhausted path) it only sets the status.
func (m *tensorFoldManager) recordRestartState(resource tensorFoldResource, status string, start func() error) error {
	expected := resource
	expected.Status = status
	var startErr error
	if start != nil {
		expected.RestartAttempts++
		expected.LastRestartAt = time.Now().UTC()
		startErr = start()
	}
	if _, _, writeErr := m.compareAndWriteResource(expected, expected); writeErr != nil {
		return errors.Join(startErr, writeErr)
	}
	return startErr
}

// fenceAndMarkStopped releases a generation whose containers never appeared,
// but only after fencing any launcher that survived an agent restart and
// could still create them.
func (m *tensorFoldManager) fenceAndMarkStopped(expected tensorFoldResource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.readResource(expected.Name)
	if err != nil {
		return err
	}
	if !sameTensorFoldGeneration(expected, current) || !expected.UpdatedAt.Equal(current.UpdatedAt) {
		return nil
	}
	if launch := m.processes[current.Name]; launch != nil && launch.id == current.LaunchID {
		select {
		case <-launch.done:
		default:
			return nil
		}
	}
	if err := m.fenceLauncherLocked(current.LaunchID); err != nil {
		return err
	}
	current.Status = "stopped"
	return m.writeResource(current)
}

// tensorFoldPreparationDeadline exceeds the BKC's four-hour readiness
// timeout by a grace period, so the coordinator's own deadline governs
// whenever a coordinator is still present.
const tensorFoldPreparationDeadline = 14400*time.Second + 15*time.Minute

// A reconcile pass shells out to docker and SSH; bound it so a hung remote
// command cannot stall agent startup or supervision indefinitely.
const tensorFoldReconcileTimeout = 2 * time.Minute

func (m *tensorFoldManager) reconcileBounded(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, tensorFoldReconcileTimeout)
	defer cancel()
	return m.reconcile(ctx)
}

func (m *tensorFoldManager) supervise(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = m.reconcileBounded(ctx)
		}
	}
}

func (m *tensorFoldManager) remove(ctx context.Context, name string) error {
	resource, err := m.inspect(name)
	if err != nil {
		return err
	}
	if resource.Status != "stopped" {
		if err := m.stop(ctx, name); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.readResource(name)
	if err != nil {
		return err
	}
	if !sameTensorFoldGeneration(resource, current) || current.Status != "stopped" {
		return fmt.Errorf("TensorFold resource changed before removal; reservation retained")
	}
	if err := os.Remove(m.resourcePath(name)); err != nil {
		return err
	}
	// Rollback captures the bounded tail before removal; the append-only
	// supervisor log and launch transport would otherwise accumulate forever.
	if err := os.Remove(m.logPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("warning: remove TensorFold supervisor log %s: %v", name, err)
	}
	if tensorFoldLaunchIDPattern.MatchString(current.LaunchID) {
		if err := os.RemoveAll(m.tensorFoldSSHTransportDir(current.LaunchID)); err != nil {
			log.Printf("warning: remove TensorFold transport for %s: %v", name, err)
		}
	}
	return nil
}

func sameTensorFoldGeneration(left, right tensorFoldResource) bool {
	return left.Name == right.Name && left.DeploymentID == right.DeploymentID && left.Generation == right.Generation && left.Role == right.Role && left.LaunchID == right.LaunchID
}

func (m *tensorFoldManager) compareAndWriteResource(expected, updated tensorFoldResource) (tensorFoldResource, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.readResource(expected.Name)
	if err != nil {
		return tensorFoldResource{}, false, err
	}
	if !sameTensorFoldGeneration(expected, current) || !expected.UpdatedAt.Equal(current.UpdatedAt) {
		return current, false, nil
	}
	if err := m.writeResource(updated); err != nil {
		return tensorFoldResource{}, false, err
	}
	current, err = m.readResource(expected.Name)
	return current, true, err
}

func (m *tensorFoldManager) inspect(name string) (tensorFoldResource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.readResource(name)
}

func (m *tensorFoldManager) inventory(ctx context.Context) ([]Container, error) {
	entries, err := os.ReadDir(filepath.Join(m.root, "resources"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	containers := make([]Container, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		// Records are replaced by atomic rename, so inventory reads without the
		// manager lock and never waits behind a long create/stop/restart.
		resource, err := m.readResource(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		status := resource.Status
		if resource.Role == bkc.MultiDeviceRoleWorker && status == "running" {
			status = "reserved"
		}
		container := Container{
			ID: resource.ID, Name: resource.Name, Image: bkc.GLM53FlashEXL3TensorFoldImage, Status: status,
			Created: resource.LaunchStartedAt, Ownership: OwnershipManaged,
			Labels: map[string]string{
				LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelDeploymentID: resource.DeploymentID,
				LabelGeneration: strconv.Itoa(resource.Generation), LabelRole: resource.Role,
				LabelBKCID: bkc.GLM53FlashEXL3TensorFoldDualGB10ID, LabelModelRevision: bkc.GLM53FlashEXL3TensorFoldModelRevision,
				LabelImageDigest: bkc.GLM53FlashEXL3TensorFoldImageDigest,
			},
		}
		if !resource.LaunchStartedAt.IsZero() {
			container.Uptime = int64(time.Since(resource.LaunchStartedAt).Seconds())
		}
		if resource.Role == bkc.MultiDeviceRoleHead {
			container.Ports = map[string]string{strconv.Itoa(resource.ServicePort): strconv.Itoa(resource.ServicePort)}
			container.Labels[LabelServiceAddress] = resource.ServiceAddress
			container.Labels[LabelServicePort] = strconv.Itoa(resource.ServicePort)
			if resource.Status == "running" {
				metrics, scrapeErr := scrapeVLLMMetricsURLWithClient(m.metricsHTTPClient, m.serviceBaseURL(resource)+"/metrics", "")
				if scrapeErr == nil {
					metrics.Model = bkc.GLM53FlashEXL3TensorFoldServedModel
					container.VLLMMetrics = metrics
				}
			}
		}
		containers = append(containers, container)
	}
	return containers, nil
}

// serviceInventory is inventory without worker reservations. A reservation
// holds the node for the head's recipe and serves nothing, so it must not be
// reported as an inference service that is down.
func (m *tensorFoldManager) serviceInventory(ctx context.Context) ([]Container, error) {
	containers, err := m.inventory(ctx)
	if err != nil {
		return nil, err
	}
	services := containers[:0]
	for _, container := range containers {
		if container.Labels[LabelRole] != bkc.MultiDeviceRoleWorker {
			services = append(services, container)
		}
	}
	return services, nil
}

func (m *tensorFoldManager) observe(ctx context.Context, name string) (tensorFoldResource, error) {
	resource, err := m.inspect(name)
	if err != nil || resource.Role != bkc.MultiDeviceRoleHead || (resource.Status != "starting" && resource.Status != "running") {
		return resource, err
	}
	localExists, localRunning, localID, err := m.inspectRecipeContainerIdentity(ctx, resource, false)
	if err != nil {
		return tensorFoldResource{}, err
	}
	workerExists, workerRunning, workerID, err := m.inspectRecipeContainerIdentity(ctx, resource, true)
	if err != nil {
		return tensorFoldResource{}, err
	}
	expected := resource
	if localExists && workerExists && localRunning && workerRunning {
		resource.Status = "running"
	} else if resource.Status == "running" && (!localExists || !workerExists || !localRunning || !workerRunning) {
		resource.Status = "failed"
	}
	if localExists {
		resource.HeadContainerID = localID
	}
	if workerExists {
		resource.WorkerContainerID = workerID
	}
	current, _, err := m.compareAndWriteResource(expected, resource)
	if err != nil {
		return tensorFoldResource{}, err
	}
	return current, nil
}

func (m *tensorFoldManager) waitForProcess(name string, launch *tensorFoldLaunch, logFile *os.File) {
	err := launch.process.Wait()
	launch.waitErr = err
	close(launch.done)
	_ = logFile.Close()
	m.mu.Lock()
	if m.processes[name] != launch {
		m.mu.Unlock()
		return
	}
	delete(m.processes, name)
	resource, readErr := m.readResource(name)
	if readErr != nil || resource.LaunchID != launch.id || resource.Status == "stopping" || resource.Status == "stopped" {
		m.mu.Unlock()
		return
	}
	resource.Status = "failed"
	if err == nil {
		resource.Status = "exited"
	}
	_ = m.writeResource(resource)
	m.mu.Unlock()
	if m.beforeWaiterCleanup != nil {
		m.beforeWaiterCleanup()
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_ = m.stopGeneration(cleanupCtx, resource)
}

func (m *tensorFoldManager) effectivePinnedFiles(commit string) map[string]string {
	if m.pinnedFilesOverride != nil {
		return m.pinnedFilesOverride
	}
	pinned, ok := tensorFoldPinnedRecipeFilesByCommit[commit]
	if !ok {
		return nil
	}
	return pinned
}

func (m *tensorFoldManager) ensurePinnedRecipe(ctx context.Context, commit string) error {
	commit = strings.TrimSpace(commit)
	if _, ok := tensorFoldPinnedRecipeFilesByCommit[commit]; !ok {
		return fmt.Errorf("TensorFold recipe commit %s is not pinned", commit)
	}
	recipeDir := m.recipePath(commit)
	if err := ensureOwnedTensorFoldDirectory(m.root); err != nil {
		return err
	}
	if err := ensureOwnedTensorFoldDirectory(filepath.Dir(recipeDir)); err != nil {
		return err
	}
	if _, err := os.Lstat(recipeDir); errors.Is(err, os.ErrNotExist) {
		if err := m.stagePinnedRecipe(ctx, commit); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if info, err := os.Lstat(recipeDir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("TensorFold recipe directory is missing or unsafe")
	} else if !tensorFoldFileOwnedByCurrentUser(info) {
		return fmt.Errorf("TensorFold recipe directory is not owned by the agent user")
	}
	revision, err := m.runner.Run(ctx, recipeDir, m.commandEnvironment(false), "git", "-C", recipeDir, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(string(revision)) != commit {
		return fmt.Errorf("TensorFold recipe checkout is not the pinned commit")
	}
	status, err := m.runner.Run(ctx, recipeDir, m.commandEnvironment(false), "git", "-C", recipeDir, "status", "--porcelain", "--untracked-files=all", "--ignored=matching")
	if err != nil {
		return fmt.Errorf("inspect TensorFold recipe checkout: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(status)), "\n") {
		if line != "" && line != "?? .env" && line != "!! .env" {
			return fmt.Errorf("TensorFold recipe checkout contains unowned changes")
		}
	}
	return m.verifyPinnedRecipeFiles(commit)
}

// stagePinnedRecipe fetches into a sibling staging directory and renames it
// into place only after the checkout completes, so an interrupted fetch never
// leaves a partial recipe that every later create would reject.
func (m *tensorFoldManager) stagePinnedRecipe(ctx context.Context, commit string) error {
	staging, err := os.MkdirTemp(filepath.Dir(m.recipePath(commit)), ".staging-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := validateOwnedTensorFoldDirectory(staging); err != nil {
		return err
	}
	for _, command := range [][]string{
		{"git", "init", "--quiet"},
		{"git", "remote", "add", "origin", bkc.GLM53FlashEXL3TensorFoldRecipeRepository},
		{"git", "fetch", "--quiet", "--depth", "1", "origin", commit},
		{"git", "checkout", "--quiet", "--detach", "FETCH_HEAD"},
	} {
		if _, err := m.runner.Run(ctx, staging, m.commandEnvironment(false), command[0], command[1:]...); err != nil {
			return fmt.Errorf("stage pinned TensorFold recipe: %w", err)
		}
	}
	if err := m.normalizeFreshPinnedRecipePermissions(staging, commit); err != nil {
		return err
	}
	return os.Rename(staging, m.recipePath(commit))
}

func (m *tensorFoldManager) normalizeFreshPinnedRecipePermissions(recipeDir string, commit string) error {
	for relative := range m.effectivePinnedFiles(commit) {
		path := filepath.Join(recipeDir, filepath.FromSlash(relative))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("pinned TensorFold file %s is missing or unsafe", relative)
		}
		if !tensorFoldFileOwnedByCurrentUser(info) {
			return fmt.Errorf("pinned TensorFold file %s is not owned by the agent user", relative)
		}
		if err := os.Chmod(path, info.Mode().Perm()&^0o022); err != nil {
			return fmt.Errorf("secure pinned TensorFold file %s: %w", relative, err)
		}
	}
	return nil
}

func (m *tensorFoldManager) verifyPinnedRecipeFiles(commit string) error {
	if _, ok := tensorFoldPinnedRecipeFilesByCommit[commit]; !ok {
		return fmt.Errorf("TensorFold recipe commit %s is not pinned", commit)
	}
	recipeDir := m.recipePath(commit)
	for _, directory := range []string{m.root, filepath.Dir(recipeDir), recipeDir, filepath.Join(recipeDir, "scripts")} {
		if err := validateOwnedTensorFoldDirectory(directory); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(filepath.Join(recipeDir, "scripts", "local.sh")); err == nil {
		return fmt.Errorf("TensorFold scripts/local.sh is forbidden because it can override the owned environment")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for relative, expected := range m.effectivePinnedFiles(commit) {
		path := filepath.Join(recipeDir, filepath.FromSlash(relative))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("pinned TensorFold file %s is missing or unsafe", relative)
		}
		if !tensorFoldFileOwnedByCurrentUser(info) {
			return fmt.Errorf("pinned TensorFold file %s is not owned by the agent user", relative)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("pinned TensorFold file %s is group/world writable", relative)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != expected {
			return fmt.Errorf("pinned TensorFold file %s failed integrity verification", relative)
		}
	}
	return nil
}

func (m *tensorFoldManager) verifyPinnedRecipeForExecution(resource tensorFoldResource) error {
	if err := m.verifyPinnedRecipeFiles(resource.Commit); err != nil {
		return err
	}
	path := filepath.Join(m.recipePath(resource.Commit), ".env")
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !tensorFoldFileOwnedByCurrentUser(info) || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("TensorFold recipe environment is unsafe")
	}
	request := tensorFoldResourceRequest{
		Name: resource.Name, DeploymentID: resource.DeploymentID, Generation: resource.Generation, Role: resource.Role,
		WorkerUser: resource.WorkerUser, WorkerAddress: resource.WorkerAddress, HeadFabricAddress: resource.HeadFabricAddress,
		ServiceAddress: resource.ServiceAddress, ServicePort: resource.ServicePort, Repository: resource.Repository, Commit: resource.Commit,
	}
	expected, err := renderTensorFoldRecipeEnvironment(request, resource.LaunchID)
	if err != nil {
		return err
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(actual) != expected {
		return fmt.Errorf("TensorFold recipe environment does not match its durable resource")
	}
	return m.verifyTensorFoldSSHTransport(resource)
}

func (m *tensorFoldManager) openLog(name string) (*os.File, error) {
	if err := ensureOwnedTensorFoldDirectory(m.root); err != nil {
		return nil, err
	}
	if err := ensureOwnedTensorFoldDirectory(filepath.Dir(m.logPath(name))); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(m.logPath(name)); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !tensorFoldFileOwnedByCurrentUser(info) {
			return nil, fmt.Errorf("TensorFold supervisor log path is unsafe")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return os.OpenFile(m.logPath(name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

func (m *tensorFoldManager) writeRecipeEnvironment(environment string, commit string) error {
	directory := m.recipePath(commit)
	if err := validateOwnedTensorFoldDirectory(directory); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".yokai-env-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(environment); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, filepath.Join(directory, ".env"))
}

func (m *tensorFoldManager) tensorFoldSSHTransportDir(launchID string) string {
	return filepath.Join(m.root, "transports", launchID)
}

// tensorFoldLaunchMarker is the launch-unique environment entry every recipe
// process inherits; fencing matches it exactly.
func (m *tensorFoldManager) tensorFoldLaunchMarker(launchID string) string {
	return "RSYNC_RSH=" + filepath.Join(m.tensorFoldSSHTransportDir(launchID), "ssh")
}

func (m *tensorFoldManager) tensorFoldSSHTransportContents(resource tensorFoldResource) (string, string, error) {
	if !tensorFoldLaunchIDPattern.MatchString(resource.LaunchID) || m.sshExecutable == "" || !filepath.IsAbs(m.sshExecutable) {
		return "", "", fmt.Errorf("TensorFold SSH transport has no valid launch identity or SSH executable")
	}
	config := strings.Join([]string{
		"Host *",
		"  BatchMode yes",
		"  StrictHostKeyChecking yes",
		"  CheckHostIP yes",
		"  UserKnownHostsFile ~/.ssh/known_hosts ~/.ssh/known_hosts2",
		"  GlobalKnownHostsFile /etc/ssh/ssh_known_hosts /etc/ssh/ssh_known_hosts2",
		"  ConnectTimeout 10",
		"  ServerAliveInterval 15",
		"",
	}, "\n")
	configPath := filepath.Join(m.tensorFoldSSHTransportDir(resource.LaunchID), "config")
	wrapper := "#!/bin/sh\nexec " + quoteTensorFoldShellWord(m.sshExecutable) +
		" -F " + quoteTensorFoldShellWord(configPath) +
		" -o BatchMode=yes -o StrictHostKeyChecking=yes -o CheckHostIP=yes \"$@\"\n"
	return config, wrapper, nil
}

func (m *tensorFoldManager) ensureTensorFoldSSHTransport(resource tensorFoldResource) error {
	config, wrapper, err := m.tensorFoldSSHTransportContents(resource)
	if err != nil {
		return err
	}
	directory := m.tensorFoldSSHTransportDir(resource.LaunchID)
	if err := ensureOwnedTensorFoldDirectory(filepath.Dir(directory)); err != nil {
		return err
	}
	if err := ensureOwnedTensorFoldDirectory(directory); err != nil {
		return err
	}
	if err := writeOwnedTensorFoldFile(filepath.Join(directory, "config"), []byte(config), 0o600); err != nil {
		return err
	}
	return writeOwnedTensorFoldFile(filepath.Join(directory, "ssh"), []byte(wrapper), 0o700)
}

func (m *tensorFoldManager) verifyTensorFoldSSHTransport(resource tensorFoldResource) error {
	config, wrapper, err := m.tensorFoldSSHTransportContents(resource)
	if err != nil {
		return err
	}
	directory := m.tensorFoldSSHTransportDir(resource.LaunchID)
	if err := validateOwnedTensorFoldDirectory(directory); err != nil {
		return fmt.Errorf("TensorFold SSH transport directory is unsafe: %w", err)
	}
	for _, file := range []struct {
		name string
		mode os.FileMode
		want string
	}{{"config", 0o600, config}, {"ssh", 0o700, wrapper}} {
		path := filepath.Join(directory, file.name)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !tensorFoldFileOwnedByCurrentUser(info) || info.Mode().Perm() != file.mode {
			return fmt.Errorf("TensorFold SSH transport %s is unsafe", file.name)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != file.want {
			return fmt.Errorf("TensorFold SSH transport %s does not match its launch", file.name)
		}
	}
	return nil
}

func writeOwnedTensorFoldFile(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := validateOwnedTensorFoldDirectory(directory); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".tensorfold-transport-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func (m *tensorFoldManager) commandEnvironment(foreground bool, launchIDs ...string) []string {
	home, _ := os.UserHomeDir()
	path := "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	environment := []string{
		"HOME=" + home,
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
	}
	if len(launchIDs) == 1 && tensorFoldLaunchIDPattern.MatchString(launchIDs[0]) {
		path = m.tensorFoldSSHTransportDir(launchIDs[0]) + ":" + path
		environment = append(environment, m.tensorFoldLaunchMarker(launchIDs[0]))
	}
	environment = append(environment, "PATH="+path)
	if foreground {
		environment = append(environment, "FOREGROUND=1")
	}
	return environment
}

func (m *tensorFoldManager) readResource(name string) (tensorFoldResource, error) {
	if !tensorFoldResourceNamePattern.MatchString(name) {
		return tensorFoldResource{}, fmt.Errorf("invalid TensorFold resource name")
	}
	path := m.resourcePath(name)
	if err := validateOwnedTensorFoldDirectory(filepath.Dir(path)); err != nil {
		return tensorFoldResource{}, err
	}
	if err := rejectTensorFoldSymlinkComponents(path, false); err != nil {
		return tensorFoldResource{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return tensorFoldResource{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !tensorFoldFileOwnedByCurrentUser(info) || info.Mode().Perm()&0o077 != 0 {
		return tensorFoldResource{}, fmt.Errorf("TensorFold resource record is unsafe")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return tensorFoldResource{}, err
	}
	var resource tensorFoldResource
	if err := json.Unmarshal(data, &resource); err != nil {
		return tensorFoldResource{}, err
	}
	if err := validateStoredTensorFoldResource(resource, name); err != nil {
		return tensorFoldResource{}, err
	}
	return resource, nil
}

func (m *tensorFoldManager) writeResource(resource tensorFoldResource) error {
	if err := validateStoredTensorFoldResource(resource, resource.Name); err != nil {
		return err
	}
	resource.UpdatedAt = time.Now().UTC()
	directory := filepath.Dir(m.resourcePath(resource.Name))
	if err := ensureOwnedTensorFoldDirectory(m.root); err != nil {
		return err
	}
	if err := ensureOwnedTensorFoldDirectory(directory); err != nil {
		return err
	}
	data, err := json.MarshalIndent(resource, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".tensorfold-resource-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, m.resourcePath(resource.Name))
}

func ensureOwnedTensorFoldDirectory(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("TensorFold state path must be absolute")
	}
	if err := rejectTensorFoldSymlinkComponents(path, true); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := rejectTensorFoldSymlinkComponents(path, false); err != nil {
		return err
	}
	return validateOwnedTensorFoldDirectory(path)
}

func validateOwnedTensorFoldDirectory(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("TensorFold state path must be absolute")
	}
	if err := rejectTensorFoldSymlinkComponents(path, false); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !tensorFoldFileOwnedByCurrentUser(info) {
		return fmt.Errorf("TensorFold state directory is unsafe")
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("secure TensorFold state directory: %w", err)
		}
	}
	return nil
}

func rejectTensorFoldSymlinkComponents(path string, allowMissing bool) error {
	clean := filepath.Clean(path)
	current := string(filepath.Separator)
	for _, component := range strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) && allowMissing {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("TensorFold path contains symlink component %s", current)
		}
	}
	return nil
}

func (m *tensorFoldManager) ensureSingletonReservation(requestedName string) error {
	directory := filepath.Join(m.root, "resources")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		if name != requestedName {
			return fmt.Errorf("TensorFold node is reserved by resource %s", name)
		}
	}
	return nil
}

func validateStoredTensorFoldResource(resource tensorFoldResource, requestedName string) error {
	request := tensorFoldResourceRequest{
		Name: resource.Name, DeploymentID: resource.DeploymentID, Generation: resource.Generation, Role: resource.Role,
		WorkerUser: resource.WorkerUser, WorkerAddress: resource.WorkerAddress, HeadFabricAddress: resource.HeadFabricAddress,
		ServiceAddress: resource.ServiceAddress, ServicePort: resource.ServicePort, Repository: resource.Repository, Commit: resource.Commit,
	}
	if requestedName != resource.Name || resource.ID != tensorFoldResourceID(resource.Name) || !resource.Managed || resource.Ownership != OwnershipManaged {
		return fmt.Errorf("TensorFold resource identity does not match its durable record")
	}
	if err := validateTensorFoldResourceRequest(request, resource.Role == bkc.MultiDeviceRoleHead); err != nil {
		return err
	}
	if resource.Role == bkc.MultiDeviceRoleHead && (resource.LaunchStartedAt.IsZero() || !tensorFoldLaunchIDPattern.MatchString(resource.LaunchID)) {
		return fmt.Errorf("TensorFold head resource has no valid launch-generation identity")
	}
	switch resource.Status {
	case "planned", "starting", "running", "stopping", "stopped", "failed", "exited", tensorFoldStatusNeedsRestart:
	default:
		return fmt.Errorf("TensorFold resource has invalid status")
	}
	return nil
}

var (
	tensorFoldResourceNamePattern = regexp.MustCompile(`^yokai-deployment-[a-zA-Z0-9._-]+-g[1-9][0-9]*-(head|worker)$`)
	tensorFoldDeploymentIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	tensorFoldSSHUserPattern      = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	tensorFoldLaunchIDPattern     = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// tensorFoldPinnedImageForCommit returns the image and patch-hash label for a
// recipe commit, failing closed on unknown commits.
func tensorFoldPinnedImageForCommit(commit string) (image, patchHash string, ok bool) {
	switch commit {
	case bkc.GLM53FlashEXL3TensorFoldRecipeCommit:
		return bkc.GLM53FlashEXL3TensorFoldImage, bkc.GLM53FlashEXL3TensorFoldImagePatchHash, true
	case bkc.GLM53FlashEXL3TensorFoldRecipeCommitV14:
		return bkc.GLM53FlashEXL3TensorFoldImageV14, bkc.GLM53FlashEXL3TensorFoldImagePatchHashV14, true
	}
	return "", "", false
}

// tensorFoldWatchdogPolicy returns the per-recipe-commit watchdog environment
// values from the BKC pin. A zero-value policy omits both TF_GLM_MULTI_WATCHDOG_*
// keys entirely; ok is false for commits the BKC does not pin (fail closed).
type tensorFoldWatchdog struct {
	exit    string
	seconds string
}

func tensorFoldWatchdogPolicy(commit string) (tensorFoldWatchdog, bool) {
	exit, seconds, ok := bkc.TensorFoldWatchdogForCommit(commit)
	return tensorFoldWatchdog{exit: exit, seconds: seconds}, ok
}

func renderTensorFoldRecipeEnvironment(request tensorFoldResourceRequest, launchIDs ...string) (string, error) {
	if err := validateTensorFoldResourceRequest(request, true); err != nil {
		return "", err
	}
	commit := request.Commit
	values := map[string]string{
		"WORKER":                     request.WorkerUser + "@" + request.WorkerAddress,
		"FABRIC_PEER":                request.WorkerAddress,
		"MASTER_ADDR":                request.HeadFabricAddress,
		"MASTER_PORT":                strconv.Itoa(bkc.GLM53FlashEXL3TensorFoldRendezvousPort),
		"TP":                         "2",
		"HOST":                       request.ServiceAddress,
		"PORT":                       strconv.Itoa(bkc.GLM53FlashEXL3TensorFoldServicePort),
		"NCCL_RAILS":                 "1",
		"MODEL_ID":                   bkc.GLM53FlashEXL3TensorFoldModel,
		"MODEL_REVISION":             bkc.GLM53FlashEXL3TensorFoldModelRevision,
		"DFLASH2_ID":                 bkc.GLM53FlashEXL3TensorFoldDrafter,
		"DFLASH2_REVISION":           bkc.GLM53FlashEXL3TensorFoldDrafterRevision,
		"CONTEXT":                    "1048576",
		"PARALLEL":                   "4",
		"KV":                         "fp8",
		"DRAFTER":                    "dflash2",
		"DENSE":                      "q4",
		"VISION":                     "1",
		"VISION_URLS":                "0",
		"MEMORY_RESERVE_GIB":         "14.5",
		"KV_POOL_GIB":                "12.5",
		"HF_HUB_OFFLINE":             "1",
		"TENSORFOLD_NO_UPDATE_CHECK": "1",
	}
	wd, ok := tensorFoldWatchdogPolicy(commit)
	if !ok {
		return "", fmt.Errorf("TensorFold recipe commit %s is not pinned", commit)
	}
	if wd.exit != "" {
		values["TF_GLM_MULTI_WATCHDOG_EXIT"] = wd.exit
	}
	if wd.seconds != "" {
		values["TF_GLM_MULTI_WATCHDOG_S"] = wd.seconds
	}
	image, _, ok := tensorFoldPinnedImageForCommit(commit)
	if !ok {
		return "", fmt.Errorf("TensorFold recipe commit %s is not pinned", commit)
	}
	values["IMAGE"] = image
	values["PULL"] = "1"
	if len(launchIDs) > 0 {
		if len(launchIDs) != 1 || !tensorFoldLaunchIDPattern.MatchString(launchIDs[0]) {
			return "", fmt.Errorf("invalid TensorFold launch identity")
		}
		values["TENSORFOLD_YOKAI_LAUNCH_ID"] = launchIDs[0]
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var output strings.Builder
	for _, key := range keys {
		output.WriteString(key)
		output.WriteByte('=')
		output.WriteString(values[key])
		output.WriteByte('\n')
	}
	return output.String(), nil
}

func validateTensorFoldResourceRequest(request tensorFoldResourceRequest, head bool) error {
	if !tensorFoldResourceNamePattern.MatchString(request.Name) || !tensorFoldDeploymentIDPattern.MatchString(request.DeploymentID) || request.Generation < 1 {
		return fmt.Errorf("invalid TensorFold deployment identity")
	}
	if request.Repository != bkc.GLM53FlashEXL3TensorFoldRecipeRepository {
		return fmt.Errorf("TensorFold recipe repository does not match the pinned BKC")
	}
	if _, ok := tensorFoldPinnedRecipeFilesByCommit[request.Commit]; !ok {
		return fmt.Errorf("TensorFold recipe commit %s is not pinned", request.Commit)
	}
	wantRole := bkc.MultiDeviceRoleWorker
	if head {
		wantRole = bkc.MultiDeviceRoleHead
	}
	if request.Role != wantRole {
		return fmt.Errorf("TensorFold resource role must be %s", wantRole)
	}
	if request.Name != fmt.Sprintf("yokai-deployment-%s-g%d-%s", request.DeploymentID, request.Generation, request.Role) {
		return fmt.Errorf("TensorFold resource name does not match deployment, generation, and role")
	}
	if !head {
		if request.WorkerUser != "" || request.WorkerAddress != "" || request.HeadFabricAddress != "" || request.ServiceAddress != "" || request.ServicePort != 0 {
			return fmt.Errorf("worker reservation must not carry head orchestration fields")
		}
		return nil
	}
	if !tensorFoldSSHUserPattern.MatchString(request.WorkerUser) {
		return fmt.Errorf("invalid worker SSH user")
	}
	worker := net.ParseIP(request.WorkerAddress)
	headFabric := net.ParseIP(request.HeadFabricAddress)
	service := net.ParseIP(request.ServiceAddress)
	// The recipe renders unbracketed user@host SSH/rsync targets and pins an
	// IPv4 RoCE v2 GID, so every topology address must be plain IPv4; Is4 also
	// rejects IPv4-mapped IPv6 spellings.
	for _, address := range []string{request.WorkerAddress, request.HeadFabricAddress, request.ServiceAddress} {
		parsed, err := netip.ParseAddr(address)
		if err != nil || !parsed.Is4() || !validPrivateDeploymentIP(net.ParseIP(address)) {
			return fmt.Errorf("worker, head fabric, and service addresses must be explicit private IPv4 addresses")
		}
	}
	if worker.Equal(headFabric) || service.Equal(worker) || service.Equal(headFabric) {
		return fmt.Errorf("TensorFold fabric and service addresses must be distinct")
	}
	if request.ServicePort != bkc.GLM53FlashEXL3TensorFoldServicePort {
		return fmt.Errorf("TensorFold service port must be %d", bkc.GLM53FlashEXL3TensorFoldServicePort)
	}
	return nil
}

func validPrivateDeploymentIP(ip net.IP) bool {
	return ip != nil && ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsLoopback() && !ip.IsMulticast()
}
