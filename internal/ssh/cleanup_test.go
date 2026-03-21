package ssh

import (
	"errors"
	"strings"
	"testing"
)

func TestCleanupDeviceSystemServiceRequiresSudo(t *testing.T) {
	t.Parallel()

	fake := &fakeRemoteClient{rules: []fakeExecRule{
		{contains: "systemctl is-active --quiet yokai-agent", out: "", err: nil},
		{contains: "id -u", out: "1000\n", err: nil},
		{contains: "sudo -n true", out: "sudo: a password is required", err: errors.New("exit status 1")},
	}}

	err := cleanupDevice(fake, CleanupOptions{RemoveDockerImages: true})
	if err == nil {
		t.Fatal("expected cleanup to fail without sudo")
	}
	if !strings.Contains(err.Error(), "sudo is required") {
		t.Fatalf("expected sudo guidance, got: %v", err)
	}
	if hasExecuted(fake.cmds, "docker ps -aq --filter \"name=yokai-\"") {
		t.Fatal("docker cleanup should not run when sudo check fails")
	}
}

func TestCleanupDeviceWithImageRemoval(t *testing.T) {
	t.Parallel()

	fake := &fakeRemoteClient{rules: []fakeExecRule{
		{contains: "systemctl is-active --quiet yokai-agent", out: "", err: errors.New("inactive")},
		{contains: "systemctl list-unit-files yokai-agent.service", out: "", err: errors.New("not found")},
		{contains: "id -u", out: "1000\n", err: nil},
	}}

	err := cleanupDevice(fake, CleanupOptions{RemoveDockerImages: true})
	if err != nil {
		t.Fatalf("cleanupDevice returned error: %v", err)
	}

	if !hasExecuted(fake.cmds, "rm -rf ~/.config/yokai") {
		t.Fatal("expected user config cleanup command")
	}
	if !hasExecuted(fake.cmds, "docker ps -aq --filter \"name=yokai-\"") {
		t.Fatal("expected yokai container removal command")
	}
	if !hasExecuted(fake.cmds, "docker rmi") {
		t.Fatal("expected docker image removal attempts when enabled")
	}
	if !hasExecuted(fake.cmds, "rm -rf /tmp/yokai-monitoring") {
		t.Fatal("expected monitoring temp cleanup command")
	}
}

func TestCleanupDeviceWithoutImageRemoval(t *testing.T) {
	t.Parallel()

	fake := &fakeRemoteClient{rules: []fakeExecRule{
		{contains: "systemctl is-active --quiet yokai-agent", out: "", err: errors.New("inactive")},
		{contains: "systemctl list-unit-files yokai-agent.service", out: "", err: errors.New("not found")},
		{contains: "id -u", out: "1000\n", err: nil},
	}}

	err := cleanupDevice(fake, CleanupOptions{RemoveDockerImages: false})
	if err != nil {
		t.Fatalf("cleanupDevice returned error: %v", err)
	}

	if !hasExecuted(fake.cmds, "docker ps -aq --filter \"name=yokai-\"") {
		t.Fatal("expected yokai container removal command")
	}
	if hasExecuted(fake.cmds, "docker rmi") {
		t.Fatal("did not expect docker image removal attempts when disabled")
	}
}
