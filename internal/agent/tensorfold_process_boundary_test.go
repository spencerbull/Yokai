package agent

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Exercise the OS waiter, not a fake whose Kill returns a successful Wait.
func TestTensorFoldIntentionalKillIsSuccessfulJoin(t *testing.T) {
	root := t.TempDir()
	m := newTensorFoldManager(root, execTensorFoldRunner{})
	log, err := os.CreateTemp(root, "process-log-")
	if err != nil {
		t.Fatal(err)
	}
	p, err := m.runner.Start(context.Background(), root, m.commandEnvironment(false), io.Discard, "/bin/sleep", "60")
	if err != nil {
		t.Fatal(err)
	}
	launch := &tensorFoldLaunch{id: strings.Repeat("a", 64), process: p, done: make(chan struct{})}
	resource := tensorFoldResource{Name: "yokai-deployment-test-g1-head", LaunchID: launch.id}
	m.mu.Lock()
	m.processes[resource.Name] = launch
	go m.waitForProcess(resource.Name, launch, log)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = m.joinTensorFoldLaunchLocked(ctx, resource)
	m.mu.Unlock()
	if err != nil {
		t.Fatalf("intentional process-group kill must reap successfully, got %v", err)
	}
}

type tensorFoldShellTransport struct{ fakeTensorFoldRunner }

func (r *tensorFoldShellTransport) Run(ctx context.Context, _ string, env []string, name string, args ...string) ([]byte, error) {
	if name != "ssh" || len(args) < 8 {
		panic("test transport accepts only simulated SSH")
	}
	// OpenSSH concatenates command arguments and sends one remote shell command.
	// Execute only the test's printf on a local shell; never start SSH or Docker.
	c := exec.CommandContext(ctx, "/bin/sh", "-c", strings.Join(args[7:], " "))
	c.Env = env
	return c.Output()
}
func TestTensorFoldSSHTransportPreservesArgumentBoundaries(t *testing.T) {
	m := newTensorFoldManager(t.TempDir(), &tensorFoldShellTransport{})
	request := validTensorFoldResourceRequest(true)
	resource := tensorFoldResource{WorkerUser: request.WorkerUser, WorkerAddress: request.WorkerAddress}
	want := `{{ index .Config.Labels "tf.patches" }}`
	got, err := m.runTensorFoldNodeCommand(context.Background(), resource, true, "printf", "%s", want)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("SSH changed Docker template argument: got %q, want %q", got, want)
	}
}
