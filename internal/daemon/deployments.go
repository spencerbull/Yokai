package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/deployments"
)

func deploymentsPath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, deployments.StoreFile), nil
}

func (d *Daemon) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	var request deployments.CreateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": err.Error()})
		return
	}
	outcome, err := d.deploymentEngine.CreateWithOutcome(r.Context(), request)
	deployment := outcome.Deployment
	if err != nil {
		status, code := deploymentErrorResponse(err, "deployment_failed")
		writeJSON(w, status, map[string]any{"error": code, "message": err.Error(), "deployment": safeDeployment(deployment)})
		return
	}
	status := http.StatusOK
	if outcome.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, safeDeployment(deployment))
}

func (d *Daemon) handleListDeployments(w http.ResponseWriter, _ *http.Request) {
	items := d.deploymentEngine.List()
	for index := range items {
		items[index] = safeDeployment(items[index])
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": items})
}

func (d *Daemon) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	deployment, err := d.deploymentEngine.Get(r.PathValue("deploymentID"))
	if err != nil {
		writeDeploymentLookupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, safeDeployment(deployment))
}

func (d *Daemon) handleTestDeployment(w http.ResponseWriter, r *http.Request) {
	var request deployments.TestRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": err.Error()})
		return
	}
	deployment, err := d.deploymentEngine.Test(r.Context(), r.PathValue("deploymentID"), request.APIKey)
	if err != nil {
		writeDeploymentActionError(w, "deployment_test_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, safeDeployment(deployment))
}

func (d *Daemon) handleStopDeployment(w http.ResponseWriter, r *http.Request) {
	deployment, err := d.deploymentEngine.Stop(r.Context(), r.PathValue("deploymentID"))
	if err != nil {
		writeDeploymentActionError(w, "deployment_stop_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, safeDeployment(deployment))
}

func (d *Daemon) handleStartDeployment(w http.ResponseWriter, r *http.Request) {
	var request deployments.TestRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": err.Error()})
		return
	}
	deployment, err := d.deploymentEngine.Start(r.Context(), r.PathValue("deploymentID"), request.APIKey)
	if err != nil {
		writeDeploymentStartError(w, deployment, err)
		return
	}
	writeJSON(w, http.StatusOK, safeDeployment(deployment))
}

func writeDeploymentStartError(w http.ResponseWriter, deployment deployments.Deployment, err error) {
	if errors.Is(err, deployments.ErrNotFound) {
		writeDeploymentLookupError(w, err)
		return
	}
	status, code := deploymentErrorResponse(err, "deployment_start_failed")
	writeJSON(w, status, map[string]any{
		"error":      code,
		"message":    err.Error(),
		"deployment": safeDeployment(deployment),
	})
}

func (d *Daemon) handleRollbackDeployment(w http.ResponseWriter, r *http.Request) {
	deployment, err := d.deploymentEngine.Rollback(r.Context(), r.PathValue("deploymentID"))
	if err != nil {
		writeDeploymentActionError(w, "deployment_rollback_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, safeDeployment(deployment))
}

func writeDeploymentLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, deployments.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "deployment_not_found", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "deployment_read_failed", "message": err.Error()})
}

func writeDeploymentActionError(w http.ResponseWriter, code string, err error) {
	if errors.Is(err, deployments.ErrNotFound) {
		writeDeploymentLookupError(w, err)
		return
	}
	status, typedCode := deploymentErrorResponse(err, code)
	writeJSON(w, status, map[string]string{"error": typedCode, "message": err.Error()})
}

func deploymentErrorResponse(err error, fallback string) (int, string) {
	if errors.Is(err, deployments.ErrIdempotencyConflict) {
		return http.StatusConflict, "idempotency_conflict"
	}
	if errors.Is(err, deployments.ErrRecoveryPerformed) {
		return http.StatusConflict, "idempotency_recovered"
	}
	switch deployments.ErrorKindOf(err) {
	case deployments.ErrorValidation:
		return http.StatusBadRequest, "invalid_deployment"
	case deployments.ErrorConflict:
		return http.StatusConflict, "deployment_conflict"
	case deployments.ErrorDependency:
		return http.StatusBadGateway, "deployment_dependency"
	case deployments.ErrorUnavailable:
		return http.StatusServiceUnavailable, "deployment_unavailable"
	default:
		return http.StatusInternalServerError, fallback
	}
}

func safeDeployment(deployment deployments.Deployment) deployments.Deployment {
	deployment.RequestHash = ""
	return deployment
}

func (d *Daemon) guardGroupedMemberMutation(deviceID, containerID string) error {
	if d.deploymentEngine == nil {
		return nil
	}
	for _, deployment := range d.deploymentEngine.List() {
		for _, member := range deployment.Members {
			matchesMember := sameContainerID(member.ContainerID, containerID) || (member.Name != "" && member.Name == strings.TrimSpace(containerID))
			if member.DeviceID == deviceID && matchesMember && member.Ownership == deployments.OwnershipManaged {
				return fmt.Errorf("container %s is managed by deployment %s; use /deployments/%s lifecycle operations", containerID, deployment.ID, deployment.ID)
			}
		}
	}
	return nil
}

func sameContainerID(left, right string) bool {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	return left == right || (len(left) >= 12 && len(right) >= 12 && (strings.HasPrefix(left, right) || strings.HasPrefix(right, left)))
}

type daemonDeploymentOperations struct {
	daemon       *Daemon
	waitTimeout  time.Duration
	waitInterval time.Duration
}

type agentHealth struct {
	Status       string   `json:"status"`
	Capabilities []string `json:"capabilities"`
}

type agentSystemInfo struct {
	GPUs []struct {
		Name string `json:"name"`
	} `json:"gpus"`
}

type agentInventory struct {
	Containers []agentContainer `json:"containers"`
}

type agentContainer struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Ownership string            `json:"ownership"`
	Labels    map[string]string `json:"labels"`
}

