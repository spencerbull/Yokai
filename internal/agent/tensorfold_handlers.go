package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/deployments"
)

func tensorFoldStateRoot() (string, error) {
	stateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME"))
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	if !filepath.IsAbs(stateHome) {
		return "", fmt.Errorf("XDG_STATE_HOME must be absolute")
	}
	return filepath.Join(stateHome, "yokai", "tensorfold"), nil
}

func registerTensorFoldRoutes(mux *http.ServeMux, manager *tensorFoldManager) {
	mux.HandleFunc("POST /deployments/tensorfold/preflight", requireAuth(manager.handlePreflight))
	mux.HandleFunc("POST /deployments/tensorfold/resources", requireAuth(manager.handleCreateResource))
	mux.HandleFunc("GET /deployments/tensorfold/resources/{name}", requireAuth(manager.handleInspectResource))
	mux.HandleFunc("POST /deployments/tensorfold/resources/{name}/stop", requireAuth(manager.handleStopResource))
	mux.HandleFunc("POST /deployments/tensorfold/resources/{name}/restart", requireAuth(manager.handleRestartResource))
	mux.HandleFunc("POST /deployments/tensorfold/resources/{name}/logs/tail", requireAuth(manager.handleResourceLogs))
	mux.HandleFunc("POST /deployments/tensorfold/resources/{name}/test", requireAuth(manager.handleTestResource))
	mux.HandleFunc("DELETE /deployments/tensorfold/resources/{name}", requireAuth(manager.handleRemoveResource))
}

