package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spencerbull/yokai/internal/deployments"
)

func TestCaptureContainerLogTailUsesStrictLineLimitAndSanitizes(t *testing.T) {
	const sentinel = "exact-request-key"
	requestedLines := 0
	capture, err := captureContainerLogTail(context.Background(), "candidate", sentinel, func(_ context.Context, id string, lines int) (dockerLogTailOutput, error) {
		if id != "candidate" {
			t.Fatalf("unexpected container selector %q", id)
		}
		requestedLines = lines
		return dockerLogTailOutput{Data: []byte("scheduler timeout " + sentinel + "\nargv --api-key=generic-secret\x00\n")}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestedLines != deployments.MaxLogTailLines {
		t.Fatalf("requested %d log lines, want %d", requestedLines, deployments.MaxLogTailLines)
	}
	if strings.Contains(capture.Tail, sentinel) || strings.Contains(capture.Tail, "generic-secret") || strings.ContainsRune(capture.Tail, '\x00') {
		t.Fatalf("unsafe log tail: %q", capture.Tail)
	}
	if !strings.Contains(capture.Tail, "scheduler timeout [REDACTED]") {
		t.Fatalf("scheduler failure was not preserved: %q", capture.Tail)
	}
}

func TestRunDockerLogTailIsBoundedAndNonFollowing(t *testing.T) {
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/bin/sh
if [ "$1" = logs ] && [ "$2" = --tail ] && [ "$3" = 2000 ] && [ "$4" = candidate ] && [ -z "$5" ]; then
  printf '%s\n' 'bounded output'
  exit 0
fi
exit 9
`
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := runDockerLogTail(context.Background(), "candidate", deployments.MaxLogTailLines)
	if err != nil || string(output.Data) != "bounded output\n" || output.Truncated {
		t.Fatalf("docker logs invocation was not exact: output=%#v err=%v", output, err)
	}
}

func TestBoundedTailWriterDrainsAndRetainsOnlyMarkedSuffix(t *testing.T) {
	writer := newBoundedTailWriter(deployments.MaxLogTailBytes)
	input := bytes.Repeat([]byte("0123456789abcdef"), deployments.MaxLogTailBytes/4)
	written, err := writer.Write(input)
	if err != nil || written != len(input) {
		t.Fatalf("bounded writer did not drain input: written=%d len=%d err=%v", written, len(input), err)
	}
	output := writer.output()
	if !output.Truncated || len(output.Data) != deployments.MaxLogTailBytes {
		t.Fatalf("bounded output mismatch: truncated=%v len=%d", output.Truncated, len(output.Data))
	}
	if cap(writer.buffer) != deployments.MaxLogTailBytes || !bytes.Equal(output.Data, input[len(input)-deployments.MaxLogTailBytes:]) {
		t.Fatalf("bounded writer did not retain the exact capped suffix: buffer_cap=%d", cap(writer.buffer))
	}
}

func TestCaptureContainerLogTailPropagatesRawTruncation(t *testing.T) {
	capture, err := captureContainerLogTail(context.Background(), "candidate", "", func(context.Context, string, int) (dockerLogTailOutput, error) {
		return dockerLogTailOutput{Data: []byte("partial line\nsuffix"), Truncated: true}, nil
	})
	if err != nil || capture.Tail != "suffix" || !capture.Truncated {
		t.Fatalf("raw truncation was not propagated: capture=%#v err=%v", capture, err)
	}
}

func TestRunDockerLogTailReturnsSanitizedPartialOutputOnNonzeroExit(t *testing.T) {
	const sentinel = "exact-request-key"
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/bin/sh
if [ "$1" = logs ] && [ "$2" = --tail ] && [ "$3" = 2000 ] && [ "$4" = candidate ] && [ -z "$5" ]; then
  printf '%s\n' 'scheduler failed exact-request-key'
  printf '%s\n' 'argv --api-key=generic-secret' >&2
  exit 7
fi
exit 9
`
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	capture, err := captureContainerLogTail(context.Background(), "candidate", sentinel, runDockerLogTail)
	if err != nil {
		t.Fatalf("drained partial output was discarded: %v", err)
	}
	if !capture.Truncated || !strings.Contains(capture.Tail, "scheduler failed [REDACTED]") {
		t.Fatalf("partial capture was not preserved and marked: %#v", capture)
	}
	if strings.Contains(capture.Tail, sentinel) || strings.Contains(capture.Tail, "generic-secret") {
		t.Fatalf("partial capture was not sanitized: %q", capture.Tail)
	}
}

func TestCaptureContainerLogTailRetainsErrorWithoutOutput(t *testing.T) {
	wantErr := errors.New("docker logs exited 7")
	capture, err := captureContainerLogTail(context.Background(), "candidate", "", func(context.Context, string, int) (dockerLogTailOutput, error) {
		return dockerLogTailOutput{}, wantErr
	})
	if !errors.Is(err, wantErr) || capture != (deployments.LogTailCapture{}) {
		t.Fatalf("empty failed capture did not retain error: capture=%#v err=%v", capture, err)
	}
}

func TestCaptureContainerLogTailDropsSecretFragmentAtRawBoundary(t *testing.T) {
	const sentinel = "exact-boundary-sentinel"
	const split = 7
	afterSecret := deployments.MaxLogTailBytes - (len(sentinel) - split) - 1
	raw := append([]byte(sentinel+"\n"), bytes.Repeat([]byte("z"), afterSecret)...)
	writer := newBoundedTailWriter(deployments.MaxLogTailBytes)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	output := writer.output()
	if !bytes.HasPrefix(output.Data, []byte(sentinel[split:])) {
		t.Fatalf("fixture did not split the sentinel at the raw boundary: %q", output.Data[:len(sentinel)-split])
	}
	capture, err := captureContainerLogTail(context.Background(), "candidate", sentinel, func(context.Context, string, int) (dockerLogTailOutput, error) {
		return output, errors.New("docker logs exited after truncated output")
	})
	if err != nil || !capture.Truncated || capture.Tail == "" || strings.Contains(capture.Tail, sentinel[split:]) {
		t.Fatalf("boundary secret fragment survived: capture=%#v err=%v", capture, err)
	}
}
