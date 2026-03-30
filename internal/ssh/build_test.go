package ssh

import "testing"

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
