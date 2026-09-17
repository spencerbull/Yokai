package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/deployments"
)

func TestDeploymentDaemonClientUsesPatientBoundedTimeout(t *testing.T) {
	client := newDeploymentDaemonClient(&config.Config{})
	if client.http.Timeout != 90*time.Minute {
		t.Fatalf("deployment client timeout = %s, want 90m", client.http.Timeout)
	}
}

func TestDeploymentTimeoutHierarchyPreservesReversibleGate(t *testing.T) {
	engineGuard := time.Duration(bkc.GLM53FlashNVFP4GuardSeconds) * time.Second
	if deployments.DefaultReadinessTimeout >= engineGuard {
		t.Fatalf("readiness %s must expire before engine guard %s", deployments.DefaultReadinessTimeout, engineGuard)
	}
	minimumClientBudget := 2*deployments.DefaultImagePullRPCTimeout +
		2*deployments.DefaultMemberMutationRPCTimeout +
		deployments.DefaultReadinessTimeout +
		deployments.DefaultCleanupTimeout +
		10*time.Minute
	if deployments.DefaultClientRequestTimeout <= minimumClientBudget {
		t.Fatalf("client timeout %s must exceed successful canary envelope %s", deployments.DefaultClientRequestTimeout, minimumClientBudget)
	}
}

func TestBuildDeploymentCreateRequestRejectsDuplicateDevice(t *testing.T) {
	_, err := buildDeploymentCreateRequest(deploymentCreateFlags{
		bkcID: "recipe", key: "key", headDevice: "spark", workerDevice: "spark",
		headFabric: "192.168.201.1", headServiceAddress: "100.96.0.20", headServicePort: "8000", workerFabric: "192.168.201.2", apiKeyEnv: "YOKAI_TEST_KEY",
	}, func(string) string { return "secret" })
	if err == nil {
		t.Fatal("expected duplicate-device rejection")
	}
}

func TestBuildDeploymentCreateRequestReadsAPIKeyAtRequestTime(t *testing.T) {
	request, err := buildDeploymentCreateRequest(deploymentCreateFlags{
		bkcID: "recipe", key: "key", headDevice: "spark-a", workerDevice: "spark-b",
		headFabric: "192.168.201.1", headServiceAddress: "100.96.0.20", headServicePort: "8000", workerFabric: "192.168.201.2", apiKeyEnv: "YOKAI_TEST_KEY",
	}, func(name string) string {
		if name == "YOKAI_TEST_KEY" {
			return "request-secret"
		}
		return ""
	})
	if err != nil || request.APIKey != "request-secret" || request.Bindings[0].Role != "head" || request.Bindings[0].ServiceAddress != "100.96.0.20" || request.Bindings[0].ServicePort != 8000 {
		t.Fatalf("unexpected request: %#v err=%v", request, err)
	}
}

func TestBuildDeploymentCreateRequestRejectsBroadHeadServiceAddress(t *testing.T) {
	_, err := buildDeploymentCreateRequest(deploymentCreateFlags{
		bkcID: "recipe", key: "key", headDevice: "spark-a", workerDevice: "spark-b",
		headFabric: "192.168.201.1", headServiceAddress: "0.0.0.0", headServicePort: "8000", workerFabric: "192.168.201.2", apiKeyEnv: "YOKAI_TEST_KEY",
	}, func(string) string { return "secret" })
	if err == nil {
		t.Fatal("expected broad head service address rejection")
	}
}

func TestBuildDeploymentCreateRequestCarriesQwen38FabricBinding(t *testing.T) {
	request, err := buildDeploymentCreateRequest(deploymentCreateFlags{
		bkcID: bkc.Qwen38FlashNextNVFP4DualGB10ID, key: "key", headDevice: "spark-a", workerDevice: "spark-b",
		headFabric: "192.168.201.1", headFabricInterface: "enP2p1s0f0np0", headFabricHCA: "roceP2p1s0f0", headFabricGID: "3",
		headServiceAddress: "100.96.0.20", headServicePort: "8888", workerFabric: "192.168.201.2",
		workerFabricInterface: "enP2p1s0f0np0", workerFabricHCA: "roceP2p1s0f0", workerFabricGID: "3", apiKeyEnv: "YOKAI_TEST_KEY",
		localModelPath: "/srv/hf/hub/models--nvidia--Qwen3.8-Flash-Next-NVFP4/snapshots/fc694b54fb0174e0913e6adf86691ef85a4ead47",
	}, func(string) string { return "secret" })
	if err != nil {
		t.Fatalf("build Qwen3.8 request: %v", err)
	}
	if request.Bindings[0].FabricInterface != "enP2p1s0f0np0" || request.Bindings[0].FabricHCA != "roceP2p1s0f0" || request.Bindings[0].FabricGIDIndex == nil || *request.Bindings[0].FabricGIDIndex != 3 || request.Bindings[1].FabricGIDIndex == nil || *request.Bindings[1].FabricGIDIndex != 3 {
		t.Fatalf("unexpected fabric bindings: %#v", request.Bindings)
	}
}

func TestBuildDeploymentCreateRequestRejectsIncompleteQwen38FabricBinding(t *testing.T) {
	_, err := buildDeploymentCreateRequest(deploymentCreateFlags{
		bkcID: bkc.Qwen38FlashNextNVFP4DualGB10ID, key: "key", headDevice: "spark-a", workerDevice: "spark-b",
		headFabric: "192.168.201.1", headFabricInterface: "enP2p1s0f0np0", headFabricHCA: "roceP2p1s0f0", headFabricGID: "3",
		headServiceAddress: "100.96.0.20", headServicePort: "8888", workerFabric: "192.168.201.2",
		workerFabricInterface: "enP2p1s0f0np0", workerFabricHCA: "roceP2p1s0f0", apiKeyEnv: "YOKAI_TEST_KEY",
	}, func(string) string { return "secret" })
	if err == nil || !strings.Contains(err.Error(), "--worker-fabric-gid-index") {
		t.Fatalf("expected missing worker GID rejection, got %v", err)
	}
}

func TestBuildDeploymentTestRequestReadsAPIKeyOnlyAtRequestTime(t *testing.T) {
	request, err := buildDeploymentTestRequest("YOKAI_TEST_KEY", func(name string) string {
		if name == "YOKAI_TEST_KEY" {
			return "transient-test-secret"
		}
		return ""
	})
	if err != nil || request.APIKey != "transient-test-secret" {
		t.Fatalf("unexpected deployment test request: %#v err=%v", request, err)
	}
}

func TestBuildDeploymentTestRequestRejectsMissingOrEmptyAPIKey(t *testing.T) {
	if _, err := buildDeploymentTestRequest("", func(string) string { return "secret" }); err == nil {
		t.Fatal("expected missing API key environment name to fail")
	}
	if _, err := buildDeploymentTestRequest("YOKAI_TEST_KEY", func(string) string { return "" }); err == nil {
		t.Fatal("expected empty API key environment value to fail")
	}
}
