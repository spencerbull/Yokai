package cli

import (
	"encoding/json"
	"fmt"
	"os"
)

// outputJSON writes a JSON value to stdout, pretty-printed.
func outputJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		exitError(fmt.Sprintf("encoding output: %v", err))
	}
}

// outputRaw writes raw JSON bytes to stdout (already formatted from daemon).
func outputRaw(data json.RawMessage) {
	// Re-indent for consistent formatting
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		// Fall back to raw output
		_, _ = fmt.Fprintln(os.Stdout, string(data))
		return
	}
	outputJSON(v)
}

// exitError writes a JSON error to stderr and exits with code 1.
func exitError(msg string) {
	enc := json.NewEncoder(os.Stderr)
	_ = enc.Encode(map[string]string{"error": msg})
	os.Exit(1)
}