type deploymentLaunchPayload struct {
	Image       string                `json:"image"`
	Name        string                `json:"name"`
	Model       string                `json:"model"`
	Ports       map[string]string     `json:"ports"`
	Env         map[string]string     `json:"env"`
	GPUIDs      string                `json:"gpu_ids"`
	ExtraArgs   string                `json:"extra_args"`
	Args        []string              `json:"args,omitempty"`
	Volumes     map[string]string     `json:"volumes"`
	Runtime     config.RuntimeOptions `json:"runtime"`
	SkipPull    bool                  `json:"skip_pull"`
	Labels      map[string]string     `json:"labels"`
	NetworkMode string                `json:"network_mode"`
	Devices     []string              `json:"devices"`
	CapAdd      []string              `json:"cap_add"`
}

func (ops *daemonDeploymentOperations) Preflight(ctx context.Context, request deployments.PreflightRequest, requiredCapabilities, targetDevices []string) error {
	binding := request.Binding
	d := ops.daemon
	d.mu.RLock()
	device := d.cfg.FindDevice(binding.DeviceID)
	if device == nil {
		d.mu.RUnlock()
		return deployments.WrapError(deployments.ErrorValidation, "resolve device", fmt.Errorf("device %s is not configured", binding.DeviceID))
	}
	deviceCopy := *device
	d.mu.RUnlock()
	if d.tunnels.LocalPort(binding.DeviceID) == 0 {
		return deployments.WrapError(deployments.ErrorUnavailable, "connect agent", fmt.Errorf("device %s is offline", binding.DeviceID))
	}
	var health agentHealth
	if err := ops.getJSON(ctx, binding.DeviceID, "/health", &health); err != nil {
		return deployments.WrapError(deployments.ErrorUnavailable, "agent health", err)
	}
	if health.Status != "ok" {
		return deployments.WrapError(deployments.ErrorUnavailable, "agent health", fmt.Errorf("agent health is %q", health.Status))
	}
	available := make(map[string]struct{}, len(health.Capabilities))
	for _, capability := range health.Capabilities {
		available[capability] = struct{}{}
	}
	for _, capability := range requiredCapabilities {
		if _, ok := available[capability]; !ok {
			return deployments.WrapError(deployments.ErrorConflict, "agent capability", fmt.Errorf("agent lacks required capability %s", capability))
		}
	}
	var info agentSystemInfo
	if err := ops.getJSON(ctx, binding.DeviceID, "/system/info", &info); err != nil {
		return classifyAgentPreflightDependency("agent system info", err)
	}
	if len(info.GPUs) != 1 {
		return deployments.WrapError(deployments.ErrorConflict, "agent GPU capability", fmt.Errorf("device %s must expose exactly one GPU, got %d", binding.DeviceID, len(info.GPUs)))
	}
	if requiresDevice(targetDevices, bkc.DeviceGB10) && !deviceIsGB10(deviceCopy, info.GPUs[0].Name) {
		return deployments.WrapError(deployments.ErrorConflict, "agent GPU capability", fmt.Errorf("device %s is not identified as GB10", binding.DeviceID))
	}
	payload := struct {
		FabricAddress     string `json:"fabric_address"`
		ServiceAddress    string `json:"service_address,omitempty"`
		ServicePort       int    `json:"service_port"`
		RendezvousPort    int    `json:"rendezvous_port"`
		Head              bool   `json:"head"`
		ObservedContainer string `json:"observed_container_id,omitempty"`
		CandidateName     string `json:"candidate_name"`
		LocalModelPath    string `json:"local_model_path,omitempty"`
	}{binding.FabricAddress, binding.ServiceAddress, request.ServicePort, request.RendezvousPort, request.Head, binding.ObservedContainerID, request.CandidateName, request.LocalModelPath}
	if err := ops.postJSON(ctx, binding.DeviceID, "/deployments/preflight", payload, nil, 30*time.Second); err != nil {
		return classifyAgentPreflightError(err)
	}
	return nil
}

func requiresDevice(targets []string, target string) bool {
	for _, candidate := range targets {
		if strings.EqualFold(candidate, target) {
			return true
		}
	}
	return false
}

func deviceIsGB10(device config.Device, gpuName string) bool {
	// Inventory tags are operator-editable hints. They may narrow selection in
	// callers, but they cannot establish the hardware capability required here.
	_ = device
	name := strings.ToLower(gpuName)
	return strings.Contains(name, "gb10") || strings.Contains(name, "dgx spark")
}

func classifyAgentPreflightError(err error) error {
	var responseErr *agentHTTPError
	if errors.As(err, &responseErr) {
		switch responseErr.Status {
		case http.StatusBadRequest:
			return deployments.WrapError(deployments.ErrorValidation, "agent deployment preflight", err)
		case http.StatusConflict:
			return deployments.WrapError(deployments.ErrorConflict, "agent deployment preflight", err)
		case http.StatusServiceUnavailable:
			return deployments.WrapError(deployments.ErrorUnavailable, "agent deployment preflight", err)
		default:
			return deployments.WrapError(deployments.ErrorDependency, "agent deployment preflight", err)
		}
	}
	return deployments.WrapError(deployments.ErrorUnavailable, "agent deployment preflight", err)
}

func classifyAgentPreflightDependency(operation string, err error) error {
	var responseErr *agentHTTPError
	if errors.As(err, &responseErr) && responseErr.Status == http.StatusServiceUnavailable {
		return deployments.WrapError(deployments.ErrorUnavailable, operation, err)
	}
	return deployments.WrapError(deployments.ErrorDependency, operation, err)
}

func (ops *daemonDeploymentOperations) Pull(ctx context.Context, binding deployments.Binding, image string) error {
	return deploymentAgentOperationError("pull candidate image", ops.postJSON(ctx, binding.DeviceID, "/images/pull", map[string]string{"image": image}, nil, deployments.DefaultImagePullRPCTimeout), false)
}

func (ops *daemonDeploymentOperations) Inspect(ctx context.Context, deviceID, containerID string) (deployments.ObservedContainer, error) {
	var inventory agentInventory
	if err := ops.getJSON(ctx, deviceID, "/containers?scope=all", &inventory); err != nil {
		return deployments.ObservedContainer{}, deploymentAgentOperationError("read agent container inventory", err, true)
	}
	container, err := matchObservedContainer(inventory.Containers, containerID)
	if err != nil {
		return deployments.ObservedContainer{}, err
	}
	generation, _ := strconv.Atoi(container.Labels["io.yokai.deployment.generation"])
	ownership := deployments.Ownership(container.Ownership)
	if ownership != deployments.OwnershipManaged && ownership != deployments.OwnershipAdopted {
		ownership = deployments.OwnershipObserved
	}
	return deployments.ObservedContainer{ID: container.ID, Name: container.Name, Status: container.Status, Ownership: ownership, Managed: container.Labels["io.yokai.managed"] == "true", Generation: generation, DeploymentID: container.Labels["io.yokai.deployment.id"], Role: container.Labels["io.yokai.deployment.role"]}, nil
}

