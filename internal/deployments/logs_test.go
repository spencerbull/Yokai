package deployments

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeLogTailRedactsExactAndGenericAPIKeys(t *testing.T) {
	const sentinel = "exact-sentinel-api-key"
	raw := strings.Join([]string{
		"scheduler failed with " + sentinel,
		"argv --api-key=generic-equals --model glm",
		"argv --api-key generic-space --port 8000",
		"argv --API_KEY='generic_underscore'",
		`sglang api_key='sglang-single-quoted'`,
		`sglang api_key="sglang-double-quoted"`,
		`{"api_key":"json-secret","status":"failed"}`,
		"bad\x00control\x01data",
	}, "\r\n") + string([]byte{0xff})
	clean, truncated := SanitizeLogTail(raw, sentinel)
	if truncated {
		t.Fatal("small log unexpectedly truncated")
	}
	for _, secret := range []string{sentinel, "generic-equals", "generic-space", "generic_underscore", "sglang-single-quoted", "sglang-double-quoted", "json-secret"} {
		if strings.Contains(clean, secret) {
			t.Fatalf("secret survived sanitization: %q in %q", secret, clean)
		}
	}
	if !strings.Contains(clean, "scheduler failed with [REDACTED]") || strings.ContainsAny(clean, "\x00\x01\r") || !utf8.ValidString(clean) {
		t.Fatalf("unexpected sanitized log: %q", clean)
	}
}

func TestSanitizeLogTailCapsLinesAndBytesAfterRedaction(t *testing.T) {
	var lines strings.Builder
	for index := 0; index < MaxLogTailLines+5; index++ {
		lines.WriteString("line\n")
	}
	clean, truncated := SanitizeLogTail(lines.String(), "")
	if !truncated || strings.Count(clean, "\n") != MaxLogTailLines {
		t.Fatalf("line cap mismatch: truncated=%v lines=%d", truncated, strings.Count(clean, "\n"))
	}

	clean, truncated = SanitizeLogTail(strings.Repeat("é", MaxLogTailBytes), "")
	if !truncated || len(clean) > MaxLogTailBytes || !utf8.ValidString(clean) {
		t.Fatalf("byte cap mismatch: truncated=%v bytes=%d valid=%v", truncated, len(clean), utf8.ValidString(clean))
	}
}
