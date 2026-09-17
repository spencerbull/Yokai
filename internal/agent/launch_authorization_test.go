package agent

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/launchauth"
)

func TestCoordinatedQwenAuthorizationAcceptsExactCandidateOnlyOnce(t *testing.T) {
	request, publicKey, privateKey, now := authorizedQwenTestRequest(t)
	store := launchauth.NewReplayStore(filepath.Join(t.TempDir(), "agent.json"))
	request.LaunchAuthorization = signAgentLaunchRequest(t, privateKey, request, now, "spark-a")
	if err := authorizeCoordinatedQwenLaunch(request, now, publicKey, "spark-a", store); err != nil {
		t.Fatalf("valid coordinator authorization was rejected: %v", err)
	}
	if err := authorizeCoordinatedQwenLaunch(request, now, publicKey, "spark-a", store); !errors.Is(err, launchauth.ErrReplay) {
		t.Fatalf("replayed coordinator authorization was not rejected: %v", err)
	}

	differentCandidate := request
	differentCandidate.Name = "yokai-deployment-dep-1-g7-other"
	differentStore := launchauth.NewReplayStore(filepath.Join(t.TempDir(), "agent.json"))
	if err := authorizeCoordinatedQwenLaunch(differentCandidate, now, publicKey, "spark-a", differentStore); err == nil || !strings.Contains(err.Error(), "exact candidate") {
		t.Fatalf("cross-candidate authorization reuse was not rejected: %v", err)
	}
}

func TestCoordinatedQwenAuthorizationIsBoundToOneAgentIdentityAndReplayStore(t *testing.T) {
	request, publicKey, privateKey, now := authorizedQwenTestRequest(t)
	request.LaunchAuthorization = signAgentLaunchRequest(t, privateKey, request, now, "spark-a")
	agentAStore := launchauth.NewReplayStore(filepath.Join(t.TempDir(), "agent-a.json"))
	agentBStore := launchauth.NewReplayStore(filepath.Join(t.TempDir(), "agent-b.json"))

	if err := authorizeCoordinatedQwenLaunch(request, now, publicKey, "spark-b", agentBStore); err == nil || !strings.Contains(err.Error(), "target device") {
		t.Fatalf("agent B accepted authorization intended for agent A: %v", err)
	}
	if err := authorizeCoordinatedQwenLaunch(request, now, publicKey, "spark-a", agentAStore); err != nil {
		t.Fatalf("agent A rejected its authorization after cross-device attempt: %v", err)
	}
	if err := authorizeCoordinatedQwenLaunch(request, now, publicKey, "spark-a", agentAStore); !errors.Is(err, launchauth.ErrReplay) {
		t.Fatalf("agent A accepted the same authorization twice: %v", err)
	}
}

func TestCoordinatedQwenAuthorizationRejectsMissingLocalDeviceIdentity(t *testing.T) {
	request, publicKey, privateKey, now := authorizedQwenTestRequest(t)
	request.LaunchAuthorization = signAgentLaunchRequest(t, privateKey, request, now, "spark-a")
	store := launchauth.NewReplayStore(filepath.Join(t.TempDir(), "agent.json"))
	if err := authorizeCoordinatedQwenLaunch(request, now, publicKey, "", store); !errors.Is(err, errCoordinatorAuthorizationUnavailable) {
		t.Fatalf("missing local device identity did not fail closed: %v", err)
	}
}

func TestCoordinatedQwenAuthorizationRejectsFabricatedExpiredTamperedAndWrongRole(t *testing.T) {
	base, publicKey, privateKey, now := authorizedQwenTestRequest(t)
	tests := []struct {
		name   string
		mutate func(*ContainerRequest)
		signAt time.Time
	}{
		{name: "fabricated labels without authorization", mutate: func(req *ContainerRequest) { req.LaunchAuthorization = "" }, signAt: now},
		{name: "expired", mutate: func(*ContainerRequest) {}, signAt: now.Add(-launchauth.Lifetime)},
		{name: "tampered", mutate: func(req *ContainerRequest) {
			req.LaunchAuthorization = tamperAgentAuthorization(req.LaunchAuthorization)
		}, signAt: now},
		{name: "wrong role", mutate: func(req *ContainerRequest) { req.Labels[LabelRole] = bkc.MultiDeviceRoleWorker }, signAt: now},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := cloneAuthorizationTestRequest(base)
			request.LaunchAuthorization = signAgentLaunchRequest(t, privateKey, request, test.signAt, "spark-a")
			test.mutate(&request)
			store := launchauth.NewReplayStore(filepath.Join(t.TempDir(), "agent.json"))
			if err := authorizeCoordinatedQwenLaunch(request, now, publicKey, "spark-a", store); err == nil {
				t.Fatal("invalid coordinator authorization was accepted")
			}
		})
	}
}

func TestLaunchAuthorizationIsRejectedForLegacyContainer(t *testing.T) {
	request := ContainerRequest{Image: "example.invalid/image", Name: "legacy", Labels: map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged}}
	if err := authorizeCoordinatedQwenLaunch(request, time.Now(), nil, "", nil); err != nil {
		t.Fatalf("legacy request without coordinator authorization changed behavior: %v", err)
	}
	request.LaunchAuthorization = "not-for-legacy"
	if err := authorizeCoordinatedQwenLaunch(request, time.Now(), nil, "", nil); err == nil {
		t.Fatal("legacy request accepted a coordinator-only authorization")
	}
}

