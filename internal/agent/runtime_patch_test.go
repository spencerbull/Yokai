package agent

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/deployments"
)

func pinnedRuntimePatchRequest() ContainerRequest {
	return ContainerRequest{
		Image: bkc.GLM53FlashNVFP4Image,
		Model: bkc.GLM53FlashNVFP4Model,
		ExtraArgs: strings.Join([]string{
			"sglang", "serve", "--model-path", bkc.GLM53FlashNVFP4Model,
			"--revision", bkc.GLM53FlashNVFP4Revision, "--tp-size", "2", "--host", "100.96.0.20", "--port", "8000",
		}, " "),
		Args: []string{"--api-key=sentinel"},
		Labels: map[string]string{
			LabelBKCID:         bkc.GLM53FlashNVFP4DualGB10ID,
			LabelModelRevision: bkc.GLM53FlashNVFP4Revision,
			LabelImageDigest:   bkc.GLM53FlashNVFP4ImageDigest,
			LabelRuntimePatch:  bkc.GLM53FlashRuntimePatchSetLabel(),
		},
	}
}

func TestGLM53FlashRuntimePatchSpecsMatchBKCLabelSet(t *testing.T) {
	cfg, ok := bkc.LookupID(bkc.GLM53FlashNVFP4DualGB10ID)
	if !ok || cfg.MultiDevice == nil {
		t.Fatal("pinned GLM-5.3-Flash BKC not found")
	}
	bkcLabels := make([]string, len(cfg.MultiDevice.RuntimePatches))
	for index, patch := range cfg.MultiDevice.RuntimePatches {
		bkcLabels[index] = patch.Label
	}
	specLabels := make([]string, len(glm53FlashRuntimePatchSpecs))
	for index, spec := range glm53FlashRuntimePatchSpecs {
		specLabels[index] = spec.label
	}
	if len(specLabels) != len(bkcLabels) || !reflect.DeepEqual(specLabels, bkcLabels) {
		t.Fatalf("agent runtime patch specs drifted from the ordered BKC patches: agent=%#v bkc=%#v", specLabels, bkcLabels)
	}
	if got := strings.Join(specLabels, "+"); got != bkc.GLM53FlashRuntimePatchSetLabel() {
		t.Fatalf("agent runtime patch labels do not match the BKC composite label: got=%q want=%q", got, bkc.GLM53FlashRuntimePatchSetLabel())
	}
}

func TestPinnedSGLangRuntimePatchWrapsFinalNormalizedArgv(t *testing.T) {
	req := pinnedRuntimePatchRequest()
	originalCommand := append(strings.Fields(req.ExtraArgs), req.Args...)
	if err := applyPinnedSGLangRuntimePatch(&req); err != nil {
		t.Fatal(err)
	}
	if req.ExtraArgs != "" || len(req.Args) < 4 {
		t.Fatalf("wrapper was not rendered as structured argv: extra=%q args=%#v", req.ExtraArgs, req.Args)
	}
	if req.Args[0] != "python3" || req.Args[1] != "-c" || strings.ContainsAny(req.Args[2], "\x00\r\n") {
		t.Fatalf("unexpected bootstrap prefix: %#v", req.Args[:3])
	}
	if strings.Contains(strings.Join(req.Args[:3], " "), "/bin/sh") || strings.Contains(strings.Join(req.Args[:3], " "), "sh -c") {
		t.Fatalf("bootstrap introduced a shell: %#v", req.Args[:3])
	}
	if !reflect.DeepEqual(req.Args[3:], originalCommand) {
		t.Fatalf("normalized command order changed:\n got %#v\nwant %#v", req.Args[3:], originalCommand)
	}
	if argSequenceCount(req.Args[3:], "sglang", "serve") != 1 {
		t.Fatalf("sglang serve command was lost or duplicated: %#v", req.Args)
	}
}

