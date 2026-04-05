package hfmem

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildCommandArgsFallsBackToUvx(t *testing.T) {
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	lookPath = func(file string) (string, error) {
		switch file {
		case "hf-mem":
			return "", errNotFound{}
		case "uvx":
			return "/usr/bin/uvx", nil
		default:
			return "", errNotFound{}
		}
	}

	args, err := buildCommandArgs("meta-llama/Llama-3.1-8B-Instruct", "hf_x", 32768, "")
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != "uvx" || args[1] != "hf-mem" {
		t.Fatalf("expected uvx fallback, got %v", args)
	}
	if args[len(args)-2] != "--hf-token" || args[len(args)-1] != "hf_x" {
		t.Fatalf("expected hf token in args, got %v", args)
	}
}

func TestParseJSONInt(t *testing.T) {
	v, err := parseJSONInt(float64(123))
	if err != nil || v != 123 {
		t.Fatalf("expected 123, got %d err=%v", v, err)
	}
	v, err = parseJSONInt(nil)
	if err != nil || v != 0 {
		t.Fatalf("expected 0 for nil, got %d err=%v", v, err)
	}
}

func TestFormatRunErrorAuthFailures(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   string
	}{
		{name: "401", stderr: "HTTPStatusError: Client error '401 Unauthorized' for url 'https://huggingface.co/foo'", want: "401 Unauthorized"},
		{name: "403", stderr: "HTTPStatusError: Client error '403 Forbidden' for url 'https://huggingface.co/foo'", want: "403 Forbidden"},
		{name: "404", stderr: "HTTPStatusError: Client error '404 Not Found' for url 'https://huggingface.co/foo'", want: "404 Not Found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := formatRunError(errors.New("exit status 1"), tt.stderr)
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q in error, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestFormatRunErrorFallsBackToLastStderrLine(t *testing.T) {
	err := formatRunError(errors.New("exit status 1"), "traceback line\nfinal useful line\n")
	if got := err.Error(); got != "running hf-mem: final useful line" {
		t.Fatalf("unexpected error: %q", got)
	}
}

type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }
