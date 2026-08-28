package agent

import (
	"context"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/deployments"
)

func TestDetachedCandidateLaunchContextOutlivesRequestCancellation(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	launchCtx, cancelLaunch := detachedCandidateLaunchContext(requestCtx, 100*time.Millisecond)
	defer cancelLaunch()
	cancelRequest()
	select {
	case <-launchCtx.Done():
		t.Fatalf("candidate launch inherited request cancellation: %v", launchCtx.Err())
	case <-time.After(10 * time.Millisecond):
	}
}

func TestCandidateLaunchRegistryMakesDeletionWaitForCleanup(t *testing.T) {
	registry := newOperationBarrierRegistry()
	finish, err := registry.begin("candidate")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- registry.wait(context.Background(), "candidate") }()
	select {
	case err := <-done:
		t.Fatalf("deletion barrier returned before launch cleanup: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	finish()
	if err := <-done; err != nil {
		t.Fatalf("deletion barrier failed after cleanup: %v", err)
	}
}

func TestCandidateLaunchTimeoutContractLeavesRPCMargin(t *testing.T) {
	minimum := deployments.DefaultCandidateLaunchCommandTimeout + deployments.DefaultCandidateLaunchSettleTimeout
	if deployments.DefaultCandidateLaunchRPCTimeout <= minimum {
		t.Fatalf("launch RPC timeout %s must exceed command plus settle barrier %s", deployments.DefaultCandidateLaunchRPCTimeout, minimum)
	}
}