func matchObservedContainer(containers []agentContainer, selector string) (agentContainer, error) {
	selector = strings.TrimSpace(selector)
	var names []agentContainer
	for _, container := range containers {
		if container.Name == selector {
			names = append(names, container)
		}
	}
	if len(names) == 1 {
		return names[0], nil
	}
	if len(names) > 1 {
		return agentContainer{}, deployments.WrapError(deployments.ErrorConflict, "match observed container", fmt.Errorf("container name %q is ambiguous", selector))
	}
	if len(selector) < 12 {
		return agentContainer{}, deployments.WrapError(deployments.ErrorValidation, "match observed container", fmt.Errorf("container ID prefix must contain at least 12 characters"))
	}
	var matches []agentContainer
	for _, container := range containers {
		if strings.HasPrefix(container.ID, selector) {
			matches = append(matches, container)
		}
	}
	switch len(matches) {
	case 0:
		return agentContainer{}, fmt.Errorf("%w: %s", deployments.ErrContainerNotFound, selector)
	case 1:
		return matches[0], nil
	default:
		return agentContainer{}, deployments.WrapError(deployments.ErrorConflict, "match observed container", fmt.Errorf("container ID prefix %q is ambiguous", selector))
	}
}

func (ops *daemonDeploymentOperations) Stop(ctx context.Context, deviceID, containerID string) error {
	return deploymentAgentOperationError("stop previous member", ops.daemon.aggregator.StopContainer(deviceID, containerID), false)
}

func (ops *daemonDeploymentOperations) StopManaged(ctx context.Context, deployment deployments.Deployment, member deployments.Member) error {
	return ops.mutateDeploymentMember(ctx, http.MethodPost, "stop", deployment, member)
}

func (ops *daemonDeploymentOperations) Launch(ctx context.Context, binding deployments.Binding, candidate deployments.Candidate) (deployments.ObservedContainer, error) {
	ports := map[string]string{}
	if candidate.Port != "" {
		ports[candidate.Port] = candidate.Port
	}
	payload := deploymentLaunchPayload{
		Image: candidate.Image, Name: candidate.Name, Model: candidate.Model,
		Ports: ports, Env: candidate.Env, GPUIDs: candidate.GPUIDs, ExtraArgs: candidate.ExtraArgs, Args: candidate.Args,
		Volumes: candidate.Volumes, Runtime: candidate.Runtime, SkipPull: true, Labels: candidate.Labels,
		NetworkMode: candidate.NetworkMode, Devices: candidate.Devices, CapAdd: candidate.CapAdd,
	}
	var response struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	launchCtx, cancel := detachedDeploymentLaunchContext(ctx)
	defer cancel()
	if err := ops.postJSON(launchCtx, binding.DeviceID, "/containers", payload, &response, deployments.DefaultCandidateLaunchRPCTimeout); err != nil {
		return deployments.ObservedContainer{}, deploymentAgentOperationError("launch candidate", err, false)
	}
	return deployments.ObservedContainer{ID: response.ID, Name: candidate.Name, Status: response.Status, Ownership: deployments.OwnershipManaged}, nil
}

func detachedDeploymentLaunchContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), deployments.DefaultCandidateLaunchRPCTimeout)
}