func TestPinnedSGLangRuntimePatchAcceptsCanonicalAndFixedLocalModels(t *testing.T) {
	for _, model := range []string{bkc.GLM53FlashNVFP4Model, deployments.FixedLocalModelPath} {
		t.Run(model, func(t *testing.T) {
			req := pinnedRuntimePatchRequest()
			req.Model = model
			req.ExtraArgs = strings.Replace(req.ExtraArgs, bkc.GLM53FlashNVFP4Model, model, 1)
			if err := applyPinnedSGLangRuntimePatch(&req); err != nil {
				t.Fatalf("authorized model %q was rejected: %v", model, err)
			}
			if modelPath, count := tokenFlagValueCount(req.Args[3:], "--model-path"); count != 1 || modelPath != model {
				t.Fatalf("wrapped model path mismatch: value=%q count=%d args=%#v", modelPath, count, req.Args)
			}
		})
	}
}

func TestPinnedSGLangRuntimePatchProvenanceFailsClosed(t *testing.T) {
	tests := map[string]func(*ContainerRequest){
		"patch label missing": func(req *ContainerRequest) { delete(req.Labels, LabelRuntimePatch) },
		"patch label":         func(req *ContainerRequest) { req.Labels[LabelRuntimePatch] = "other" },
		"bkc id":              func(req *ContainerRequest) { req.Labels[LabelBKCID] = "other" },
		"image":               func(req *ContainerRequest) { req.Image = "lmsysorg/sglang:latest" },
		"model":               func(req *ContainerRequest) { req.Model = "other/model" },
		"model path mismatch": func(req *ContainerRequest) {
			req.ExtraArgs = strings.Replace(req.ExtraArgs, bkc.GLM53FlashNVFP4Model, deployments.FixedLocalModelPath, 1)
		},
		"duplicate model path": func(req *ContainerRequest) {
			req.ExtraArgs += " --model-path " + req.Model
		},
		"image digest":   func(req *ContainerRequest) { req.Labels[LabelImageDigest] = "bad" },
		"model revision": func(req *ContainerRequest) { req.Labels[LabelModelRevision] = "bad" },
		"missing revision": func(req *ContainerRequest) {
			req.ExtraArgs = strings.Replace(req.ExtraArgs, " --revision "+bkc.GLM53FlashNVFP4Revision, "", 1)
		},
		"duplicate revision": func(req *ContainerRequest) {
			req.ExtraArgs += " --revision " + bkc.GLM53FlashNVFP4Revision
		},
		"conflicting revision": func(req *ContainerRequest) { req.ExtraArgs += " --revision other" },
		"missing tp": func(req *ContainerRequest) {
			req.ExtraArgs = strings.Replace(req.ExtraArgs, " --tp-size 2", "", 1)
		},
		"tp drift": func(req *ContainerRequest) {
			req.ExtraArgs = strings.Replace(req.ExtraArgs, "--tp-size 2", "--tp-size 1", 1)
		},
		"duplicate tp":      func(req *ContainerRequest) { req.ExtraArgs += " --tp-size 2" },
		"command prefix":    func(req *ContainerRequest) { req.ExtraArgs = strings.TrimPrefix(req.ExtraArgs, "sglang ") },
		"duplicate command": func(req *ContainerRequest) { req.ExtraArgs += " sglang serve" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			req := pinnedRuntimePatchRequest()
			mutate(&req)
			beforeArgs := append([]string(nil), req.Args...)
			beforeExtra := req.ExtraArgs
			if err := applyPinnedSGLangRuntimePatch(&req); err == nil {
				t.Fatal("expected runtime patch gate to reject drift")
			}
			if req.ExtraArgs != beforeExtra {
				t.Fatalf("gate mutated extra args on failure: %q", req.ExtraArgs)
			}
			if !reflect.DeepEqual(req.Args, beforeArgs) {
				t.Fatalf("gate mutated structured args on failure: %#v", req.Args)
			}
		})
	}
}

func TestRuntimePatchLabelAbsentLeavesNonTargetUnchanged(t *testing.T) {
	req := ContainerRequest{Image: "lmsysorg/sglang:latest", Model: "other/model", ExtraArgs: "sglang serve --revision other", Args: []string{"--api-key=keep"}, Labels: map[string]string{}}
	want := req
	want.Args = append([]string(nil), req.Args...)
	if err := applyPinnedSGLangRuntimePatch(&req); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(req, want) {
		t.Fatalf("non-target request changed: got %#v want %#v", req, want)
	}
}