func (m *tensorFoldManager) handlePreflight(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeTensorFoldResourceRequest(w, r)
	if !ok {
		return
	}
	if err := m.preflight(r.Context(), request); err != nil {
		writeError(w, http.StatusConflict, "tensorfold_preflight_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (m *tensorFoldManager) handleCreateResource(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeTensorFoldResourceRequest(w, r)
	if !ok {
		return
	}
	resource, err := m.create(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusConflict, "tensorfold_create_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, resource)
}

func decodeTensorFoldResourceRequest(w http.ResponseWriter, r *http.Request) (tensorFoldResourceRequest, bool) {
	var request tensorFoldResourceRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_tensorfold_request", err.Error())
		return tensorFoldResourceRequest{}, false
	}
	return request, true
}

func (m *tensorFoldManager) authorizedResource(w http.ResponseWriter, r *http.Request) (tensorFoldResource, bool) {
	resource, err := m.observe(r.Context(), r.PathValue("name"))
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		writeError(w, status, "tensorfold_resource_invalid", err.Error())
		return tensorFoldResource{}, false
	}
	generation, err := strconv.Atoi(r.URL.Query().Get("generation"))
	if err != nil || r.URL.Query().Get("deployment_id") != resource.DeploymentID || generation != resource.Generation || r.URL.Query().Get("role") != resource.Role {
		writeError(w, http.StatusConflict, "tensorfold_resource_mismatch", "resource deployment, generation, and role must match the durable record")
		return tensorFoldResource{}, false
	}
	return resource, true
}

func (m *tensorFoldManager) handleInspectResource(w http.ResponseWriter, r *http.Request) {
	resource, ok := m.authorizedResource(w, r)
	if ok {
		writeJSON(w, http.StatusOK, resource)
	}
}

func (m *tensorFoldManager) handleStopResource(w http.ResponseWriter, r *http.Request) {
	resource, ok := m.authorizedResource(w, r)
	if !ok {
		return
	}
	if err := m.stop(r.Context(), resource.Name); err != nil {
		writeError(w, http.StatusConflict, "tensorfold_stop_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (m *tensorFoldManager) handleRestartResource(w http.ResponseWriter, r *http.Request) {
	resource, ok := m.authorizedResource(w, r)
	if !ok {
		return
	}
	if err := m.restart(r.Context(), resource.Name); err != nil {
		writeError(w, http.StatusConflict, "tensorfold_restart_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "starting"})
}

func (m *tensorFoldManager) handleRemoveResource(w http.ResponseWriter, r *http.Request) {
	resource, ok := m.authorizedResource(w, r)
	if !ok {
		return
	}
	if err := m.remove(r.Context(), resource.Name); err != nil {
		writeError(w, http.StatusConflict, "tensorfold_remove_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (m *tensorFoldManager) handleResourceLogs(w http.ResponseWriter, r *http.Request) {
	resource, ok := m.authorizedResource(w, r)
	if !ok {
		return
	}
	tail, truncated, err := m.logs(r.Context(), resource)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tensorfold_logs_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tail": tail, "truncated": truncated})
}

func (m *tensorFoldManager) handleTestResource(w http.ResponseWriter, r *http.Request) {
	resource, ok := m.authorizedResource(w, r)
	if !ok {
		return
	}
	result, err := m.testResource(r.Context(), resource)
	if err != nil {
		writeError(w, http.StatusBadGateway, "tensorfold_readiness_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (m *tensorFoldManager) preflight(ctx context.Context, request tensorFoldResourceRequest) error {
	head := request.Role == "head"
	if err := validateTensorFoldResourceRequest(request, head); err != nil {
		return err
	}
	m.mu.Lock()
	reservationErr := m.ensureSingletonReservation(request.Name)
	m.mu.Unlock()
	if reservationErr != nil {
		return reservationErr
	}
	if _, err := os.Lstat(m.resourcePath(request.Name)); err == nil {
		return fmt.Errorf("TensorFold resource already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := m.runner.Run(ctx, "", m.commandEnvironment(false), "docker", "info", "--format", "{{.ServerVersion}}"); err != nil {
		return fmt.Errorf("local Docker is unavailable: %w", err)
	}
	if exists, err := m.recipeContainerNameExists(ctx, tensorFoldResourceFromRequest(request), false); err != nil {
		return fmt.Errorf("inspect local TensorFold container name: %w", err)
	} else if exists {
		return fmt.Errorf("container %s already exists and will not be adopted", tensorFoldContainerName)
	}
	resource := tensorFoldResourceFromRequest(request)
	if err := m.verifyTensorFoldImageProvenance(ctx, resource, false); err != nil {
		return err
	}
	if !head {
		return nil
	}
	for _, tool := range []string{"git", "rsync"} {
		if _, err := m.runner.Run(ctx, "", m.commandEnvironment(false), tool, "--version"); err != nil {
			return fmt.Errorf("required TensorFold tool %s is unavailable: %w", tool, err)
		}
	}
	if _, err := m.runTensorFoldNodeCommand(ctx, resource, true, "true"); err != nil {
		return fmt.Errorf("strict passwordless worker SSH is unavailable: %w", err)
	}
	if exists, err := m.recipeContainerNameExists(ctx, resource, true); err != nil {
		return fmt.Errorf("inspect worker TensorFold container name: %w", err)
	} else if exists {
		return fmt.Errorf("worker container %s already exists and will not be adopted", tensorFoldContainerName)
	}
	if err := m.verifyTensorFoldImageProvenance(ctx, resource, true); err != nil {
		return err
	}
	if err := m.verifyTensorFoldFabric(ctx, resource); err != nil {
		return err
	}
	// Stage and verify the exact checkout here, before the engine stops any
	// previous service, so an unreachable or drifted recipe is a preflight
	// rejection rather than a cutover followed by rollback.
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensurePinnedRecipe(ctx)
}

func (m *tensorFoldManager) verifyTensorFoldImageProvenance(ctx context.Context, resource tensorFoldResource, worker bool) error {
	output, err := m.runTensorFoldNodeCommand(ctx, resource, worker, "docker", "image", "inspect", "--format", `{{ index .Config.Labels "tf.patches" }}`, bkc.GLM53FlashEXL3TensorFoldImage)
	node := "head"
	if worker {
		node = "worker"
	}
	if err != nil {
		return fmt.Errorf("pinned TensorFold image is not staged on %s: %w", node, err)
	}
	if strings.TrimSpace(string(output)) != bkc.GLM53FlashEXL3TensorFoldImagePatchHash {
		return fmt.Errorf("pinned TensorFold image patch provenance does not match on %s", node)
	}
	return nil
}

func tensorFoldResourceFromRequest(request tensorFoldResourceRequest) tensorFoldResource {
	return tensorFoldResource{
		ID: tensorFoldResourceID(request.Name), Name: request.Name, DeploymentID: request.DeploymentID,
		Generation: request.Generation, Role: request.Role, Status: "planned", Ownership: OwnershipManaged, Managed: true,
		Repository: request.Repository, Commit: request.Commit, WorkerUser: request.WorkerUser,
		WorkerAddress: request.WorkerAddress, HeadFabricAddress: request.HeadFabricAddress,
		ServiceAddress: request.ServiceAddress, ServicePort: request.ServicePort,
	}
}

func (m *tensorFoldManager) recipeContainerNameExists(ctx context.Context, resource tensorFoldResource, worker bool) (bool, error) {
	output, err := m.runTensorFoldNodeCommand(ctx, resource, worker, "docker", "ps", "-a", "--no-trunc", "--filter", "name=^/"+tensorFoldContainerName+"$", "--format", "{{.ID}}")
	if err != nil {
		return false, err
	}
	identifiers := strings.Fields(string(output))
	if len(identifiers) > 1 {
		return false, fmt.Errorf("container name is ambiguous")
	}
	return len(identifiers) == 1, nil
}

func (m *tensorFoldManager) verifyTensorFoldFabric(ctx context.Context, resource tensorFoldResource) error {
	type route struct {
		Device    string `json:"dev"`
		Source    string `json:"src"`
		Preferred string `json:"prefsrc"`
	}
	checkRoute := func(output []byte, source string) error {
		var routes []route
		if json.Unmarshal(output, &routes) != nil || len(routes) == 0 {
			return fmt.Errorf("fabric route inventory is invalid")
		}
		actualSource := routes[0].Source
		if actualSource == "" {
			actualSource = routes[0].Preferred
		}
		if routes[0].Device != "enP2p1s0f0np0" || actualSource != source {
			return fmt.Errorf("fabric route must use enP2p1s0f0np0 with source %s", source)
		}
		return nil
	}
	local, err := m.runner.Run(ctx, "", m.commandEnvironment(false), "ip", "-j", "route", "get", resource.WorkerAddress)
	if err != nil || checkRoute(local, resource.HeadFabricAddress) != nil {
		return fmt.Errorf("head fabric route does not match the pinned topology")
	}
	remote, err := m.runTensorFoldNodeCommand(ctx, resource, true, "ip", "-j", "route", "get", resource.HeadFabricAddress)
	if err != nil || checkRoute(remote, resource.WorkerAddress) != nil {
		return fmt.Errorf("worker fabric route does not match the pinned topology")
	}
	const typePath = "/sys/class/infiniband/roceP2p1s0f0/ports/1/gid_attrs/types/3"
	const devicePath = "/sys/class/infiniband/roceP2p1s0f0/ports/1/gid_attrs/ndevs/3"
	const gidPath = "/sys/class/infiniband/roceP2p1s0f0/ports/1/gids/3"
	for _, worker := range []bool{false, true} {
		address := resource.HeadFabricAddress
		if worker {
			address = resource.WorkerAddress
		}
		typeValue, typeErr := m.runTensorFoldNodeCommand(ctx, resource, worker, "cat", typePath)
		deviceValue, deviceErr := m.runTensorFoldNodeCommand(ctx, resource, worker, "cat", devicePath)
		gidValue, gidErr := m.runTensorFoldNodeCommand(ctx, resource, worker, "cat", gidPath)
		if typeErr != nil || deviceErr != nil || gidErr != nil || strings.TrimSpace(string(typeValue)) != "RoCE v2" ||
			validateFabricGIDValue("enP2p1s0f0np0", address, gidValue, deviceValue) != nil {
			return fmt.Errorf("rank fabric must expose roceP2p1s0f0 GID 3 as IPv4 RoCE v2 for %s on enP2p1s0f0np0", address)
		}
	}
	return nil
}

func (m *tensorFoldManager) verifyHeadLaunchBoundary(ctx context.Context, resource tensorFoldResource) error {
	if _, err := m.runner.Run(ctx, "", m.commandEnvironment(false), "docker", "info", "--format", "{{.ServerVersion}}"); err != nil {
		return fmt.Errorf("local Docker is unavailable: %w", err)
	}
	if exists, err := m.recipeContainerNameExists(ctx, resource, false); err != nil || exists {
		return fmt.Errorf("head fixed container name is not exclusively available")
	}
	if exists, err := m.recipeContainerNameExists(ctx, resource, true); err != nil || exists {
		return fmt.Errorf("worker fixed container name is not exclusively available")
	}
	if err := m.verifyTensorFoldImageProvenance(ctx, resource, false); err != nil {
		return err
	}
	if err := m.verifyTensorFoldImageProvenance(ctx, resource, true); err != nil {
		return err
	}
	return m.verifyTensorFoldFabric(ctx, resource)
}

func (m *tensorFoldManager) restart(ctx context.Context, name string) error {
	resource, err := m.inspect(name)
	if err != nil {
		return err
	}
	if resource.Status != "stopped" {
		if err := m.stop(ctx, name); err != nil {
			return err
		}
	}
	if resource.Role == "worker" {
		m.mu.Lock()
		defer m.mu.Unlock()
		resource.Status = "running"
		return m.writeResource(resource)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensurePinnedRecipe(ctx); err != nil {
		return err
	}
	if err := m.verifyHeadLaunchBoundary(ctx, resource); err != nil {
		return err
	}
	request := tensorFoldResourceRequest{
		Name: resource.Name, DeploymentID: resource.DeploymentID, Generation: resource.Generation, Role: resource.Role,
		WorkerUser: resource.WorkerUser, WorkerAddress: resource.WorkerAddress, HeadFabricAddress: resource.HeadFabricAddress,
		ServiceAddress: resource.ServiceAddress, ServicePort: resource.ServicePort, Repository: resource.Repository, Commit: resource.Commit,
	}
	launchID, err := newTensorFoldLaunchID()
	if err != nil {
		return err
	}
	resource.LaunchID = launchID
	resource.HeadContainerID = ""
	resource.WorkerContainerID = ""
	environment, err := renderTensorFoldRecipeEnvironment(request, launchID)
	if err != nil {
		return err
	}
	if err := m.writeRecipeEnvironment(environment); err != nil {
		return err
	}
	if err := m.ensureTensorFoldSSHTransport(resource); err != nil {
		return err
	}
	logFile, err := m.openLog(name)
	if err != nil {
		return err
	}
	resource.Status = "starting"
	resource.LaunchStartedAt = time.Now().UTC()
	resource.ReadinessDeadline = resource.LaunchStartedAt.Add(tensorFoldPreparationDeadline)
	if err := m.writeResource(resource); err != nil {
		_ = logFile.Close()
		return err
	}
	process, err := m.runner.Start(context.Background(), m.recipePath(), m.commandEnvironment(true, launchID), logFile, "./start.sh")
	if err != nil {
		_ = logFile.Close()
		resource.Status = "failed"
		_ = m.writeResource(resource)
		return err
	}
	launch := &tensorFoldLaunch{id: launchID, process: process, done: make(chan struct{})}
	m.processes[name] = launch
	go m.waitForProcess(name, launch, logFile)
	return nil
}

func (m *tensorFoldManager) logs(ctx context.Context, resource tensorFoldResource) (string, bool, error) {
	var data []byte
	readTruncated := false
	if resource.Role == "head" {
		path := m.logPath(resource.Name)
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			return "", false, nil
		}
		if statErr != nil {
			return "", false, statErr
		}
		if err := validateOwnedTensorFoldDirectory(filepath.Dir(path)); err != nil {
			return "", false, err
		}
		if err := rejectTensorFoldSymlinkComponents(path, false); err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !tensorFoldFileOwnedByCurrentUser(info) || info.Mode().Perm()&0o077 != 0 {
			return "", false, fmt.Errorf("TensorFold supervisor log path is unsafe")
		}
		var err error
		if data, readTruncated, err = readTensorFoldLogTail(path); err != nil {
			return "", false, err
		}
	} else {
		var err error
		if data, err = m.runner.Run(ctx, "", m.commandEnvironment(false), "docker", "logs", "--tail", strconv.Itoa(deployments.MaxLogTailLines), tensorFoldContainerName); err != nil {
			return "", false, err
		}
	}
	tail, truncated := deployments.SanitizeLogTail(string(data), "")
	return tail, truncated || readTruncated, nil
}

// tensorFoldLogReadLimit bounds the supervisor log suffix loaded into memory.
// The supervisor log appends across restarts, so it can grow without bound;
// four times the persisted tail leaves room for sanitization to drop bytes.
const tensorFoldLogReadLimit = 4 * deployments.MaxLogTailBytes

func readTensorFoldLogTail(path string) ([]byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	offset := info.Size() - tensorFoldLogReadLimit
	if offset <= 0 {
		data, err := io.ReadAll(io.LimitReader(file, tensorFoldLogReadLimit))
		return data, false, err
	}
	data, err := io.ReadAll(io.NewSectionReader(file, offset, tensorFoldLogReadLimit))
	if err != nil {
		return nil, false, err
	}
	// Drop the partial line at the seek boundary.
	if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
		data = data[newline+1:]
	}
	return data, true, nil
}