func (ops *daemonDeploymentOperations) WaitRunning(ctx context.Context, deviceID, containerID string) error {
	interval := ops.waitInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	timeoutDuration := ops.waitTimeout
	if timeoutDuration <= 0 {
		timeoutDuration = deployments.DefaultReadinessTimeout
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timeout := time.NewTimer(timeoutDuration)
	defer timeout.Stop()
	for {
		container, err := ops.Inspect(ctx, deviceID, containerID)
		if err == nil && container.Status == "running" {
			return nil
		}
		if err == nil && (container.Status == "stopped" || container.Status == "exited" || container.Status == "dead" || container.Status == "unknown") {
			return fmt.Errorf("candidate entered %s state", container.Status)
		}
		select {
		case <-ctx.Done():
			return deployments.WrapError(deployments.ErrorUnavailable, "wait for member running", ctx.Err())
		case <-timeout.C:
			return deployments.WrapError(deployments.ErrorDependency, "wait for member running", fmt.Errorf("candidate did not reach running state"))
		case <-ticker.C:
		}
	}
}

func (ops *daemonDeploymentOperations) Test(_ context.Context, deviceID, containerID, apiKey string) (deployments.TestResult, error) {
	result, err := ops.daemon.aggregator.TestContainerWithMetrics(deviceID, containerID, apiKey)
	if err != nil {
		return deployments.TestResult{}, deploymentAgentOperationError("test rank-0 service", err, false)
	}
	return deployments.TestResult{OK: result.OK, MetricsReady: result.MetricsReady, Message: result.Message, Model: result.Model, Response: result.Response}, nil
}

func (ops *daemonDeploymentOperations) CaptureManagedLogs(ctx context.Context, deployment deployments.Deployment, member deployments.Member, exactRedaction string) (deployments.LogTailCapture, error) {
	query := url.Values{}
	query.Set("generation", strconv.Itoa(member.Generation))
	query.Set("role", member.Role)
	query.Set("name", member.Name)
	path := "/deployments/" + url.PathEscape(deployment.ID) + "/members/" + url.PathEscape(memberLocatorForAgent(member)) + "/logs/tail?" + query.Encode()
	var response struct {
		Tail      string `json:"tail"`
		Truncated bool   `json:"truncated"`
	}
	captureCtx, cancel := context.WithTimeout(ctx, deployments.DefaultLogCaptureRPCTimeout)
	defer cancel()
	if err := ops.postJSON(captureCtx, member.DeviceID, path, map[string]string{"redact": exactRedaction}, &response, deployments.DefaultLogCaptureRPCTimeout); err != nil {
		return deployments.LogTailCapture{}, deploymentAgentOperationError("capture candidate logs", err, false)
	}
	tail, truncated := deployments.SanitizeLogTail(response.Tail, exactRedaction)
	return deployments.LogTailCapture{Tail: tail, Truncated: response.Truncated || truncated}, nil
}

func (ops *daemonDeploymentOperations) Remove(ctx context.Context, deviceID, containerID string) error {
	return deploymentAgentOperationError("remove previous member", ops.daemon.aggregator.RemoveContainer(deviceID, containerID), false)
}

func (ops *daemonDeploymentOperations) RemoveManaged(ctx context.Context, deployment deployments.Deployment, member deployments.Member) error {
	err := ops.mutateDeploymentMember(ctx, http.MethodDelete, "", deployment, member)
	var responseErr *agentHTTPError
	if errors.As(err, &responseErr) && responseErr.Status == http.StatusNotFound {
		return nil
	}
	return err
}

func (ops *daemonDeploymentOperations) Restart(ctx context.Context, deviceID, containerID string) error {
	return deploymentAgentOperationError("restart previous member", ops.daemon.aggregator.RestartContainer(deviceID, containerID), false)
}

func (ops *daemonDeploymentOperations) RestartManaged(ctx context.Context, deployment deployments.Deployment, member deployments.Member) error {
	return ops.mutateDeploymentMember(ctx, http.MethodPost, "restart", deployment, member)
}

func (ops *daemonDeploymentOperations) mutateDeploymentMember(ctx context.Context, method, action string, deployment deployments.Deployment, member deployments.Member) error {
	query := url.Values{}
	query.Set("generation", strconv.Itoa(member.Generation))
	query.Set("role", member.Role)
	query.Set("name", member.Name)
	path := "/deployments/" + url.PathEscape(deployment.ID) + "/members/" + url.PathEscape(memberLocatorForAgent(member))
	if action != "" {
		path += "/" + action
	}
	path += "?" + query.Encode()
	return deploymentAgentOperationError("mutate deployment member", ops.requestJSON(ctx, method, member.DeviceID, path, nil, nil, deploymentMemberMutationTimeout(method)), false)
}

func deploymentMemberMutationTimeout(method string) time.Duration {
	if method == http.MethodDelete {
		return deployments.DefaultCandidateLaunchRPCTimeout
	}
	return deployments.DefaultMemberMutationRPCTimeout
}

func memberLocatorForAgent(member deployments.Member) string {
	if member.ContainerID != "" {
		return member.ContainerID
	}
	return member.Name
}

func (ops *daemonDeploymentOperations) getJSON(ctx context.Context, deviceID, path string, out any) error {
	localPort := ops.daemon.tunnels.LocalPort(deviceID)
	if localPort == 0 {
		return fmt.Errorf("device %s is offline", deviceID)
	}
	requestURL := fmt.Sprintf("http://localhost:%d%s", localPort, path)
	request, err := ops.daemon.aggregator.agentRequest(http.MethodGet, requestURL, deviceID, nil)
	if err != nil {
		return err
	}
	request = request.WithContext(ctx)
	response, err := ops.daemon.aggregator.client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return agentResponseError(response)
	}
	return json.NewDecoder(response.Body).Decode(out)
}