func TestSGLangRuntimePatchScriptPristinePatchesBothAndExecs(t *testing.T) {
	dir := t.TempDir()
	specs, pristine := prepareExactRuntimePatchFixtures(t, dir, false, false)
	marker := filepath.Join(dir, "exec-pristine")
	output := runPatchBootstrap(t, buildSGLangRuntimePatchScript(specs), marker, "rank", "1")
	wantEvidence := fmt.Sprintf("TP=2 tile=32/1/128 shared-memory required=%d observed=%d", bkc.GLM53FlashTileSharedMemoryBytes, bkc.GLM53FlashTileSharedMemoryBytes)
	if !strings.Contains(string(output), wantEvidence) {
		t.Fatalf("hardware guard evidence missing: %s", output)
	}
	assertFinalRuntimePatchFiles(t, specs)
	for index, spec := range specs {
		got, err := os.ReadFile(spec.path)
		if err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(got, pristine[index]) {
			t.Fatalf("pristine patch %d was not changed", index)
		}
	}
	if got, _ := os.ReadFile(marker); string(got) != "rank|1" {
		t.Fatalf("original command was not execed with exact argv: %q", got)
	}
}

func TestSGLangRuntimePatchScriptAlreadyPatchedDoesNotRewriteAndExecs(t *testing.T) {
	dir := t.TempDir()
	specs, _ := prepareExactRuntimePatchFixtures(t, dir, true, true)
	preservedTime := time.Unix(123456789, 0)
	for _, spec := range specs {
		if err := os.Chtimes(spec.path, preservedTime, preservedTime); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(dir, "exec-patched")
	runPatchBootstrap(t, buildSGLangRuntimePatchScript(specs), marker, "rank", "0")
	for _, spec := range specs {
		info, err := os.Stat(spec.path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(preservedTime) {
			t.Fatalf("already-patched source %s was rewritten: mtime=%s", spec.label, info.ModTime())
		}
	}
	if got, _ := os.ReadFile(marker); string(got) != "rank|0" {
		t.Fatalf("restart command was not execed with exact argv: %q", got)
	}
}

func TestSGLangRuntimePatchScriptMixedStateOnlyPatchesPristineFile(t *testing.T) {
	dir := t.TempDir()
	specs, _ := prepareExactRuntimePatchFixtures(t, dir, true, false)
	preservedTime := time.Unix(123456789, 0)
	for _, spec := range specs {
		if err := os.Chtimes(spec.path, preservedTime, preservedTime); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(dir, "exec-mixed")
	runPatchBootstrap(t, buildSGLangRuntimePatchScript(specs), marker)
	assertFinalRuntimePatchFiles(t, specs)
	loaderInfo, err := os.Stat(specs[0].path)
	if err != nil {
		t.Fatal(err)
	}
	tileInfo, err := os.Stat(specs[1].path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaderInfo.ModTime().Equal(preservedTime) {
		t.Fatal("already-patched loader was rewritten in a mixed state")
	}
	if tileInfo.ModTime().Equal(preservedTime) {
		t.Fatal("pristine TileLang source was not patched in a mixed state")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("mixed-state command did not exec: %v", err)
	}
}

func TestSGLangRuntimePatchScriptReverseMixedStateOnlyPatchesPristineLoader(t *testing.T) {
	dir := t.TempDir()
	specs, _ := prepareExactRuntimePatchFixtures(t, dir, false, true)
	preservedTime := time.Unix(123456789, 0)
	for _, spec := range specs {
		if err := os.Chtimes(spec.path, preservedTime, preservedTime); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(dir, "exec-reverse-mixed")
	runPatchBootstrap(t, buildSGLangRuntimePatchScript(specs), marker)
	assertFinalRuntimePatchFiles(t, specs)
	loaderInfo, err := os.Stat(specs[0].path)
	if err != nil {
		t.Fatal(err)
	}
	tileInfo, err := os.Stat(specs[1].path)
	if err != nil {
		t.Fatal(err)
	}
	if loaderInfo.ModTime().Equal(preservedTime) {
		t.Fatal("pristine loader was not patched in the reverse mixed state")
	}
	if !tileInfo.ModTime().Equal(preservedTime) {
		t.Fatal("already-patched TileLang source was rewritten in the reverse mixed state")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("reverse mixed-state command did not exec: %v", err)
	}
}

func TestSGLangRuntimePatchScriptFirstSpecDriftLeavesBothFilesUntouched(t *testing.T) {
	dir := t.TempDir()
	specs, pristine := prepareExactRuntimePatchFixtures(t, dir, false, false)
	firstDrift := append(append([]byte(nil), pristine[0]...), []byte("# drift\n")...)
	if err := os.WriteFile(specs[0].path, firstDrift, 0600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "must-not-exec")
	if output, err := patchBootstrapCommand(t, buildSGLangRuntimePatchScript(specs), marker, bkc.GLM53FlashTileSharedMemoryBytes).CombinedOutput(); err == nil {
		t.Fatalf("first-spec drift unexpectedly executed command: %s", output)
	}
	for index, want := range [][]byte{firstDrift, pristine[1]} {
		got, err := os.ReadFile(specs[index].path)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("first-spec preflight failure modified source %d", index)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("original command ran after first-spec preflight failure: %v", err)
	}
}

func TestSGLangRuntimePatchScriptSecondSpecFailureLeavesFirstPristine(t *testing.T) {
	tests := map[string]func([]sglangRuntimePatchSpec, []byte) ([]sglangRuntimePatchSpec, []byte){
		"drift": func(specs []sglangRuntimePatchSpec, tile []byte) ([]sglangRuntimePatchSpec, []byte) {
			return specs, append(append([]byte(nil), tile...), []byte("# drift\n")...)
		},
		"duplicate old line": func(specs []sglangRuntimePatchSpec, tile []byte) ([]sglangRuntimePatchSpec, []byte) {
			content := append(append([]byte(nil), tile...), []byte(bkc.GLM53FlashDSAGB10TilePatchOldLine+"\n")...)
			specs[1].originalSHA256 = sha256Hex(content)
			return specs, content
		},
		"missing old line": func(specs []sglangRuntimePatchSpec, tile []byte) ([]sglangRuntimePatchSpec, []byte) {
			content := []byte(strings.Replace(string(tile), bkc.GLM53FlashDSAGB10TilePatchOldLine, "            missing_tile_call", 1))
			specs[1].originalSHA256 = sha256Hex(content)
			return specs, content
		},
		"wrong pristine hash": func(specs []sglangRuntimePatchSpec, tile []byte) ([]sglangRuntimePatchSpec, []byte) {
			specs[1].originalSHA256 = strings.Repeat("0", 64)
			return specs, tile
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			specs, pristine := prepareExactRuntimePatchFixtures(t, dir, false, false)
			specs, secondContent := mutate(specs, pristine[1])
			if err := os.WriteFile(specs[1].path, secondContent, 0600); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(dir, "must-not-exec")
			if output, err := patchBootstrapCommand(t, buildSGLangRuntimePatchScript(specs), marker, bkc.GLM53FlashTileSharedMemoryBytes).CombinedOutput(); err == nil {
				t.Fatalf("second-spec failure unexpectedly executed command: %s", output)
			}
			firstAfter, err := os.ReadFile(specs[0].path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(firstAfter, pristine[0]) {
				t.Fatal("later preflight failure modified the first pristine source")
			}
			secondAfter, err := os.ReadFile(specs[1].path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(secondAfter, secondContent) {
				t.Fatal("rejected second source was modified")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("original command ran after preflight failure: %v", err)
			}
		})
	}
}

func TestSGLangRuntimePatchScriptRejectsFinalVerificationFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shared.py")
	pristine := []byte("alpha\n")
	if err := os.WriteFile(path, pristine, 0600); err != nil {
		t.Fatal(err)
	}
	specs := []sglangRuntimePatchSpec{
		{label: "first", path: path, originalLine: "alpha", replacementLine: "first", originalSHA256: sha256Hex(pristine), patchedSHA256: sha256Hex([]byte("first\n"))},
		{label: "second", path: path, originalLine: "alpha", replacementLine: "second", originalSHA256: sha256Hex(pristine), patchedSHA256: sha256Hex([]byte("second\n"))},
	}
	marker := filepath.Join(dir, "must-not-exec")
	output, err := patchBootstrapCommand(t, buildSGLangRuntimePatchScript(specs), marker, bkc.GLM53FlashTileSharedMemoryBytes).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "runtime patch verification failed: first") {
		t.Fatalf("injected final verification failure was not rejected: err=%v output=%s", err, output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("command ran after final verification failure: %v", err)
	}
}

func TestSGLangRuntimePatchScriptRejectsInsufficientSharedMemoryBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	specs, pristine := prepareExactRuntimePatchFixtures(t, dir, false, false)
	marker := filepath.Join(dir, "must-not-exec")
	output, err := patchBootstrapCommand(t, buildSGLangRuntimePatchScript(specs), marker, bkc.GLM53FlashTileSharedMemoryBytes-1).CombinedOutput()
	wantLine := fmt.Sprintf("GLM-5.3-Flash TP=2 tile=32/1/128 shared-memory required=%d observed=%d\n", bkc.GLM53FlashTileSharedMemoryBytes, bkc.GLM53FlashTileSharedMemoryBytes-1)
	if err == nil || string(output) != wantLine {
		t.Fatalf("hardware guard did not fail concisely: err=%v output=%q", err, output)
	}
	for index, spec := range specs {
		got, readErr := os.ReadFile(spec.path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !reflect.DeepEqual(got, pristine[index]) {
			t.Fatalf("hardware guard modified pristine patch %d", index)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("command ran with insufficient shared memory: %v", err)
	}
}

func TestSGLangRuntimePatchScriptRejectsUnreadableSharedMemoryBeforeWriting(t *testing.T) {
	tests := []struct {
		name       string
		torch      string
		exactError string
		contains   string
	}{
		{
			name:       "missing attribute",
			torch:      "class Properties:\n    pass\nclass CUDA:\n    def get_device_properties(self, index):\n        assert index == 0\n        return Properties()\ncuda = CUDA()\n",
			exactError: "unable to read the GB10 opt-in shared-memory limit\n",
		},
		{
			name:       "non-integer attribute",
			torch:      "class Properties:\n    shared_memory_per_block_optin = '100352'\nclass CUDA:\n    def get_device_properties(self, index):\n        assert index == 0\n        return Properties()\ncuda = CUDA()\n",
			exactError: "unable to read the GB10 opt-in shared-memory limit\n",
		},
		{
			name:     "device property probe raises",
			torch:    "class CUDA:\n    def get_device_properties(self, index):\n        raise RuntimeError('device property probe failed')\ncuda = CUDA()\n",
			contains: "device property probe failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			specs, pristine := prepareExactRuntimePatchFixtures(t, dir, false, false)
			marker := filepath.Join(dir, "must-not-exec")
			output, err := patchBootstrapCommandWithTorch(t, buildSGLangRuntimePatchScript(specs), marker, test.torch).CombinedOutput()
			if err == nil {
				t.Fatalf("unreadable shared-memory property unexpectedly executed command: %s", output)
			}
			if test.exactError != "" && string(output) != test.exactError {
				t.Fatalf("shared-memory property failure was not concise: got=%q want=%q", output, test.exactError)
			}
			if test.contains != "" && !strings.Contains(string(output), test.contains) {
				t.Fatalf("device property probe failure was not reported: %s", output)
			}
			for index, spec := range specs {
				got, readErr := os.ReadFile(spec.path)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if !reflect.DeepEqual(got, pristine[index]) {
					t.Fatalf("unreadable shared-memory property modified pristine patch %d", index)
				}
			}
			if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
				t.Fatalf("command ran with unreadable shared-memory property: %v", statErr)
			}
		})
	}
}

func prepareExactRuntimePatchFixtures(t *testing.T, dir string, loaderPatched, tilePatched bool) ([]sglangRuntimePatchSpec, [][]byte) {
	t.Helper()
	fixturePaths := []string{"testdata/glm53_load_model_utils.py", "testdata/glm53_tilelang_kernel.py"}
	specs := append([]sglangRuntimePatchSpec(nil), glm53FlashRuntimePatchSpecs...)
	pristine := make([][]byte, len(specs))
	patchedStates := []bool{loaderPatched, tilePatched}
	for index := range specs {
		content, err := os.ReadFile(fixturePaths[index])
		if err != nil {
			t.Fatal(err)
		}
		if got := sha256Hex(content); got != specs[index].originalSHA256 {
			t.Fatalf("fixture %s source hash drifted: %s", specs[index].label, got)
		}
		if count := exactLineCount(content, specs[index].originalLine); count != 1 {
			t.Fatalf("fixture %s old-line count drifted: %d", specs[index].label, count)
		}
		if count := exactLineCount(content, specs[index].replacementLine); count != 0 {
			t.Fatalf("fixture %s unexpectedly contains patched line: %d", specs[index].label, count)
		}
		pristine[index] = append([]byte(nil), content...)
		if patchedStates[index] {
			content = []byte(strings.Replace(string(content), specs[index].originalLine, specs[index].replacementLine, 1))
			if got := sha256Hex(content); got != specs[index].patchedSHA256 {
				t.Fatalf("fixture %s patched hash drifted: %s", specs[index].label, got)
			}
		}
		specs[index].path = filepath.Join(dir, filepath.Base(specs[index].path))
		if err := os.WriteFile(specs[index].path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return specs, pristine
}

func assertFinalRuntimePatchFiles(t *testing.T, specs []sglangRuntimePatchSpec) {
	t.Helper()
	for _, spec := range specs {
		content, err := os.ReadFile(spec.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := sha256Hex(content); got != spec.patchedSHA256 || exactLineCount(content, spec.replacementLine) != 1 || exactLineCount(content, spec.originalLine) != 0 {
			t.Fatalf("final patch verification mismatch for %s: hash=%s", spec.label, got)
		}
	}
}

func sha256Hex(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}

func exactLineCount(content []byte, line string) int {
	count := 0
	for _, candidate := range strings.Split(string(content), "\n") {
		if candidate == line {
			count++
		}
	}
	return count
}

func runPatchBootstrap(t *testing.T, script, marker string, args ...string) []byte {
	t.Helper()
	output, err := patchBootstrapCommand(t, script, marker, bkc.GLM53FlashTileSharedMemoryBytes, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrap failed: %v\n%s", err, output)
	}
	return output
}

func patchBootstrapCommand(t *testing.T, script, marker string, sharedMemory int, args ...string) *exec.Cmd {
	t.Helper()
	torchModule := fmt.Sprintf("class Properties:\n    shared_memory_per_block_optin = %d\nclass CUDA:\n    def get_device_properties(self, index):\n        assert index == 0\n        return Properties()\ncuda = CUDA()\n", sharedMemory)
	return patchBootstrapCommandWithTorch(t, script, marker, torchModule, args...)
}

func patchBootstrapCommandWithTorch(t *testing.T, script, marker, torchModule string, args ...string) *exec.Cmd {
	t.Helper()
	markerScript := "import pathlib,sys;pathlib.Path(" + strconv.Quote(marker) + ").write_text('|'.join(sys.argv[1:]))"
	command := []string{"-c", script, "python3", "-c", markerScript}
	command = append(command, args...)
	cmd := exec.Command("python3", command...)
	fakeModules := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeModules, "torch.py"), []byte(torchModule), 0600); err != nil {
		t.Fatal(err)
	}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PYTHONPATH=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "PYTHONPATH="+fakeModules)
	return cmd
}