func TestQwenCandidateCannotBypassAuthorizationByOmittingBKCLabel(t *testing.T) {
	request, publicKey, _, now := authorizedQwenTestRequest(t)
	delete(request.Labels, LabelBKCID)
	request.LaunchAuthorization = ""
	store := launchauth.NewReplayStore(filepath.Join(t.TempDir(), "agent.json"))
	if err := authorizeCoordinatedQwenLaunch(request, now, publicKey, "spark-a", store); err == nil {
		t.Fatal("Qwen candidate without its BKC label bypassed coordinator authorization")
	}
}

func TestInvalidQwenAuthorizationsAreRejectedBeforeDockerMutation(t *testing.T) {
	base, publicKey, privateKey, now := authorizedQwenTestRequest(t)
	marker := filepath.Join(t.TempDir(), "docker-called")
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte("#!/bin/sh\n: > "+marker+"\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	originalPublicKey := coordinatorVerificationKey
	originalDeviceID := agentDeviceID
	originalReplayStore := launchAuthorizationReplayStore
	coordinatorVerificationKey = publicKey
	agentDeviceID = "spark-a"
	defer func() {
		coordinatorVerificationKey = originalPublicKey
		agentDeviceID = originalDeviceID
		launchAuthorizationReplayStore = originalReplayStore
	}()

	tests := []struct {
		name          string
		mutate        func(*ContainerRequest)
		signAt        time.Time
		localDeviceID string
	}{
		{name: "expired", mutate: func(*ContainerRequest) {}, signAt: now.Add(-launchauth.Lifetime), localDeviceID: "spark-a"},
		{name: "tampered", mutate: func(req *ContainerRequest) {
			req.LaunchAuthorization = tamperAgentAuthorization(req.LaunchAuthorization)
		}, signAt: now, localDeviceID: "spark-a"},
		{name: "wrong role", mutate: func(req *ContainerRequest) { req.Labels[LabelRole] = bkc.MultiDeviceRoleWorker }, signAt: now, localDeviceID: "spark-a"},
		{name: "wrong device", mutate: func(*ContainerRequest) {}, signAt: now, localDeviceID: "spark-b"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestBody := cloneAuthorizationTestRequest(base)
			requestBody.LaunchAuthorization = signAgentLaunchRequest(t, privateKey, requestBody, test.signAt, "spark-a")
			test.mutate(&requestBody)
			agentDeviceID = test.localDeviceID
			launchAuthorizationReplayStore = launchauth.NewReplayStore(filepath.Join(t.TempDir(), "agent.json"))
			body, err := json.Marshal(requestBody)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/containers", bytes.NewReader(body))
			recorder := httptest.NewRecorder()
			handleContainerDeploy(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("invalid authorization was not rejected: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Docker was invoked for invalid coordinator authorization: %v", err)
	}
}

func authorizedQwenTestRequest(t *testing.T) (ContainerRequest, ed25519.PublicKey, ed25519.PrivateKey, time.Time) {
	t.Helper()
	publicKey, privateKey, err := launchauth.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	request := ContainerRequest{
		Image: bkc.Qwen38FlashNextNVFP4Image, Name: "yokai-deployment-dep-1-g7-head", Model: bkc.Qwen38FlashNextContainerModel,
		Ports: map[string]string{"8888": "8888"}, Env: map[string]string{"NCCL_NET": "IB"}, GPUIDs: "0",
		Args: []string{"serve", "--api-key=sentinel-api-key"}, Volumes: map[string]string{"/host/repo": "/root/.cache/huggingface/hub/models--nvidia--Qwen3.8-Flash-Next-NVFP4:ro"},
		SkipPull: true, NetworkMode: "host", Devices: []string{"/dev/infiniband:/dev/infiniband"}, CapAdd: []string{"SYS_NICE"},
		Labels: map[string]string{
			LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelDeploymentID: "dep-1", LabelGeneration: "7",
			LabelRole: bkc.MultiDeviceRoleHead, LabelBKCID: bkc.Qwen38FlashNextNVFP4DualGB10ID,
			LabelModelRevision: bkc.Qwen38FlashNextNVFP4Revision, LabelSourceRevision: bkc.Qwen38FlashNextSourceRevision,
			LabelImageDigest: strings.TrimPrefix(bkc.Qwen38FlashNextNVFP4Image[strings.Index(bkc.Qwen38FlashNextNVFP4Image, "@")+1:], "sha256:"),
		},
	}
	return request, publicKey, privateKey, now
}

func signAgentLaunchRequest(t *testing.T, privateKey ed25519.PrivateKey, request ContainerRequest, now time.Time, targetDeviceID string) string {
	t.Helper()
	generation := 7
	claims, err := launchauth.NewClaims(now, launchAuthorizationRequest(request), targetDeviceID, request.Labels[LabelDeploymentID], generation,
		normalizedContainerName(request.Name), request.Labels[LabelRole], request.Labels[LabelBKCID], request.Labels[LabelModelRevision],
		request.Labels[LabelSourceRevision], request.Labels[LabelImageDigest])
	if err != nil {
		t.Fatal(err)
	}
	token, err := launchauth.Sign(privateKey, claims)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func cloneAuthorizationTestRequest(request ContainerRequest) ContainerRequest {
	cloned := request
	cloned.Labels = make(map[string]string, len(request.Labels))
	for key, value := range request.Labels {
		cloned.Labels[key] = value
	}
	cloned.Args = append([]string(nil), request.Args...)
	return cloned
}

func tamperAgentAuthorization(token string) string {
	index := len(token) / 2
	replacement := byte('A')
	if token[index] == replacement {
		replacement = 'B'
	}
	return token[:index] + string(replacement) + token[index+1:]
}