func (ops *daemonDeploymentOperations) postJSON(ctx context.Context, deviceID, path string, body, out any, timeout time.Duration) error {
	return ops.requestJSON(ctx, http.MethodPost, deviceID, path, body, out, timeout)
}

func (ops *daemonDeploymentOperations) requestJSON(ctx context.Context, method, deviceID, path string, body, out any, timeout time.Duration) error {
	localPort := ops.daemon.tunnels.LocalPort(deviceID)
	if localPort == 0 {
		return fmt.Errorf("device %s is offline", deviceID)
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	requestURL := fmt.Sprintf("http://localhost:%d%s", localPort, path)
	request, err := ops.daemon.aggregator.agentRequest(method, requestURL, deviceID, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request = request.WithContext(ctx)
	client := &http.Client{Timeout: timeout}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return agentResponseError(response)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}

type agentHTTPError struct {
	Status  int
	Code    string
	Message string
}

func (e *agentHTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("agent %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("agent returned status %d", e.Status)
}

// agentErrorReadLimit bounds how much of an agent error response the daemon
// reads before decoding. It comfortably exceeds the agent's bounded pull
// diagnostic envelope so the real docker error is decoded rather than truncated.
const agentErrorReadLimit = 32 * 1024

func agentResponseError(response *http.Response) error {
	// The agent's pull diagnostics are bounded (last 8 KiB of docker output via
	// agent.boundedBuffer); read enough to decode the complete envelope so the
	// real docker error surfaces instead of being dropped as "agent returned
	// status 500".
	data, _ := io.ReadAll(io.LimitReader(response.Body, agentErrorReadLimit))
	var envelope struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &envelope) == nil && envelope.Message != "" {
		return &agentHTTPError{Status: response.StatusCode, Code: envelope.Error, Message: envelope.Message}
	}
	return &agentHTTPError{Status: response.StatusCode}
}

func deploymentAgentOperationError(op string, err error, clientErrorsAreConflicts bool) error {
	if err == nil || deployments.ErrorKindOf(err) != "" {
		return err
	}
	// Log remote operation failures with the agent's full message (which now
	// includes captured docker stderr) so the underlying cause is inspectable
	// in the daemon log instead of only the terse status surfaced to the TUI.
	if responseErr, ok := err.(*agentHTTPError); ok && responseErr.Message != "" {
		log.Printf("remote operation %q failed: %s", op, responseErr.Message)
	} else {
		log.Printf("remote operation %q failed: %v", op, err)
	}
	var responseErr *agentHTTPError
	if errors.As(err, &responseErr) {
		if responseErr.Code == "service_unauthorized" {
			return deployments.WrapError(deployments.ErrorDependency, op, deployments.ErrServiceUnauthorized)
		}
		if clientErrorsAreConflicts && responseErr.Status >= 400 && responseErr.Status < 500 {
			return deployments.WrapError(deployments.ErrorConflict, op, err)
		}
		if responseErr.Status == http.StatusNotFound || responseErr.Status == http.StatusConflict {
			return deployments.WrapError(deployments.ErrorConflict, op, err)
		}
		return deployments.WrapError(deployments.ErrorDependency, op, err)
	}
	return deployments.WrapError(deployments.ErrorUnavailable, op, err)
}
