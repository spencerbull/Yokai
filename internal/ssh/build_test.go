package ssh

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeTargetOS(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "Linux", want: "linux"},
		{input: "darwin", want: "darwin"},
		{input: "windows", wantErr: true},
	}

	for _, tt := range tests {
		got, err := normalizeTargetOS(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("normalizeTargetOS(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalizeTargetOS(%q) unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("normalizeTargetOS(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeTargetArch(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "x86_64", want: "amd64"},
		{input: "amd64", want: "amd64"},
		{input: "aarch64", want: "arm64"},
		{input: "arm64", want: "arm64"},
		{input: "armv7", wantErr: true},
	}

	for _, tt := range tests {
		got, err := normalizeTargetArch(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("normalizeTargetArch(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalizeTargetArch(%q) unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("normalizeTargetArch(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLocalGoBuildCommandPrefersPinnedMiseToolchain(t *testing.T) {
	binDir := t.TempDir()
	misePath := filepath.Join(binDir, "mise")
	goPath := filepath.Join(binDir, "go")
	for _, path := range []string{misePath, goPath} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("writing fake executable: %v", err)
		}
	}
	t.Setenv("PATH", binDir)

	cmd, err := localGoBuildCommand("/tmp/yokai-test")
	if err != nil {
		t.Fatalf("localGoBuildCommand() unexpected error: %v", err)
	}
	if cmd.Path != misePath {
		t.Fatalf("command path = %q, want %q", cmd.Path, misePath)
	}
	wantArgs := []string{misePath, "exec", "go@" + defaultGoVersion, "--", "go", "build", "-o", "/tmp/yokai-test", "./cmd/yokai"}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", cmd.Args, wantArgs)
	}
}

func TestLocalGoBuildCommandFallsBackToGo(t *testing.T) {
	binDir := t.TempDir()
	goPath := filepath.Join(binDir, "go")
	if err := os.WriteFile(goPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("writing fake go executable: %v", err)
	}
	t.Setenv("PATH", binDir)

	cmd, err := localGoBuildCommand("/tmp/yokai-test")
	if err != nil {
		t.Fatalf("localGoBuildCommand() unexpected error: %v", err)
	}
	if cmd.Path != goPath {
		t.Fatalf("command path = %q, want %q", cmd.Path, goPath)
	}
	wantArgs := []string{goPath, "build", "-o", "/tmp/yokai-test", "./cmd/yokai"}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", cmd.Args, wantArgs)
	}
}
