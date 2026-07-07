package agent

import "testing"

func TestSanitizeRepoPathAccepts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"model.gguf", "model.gguf"},
		{"Q4_K_M/model-00001-of-00002.gguf", "Q4_K_M/model-00001-of-00002.gguf"},
		{"nested/dir/model.gguf", "nested/dir/model.gguf"},
		{"  Q4_K_M/model.gguf  ", "Q4_K_M/model.gguf"},
		{"./model.gguf", "model.gguf"},
	}
	for _, tc := range cases {
		got, err := sanitizeRepoPath(tc.in)
		if err != nil {
			t.Errorf("sanitizeRepoPath(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("sanitizeRepoPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSanitizeRepoPathRejects(t *testing.T) {
	t.Parallel()

	cases := []string{
		"/etc/passwd",
		"../secret",
		"Q4/../../../etc/passwd",
		"..",
		"foo\\bar.gguf",
	}
	for _, in := range cases {
		if _, err := sanitizeRepoPath(in); err == nil {
			t.Errorf("sanitizeRepoPath(%q) expected error, got none", in)
		}
	}
}

func TestSanitizeRepoPathEmpty(t *testing.T) {
	t.Parallel()

	got, err := sanitizeRepoPath("   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty result for whitespace input, got %q", got)
	}
}
