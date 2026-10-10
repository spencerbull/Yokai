package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
)

type tensorFoldContainerInspect struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	Created string `json:"Created"`
	Config  struct {
		Image string   `json:"Image"`
		Env   []string `json:"Env"`
		Cmd   []string `json:"Cmd"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
}

func (m *tensorFoldManager) testResource(ctx context.Context, resource tensorFoldResource) (*ServiceTestResult, error) {
	if resource.Role != bkc.MultiDeviceRoleHead {
		return nil, fmt.Errorf("TensorFold semantic readiness belongs to the head resource")
	}
	if err := m.verifyRecipeContainer(ctx, resource, false); err != nil {
		return nil, fmt.Errorf("head recipe container: %w", err)
	}
	if err := m.verifyRecipeContainer(ctx, resource, true); err != nil {
		return nil, fmt.Errorf("worker recipe container: %w", err)
	}
	baseURL := m.serviceBaseURL(resource)

	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := m.getTensorFoldJSON(ctx, baseURL+"/v1/models", &models); err != nil {
		return nil, fmt.Errorf("TensorFold model discovery: %w", err)
	}
	if len(models.Data) != 1 || models.Data[0].ID != bkc.GLM53FlashEXL3TensorFoldServedModel {
		return nil, fmt.Errorf("TensorFold served model identity mismatch")
	}
	var tokenize struct {
		MaxModelLen int `json:"max_model_len"`
	}
	if err := m.postTensorFoldJSON(ctx, baseURL+"/tokenize", map[string]any{"prompt": "policy probe"}, &tokenize); err != nil {
		return nil, fmt.Errorf("TensorFold tokenizer metadata: %w", err)
	}
	if tokenize.MaxModelLen != 1048576 {
		return nil, fmt.Errorf("TensorFold effective context is %d, want 1048576", tokenize.MaxModelLen)
	}
	var health json.RawMessage
	if err := m.getTensorFoldJSON(ctx, baseURL+"/health", &health); err != nil {
		return nil, fmt.Errorf("TensorFold health endpoint: %w", err)
	}
	metrics, err := scrapeVLLMMetricsURLWithClient(m.httpClient, baseURL+"/metrics", "")
	if err != nil {
		return nil, fmt.Errorf("TensorFold metrics: %w", err)
	}
	if !metrics.HasTensorFoldNativeMetric || !metrics.HasRequestsRunning || !metrics.HasTensorFoldContext || !metrics.HasTensorFoldStreamsMax || !metrics.HasTensorFoldPoolTokens {
		return nil, fmt.Errorf("TensorFold native metrics are unavailable")
	}
	if metrics.TensorFoldContextLength != 1048576 || metrics.TensorFoldStreamsMax != 4 || metrics.TensorFoldPoolTokens < 1048576 {
		return nil, fmt.Errorf("TensorFold effective metric policy mismatch: context=%.0f streams=%.0f pool=%.0f", metrics.TensorFoldContextLength, metrics.TensorFoldStreamsMax, metrics.TensorFoldPoolTokens)
	}

	smoke := map[string]any{
		"model":      bkc.GLM53FlashEXL3TensorFoldServedModel,
		"messages":   []map[string]string{{"role": "user", "content": "Reply with exactly: OK"}},
		"max_tokens": 32, "temperature": 0,
		"chat_template_kwargs": map[string]bool{"enable_thinking": false},
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := m.postTensorFoldJSON(ctx, baseURL+"/v1/chat/completions", smoke, &completion); err != nil {
		return nil, fmt.Errorf("TensorFold non-streaming smoke: %w", err)
	}
	if len(completion.Choices) != 1 || !strings.EqualFold(strings.TrimSpace(completion.Choices[0].Message.Content), "ok") {
		return nil, fmt.Errorf("TensorFold smoke response was not exactly OK")
	}
	smoke["stream"] = true
	if err := m.testTensorFoldStream(ctx, baseURL+"/v1/chat/completions", smoke); err != nil {
		return nil, err
	}
	if err := m.markReady(resource); err != nil {
		return nil, fmt.Errorf("persist TensorFold readiness: %w", err)
	}
	return &ServiceTestResult{
		OK: true, ServiceType: "tensorfold", Message: "TensorFold effective policy and chat probes passed",
		Model: bkc.GLM53FlashEXL3TensorFoldServedModel, Response: strings.TrimSpace(completion.Choices[0].Message.Content), MetricsReady: true,
	}, nil
}

// markReady records proven semantic readiness, retiring this launch's
// agent-side deadline.
func (m *tensorFoldManager) markReady(resource tensorFoldResource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.readResource(resource.Name)
	if err != nil {
		return err
	}
	if !sameTensorFoldGeneration(resource, current) {
		return fmt.Errorf("TensorFold launch changed during readiness probing")
	}
	if current.ReadinessDeadline.IsZero() {
		return nil
	}
	current.ReadinessDeadline = time.Time{}
	return m.writeResource(current)
}

func (m *tensorFoldManager) verifyRecipeContainer(ctx context.Context, resource tensorFoldResource, worker bool) error {
	exists, running, err := m.inspectRecipeContainer(ctx, resource, worker)
	if err != nil {
		return err
	}
	if !exists || !running {
		return fmt.Errorf("container is not running")
	}
	return nil
}

func (m *tensorFoldManager) inspectRecipeContainer(ctx context.Context, resource tensorFoldResource, worker bool) (bool, bool, error) {
	exists, running, _, err := m.inspectRecipeContainerIdentity(ctx, resource, worker)
	return exists, running, err
}

func (m *tensorFoldManager) inspectRecipeContainerIdentity(ctx context.Context, resource tensorFoldResource, worker bool) (bool, bool, string, error) {
	listArgs := []string{"ps", "-a", "--no-trunc", "--filter", "name=^/" + tensorFoldContainerName + "$", "--format", "{{.ID}}"}
	output, err := m.runTensorFoldNodeCommand(ctx, resource, worker, "docker", listArgs...)
	if err != nil {
		return false, false, "", err
	}
	identifiers := strings.Fields(string(output))
	if len(identifiers) == 0 {
		expectedID := resource.HeadContainerID
		if worker {
			expectedID = resource.WorkerContainerID
		}
		if expectedID != "" {
			exists, exactErr := m.recipeContainerIDExists(ctx, resource, worker, expectedID)
			if exactErr != nil {
				return false, false, "", exactErr
			}
			if exists {
				return true, false, expectedID, fmt.Errorf("durable container ID exists without the fixed recipe name")
			}
		}
		return false, false, "", nil
	}
	if len(identifiers) != 1 {
		return false, false, "", fmt.Errorf("container name is ambiguous")
	}
	identifier := identifiers[0]
	expectedID := resource.HeadContainerID
	if worker {
		expectedID = resource.WorkerContainerID
	}
	if expectedID != "" && identifier != expectedID {
		return true, false, identifier, fmt.Errorf("fixed container name resolves to %s, not durable owned ID %s", identifier, expectedID)
	}
	output, err = m.runTensorFoldNodeCommand(ctx, resource, worker, "docker", "inspect", identifier)
	if err != nil {
		return false, false, "", err
	}
	var records []tensorFoldContainerInspect
	if err := json.Unmarshal(output, &records); err != nil || len(records) != 1 {
		return false, false, "", fmt.Errorf("invalid container inspection")
	}
	record := records[0]
	if record.ID != identifier || record.Name != "/"+tensorFoldContainerName {
		return true, record.State.Running, identifier, fmt.Errorf("container inspection identity does not match the fixed target")
	}
	// Only the head's creation time shares a clock with LaunchStartedAt. The
	// worker's generation is proven by the launch ID marker checked below.
	if !worker {
		created, timeErr := time.Parse(time.RFC3339Nano, record.Created)
		if timeErr != nil || resource.LaunchStartedAt.IsZero() || created.Before(resource.LaunchStartedAt) {
			return true, record.State.Running, identifier, fmt.Errorf("container creation does not belong to this launch generation")
		}
	}
	pinnedImage, _, ok := tensorFoldPinnedImageForCommit(resource.Commit)
	if !ok || record.Config.Image != pinnedImage {
		return true, record.State.Running, identifier, fmt.Errorf("container image is not the pinned digest")
	}
	environment := make(map[string]string, len(record.Config.Env))
	environmentCounts := make(map[string]int, len(record.Config.Env))
	for _, item := range record.Config.Env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			environment[key] = value
			environmentCounts[key]++
		}
	}
	wantEnvironment := map[string]string{
		"HF_HUB_OFFLINE": "1", "TF_GLM_KV": "fp8", "TF_GLM_DENSE": "q4",
		"TENSORFOLD_MEMORY_RESERVE_GIB": "14.5", "TF_GLM_CACHE_GIB": "12.5",
		"NCCL_SOCKET_IFNAME": "enP2p1s0f0np0", "NCCL_IB_HCA": "roceP2p1s0f0", "NCCL_IB_GID_INDEX": "3",
	}
	for key, value := range wantEnvironment {
		if environmentCounts[key] != 1 || environment[key] != value {
			return true, record.State.Running, identifier, fmt.Errorf("container environment %s does not match pinned policy", key)
		}
	}
	if environmentCounts["TENSORFOLD_YOKAI_LAUNCH_ID"] != 1 || environment["TENSORFOLD_YOKAI_LAUNCH_ID"] != resource.LaunchID {
		return true, record.State.Running, identifier, fmt.Errorf("container launch identity does not match the durable generation")
	}
	command := record.Config.Cmd
	modelPath := "/root/.cache/huggingface/hub/models--Mia-AiLab--GLM-5.3-Flash-EXL3-4bpw-TensorFold/snapshots/" + bkc.GLM53FlashEXL3TensorFoldModelRevision
	drafterPath := "/root/.cache/huggingface/hub/models--incoai--GLM-5.3-Flash-DFlash2/snapshots/" + bkc.GLM53FlashEXL3TensorFoldDrafterRevision
	if len(command) < 3 || command[0] != "tensorfold" || command[1] != "serve" || command[2] != modelPath || !tensorFoldCommandHasExactFlag(command, "--drafter", drafterPath) {
		return true, record.State.Running, identifier, fmt.Errorf("container model and drafter positions do not match pinned snapshots")
	}
	wantRank := "0"
	if worker {
		wantRank = "1"
	}
	for flag, value := range map[string]string{
		"--tp": "2", "--rank": wantRank, "--master": resource.HeadFabricAddress,
		"--master-port": strconv.Itoa(bkc.GLM53FlashEXL3TensorFoldRendezvousPort),
		"--context":     "1048576", "--parallel": "4",
	} {
		if !tensorFoldCommandHasExactFlag(command, flag, value) {
			return true, record.State.Running, identifier, fmt.Errorf("container command %s does not match pinned policy", flag)
		}
	}
	if !tensorFoldCommandHasSwitch(command, "--vision") {
		return true, record.State.Running, identifier, fmt.Errorf("container model, drafter, or vision policy does not match pinned recipe")
	}
	if !worker {
		if !tensorFoldCommandHasExactFlag(command, "--host", resource.ServiceAddress) ||
			!tensorFoldCommandHasExactFlag(command, "--port", strconv.Itoa(resource.ServicePort)) ||
			tensorFoldCommandHasExactFlag(command, "--host", "0.0.0.0") || tensorFoldCommandHasExactFlag(command, "--host", "::") {
			return true, record.State.Running, identifier, fmt.Errorf("head container service bind does not match the explicit private endpoint")
		}
	}
	return true, record.State.Running, identifier, nil
}

func (m *tensorFoldManager) runTensorFoldNodeCommand(ctx context.Context, resource tensorFoldResource, worker bool, command string, args ...string) ([]byte, error) {
	if !worker {
		return m.runner.Run(ctx, "", m.commandEnvironment(false), command, args...)
	}
	sshArgs := []string{
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10",
		resource.WorkerUser + "@" + resource.WorkerAddress, command,
	}
	// OpenSSH joins remote argv into a shell command; local exec argv alone
	// does not protect spaces, quoting, expansion, or shell metacharacters.
	sshArgs[len(sshArgs)-1] = quoteTensorFoldShellWord(command)
	for _, argument := range args {
		sshArgs = append(sshArgs, quoteTensorFoldShellWord(argument))
	}
	return m.runner.Run(ctx, "", m.commandEnvironment(false), "ssh", sshArgs...)
}

func quoteTensorFoldShellWord(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./^-", r)
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func tensorFoldCommandHasExactFlag(command []string, flag, value string) bool {
	occurrences, matches := 0, 0
	for index, item := range command {
		if item == flag {
			occurrences++
			if index+1 < len(command) && command[index+1] == value {
				matches++
			}
		}
		if strings.HasPrefix(item, flag+"=") {
			occurrences++
		}
	}
	return occurrences == 1 && matches == 1
}

func tensorFoldCommandHasSwitch(command []string, flag string) bool {
	count := 0
	for _, item := range command {
		if item == flag {
			count++
		} else if strings.HasPrefix(item, flag+"=") {
			return false
		}
	}
	return count == 1
}

func (m *tensorFoldManager) getTensorFoldJSON(ctx context.Context, url string, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return m.doTensorFoldJSON(request, output)
}

func (m *tensorFoldManager) postTensorFoldJSON(ctx context.Context, url string, input, output any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return m.doTensorFoldJSON(request, output)
}

func (m *tensorFoldManager) doTensorFoldJSON(request *http.Request, output any) error {
	response, err := m.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("endpoint returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(output)
}

func (m *tensorFoldManager) testTensorFoldStream(ctx context.Context, url string, input any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := m.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("TensorFold streaming smoke: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("TensorFold streaming smoke returned HTTP %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 4<<20))
	hasContent, done := false, false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "data: [DONE]" {
			done = true
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) == nil && len(event.Choices) > 0 && event.Choices[0].Delta.Content != "" {
			hasContent = true
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !hasContent || !done {
		return fmt.Errorf("TensorFold streaming smoke did not produce content and [DONE]")
	}
	return nil
}
