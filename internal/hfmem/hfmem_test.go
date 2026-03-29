package hfmem

import "testing"

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

	args, err := buildCommandArgs("meta-llama/Llama-3.1-8B-Instruct", "hf_x", 32768)
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

type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }
