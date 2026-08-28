package deployments

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const redactedLogValue = "[REDACTED]"

var (
	apiKeyEqualsPattern     = regexp.MustCompile(`(?i)--api[-_]key[ \t]*=[ \t]*("[^"\r\n]*"|'[^'\r\n]*'|[^\s,;\]\}]+)`)
	apiKeySpacePattern      = regexp.MustCompile(`(?i)--api[-_]key[ \t]+("[^"\r\n]*"|'[^'\r\n]*'|[^\s,;\]\}]+)`)
	apiKeyAssignmentPattern = regexp.MustCompile(`(?i)\bapi_key[ \t]*=[ \t]*("[^"\r\n]*"|'[^'\r\n]*'|[^\s,;\]\}]+)`)
	apiKeyJSONPattern       = regexp.MustCompile(`(?i)"api_key"[ \t]*:[ \t]*"(?:\\.|[^"\\\r\n])*"`)
)

// SanitizeLogTail removes credentials and unsafe control data, then retains
// only the bounded suffix suitable for the durable deployment record. Exact
// request credentials are transient input and never appear in the result.
func SanitizeLogTail(raw, exactAPIKey string) (string, bool) {
	clean := strings.ToValidUTF8(raw, "\uFFFD")
	clean = strings.ReplaceAll(clean, "\r\n", "\n")
	clean = strings.ReplaceAll(clean, "\r", "\n")

	var safe strings.Builder
	safe.Grow(len(clean))
	for _, r := range clean {
		switch {
		case r == '\n' || r == '\t':
			safe.WriteRune(r)
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			continue
		default:
			safe.WriteRune(r)
		}
	}
	clean = safe.String()
	if exactAPIKey != "" {
		clean = strings.ReplaceAll(clean, exactAPIKey, redactedLogValue)
	}
	clean = apiKeyEqualsPattern.ReplaceAllString(clean, "--api-key="+redactedLogValue)
	clean = apiKeySpacePattern.ReplaceAllString(clean, "--api-key "+redactedLogValue)
	clean = apiKeyAssignmentPattern.ReplaceAllString(clean, "api_key="+redactedLogValue)
	clean = apiKeyJSONPattern.ReplaceAllString(clean, `"api_key":"`+redactedLogValue+`"`)

	var truncated bool
	clean, truncated = keepLastLogLines(clean, MaxLogTailLines)
	if len(clean) > MaxLogTailBytes {
		start := len(clean) - MaxLogTailBytes
		for start < len(clean) && !utf8.RuneStart(clean[start]) {
			start++
		}
		clean = clean[start:]
		truncated = true
	}
	return clean, truncated
}

func keepLastLogLines(value string, maximum int) (string, bool) {
	if maximum <= 0 {
		return "", value != ""
	}
	lines := 0
	end := len(value) - 1
	if end >= 0 && value[end] == '\n' {
		end--
	}
	for index := end; index >= 0; index-- {
		if value[index] != '\n' {
			continue
		}
		lines++
		if lines == maximum {
			return value[index+1:], true
		}
	}
	return value, false
}
