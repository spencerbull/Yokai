package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/plugins"
)

const glmTestImage = "lmsysorg/sglang@sha256:73f9294b78e38d8cc297bfed16daec8ac192b126a2d1fb9055e259a632c68f00"

func TestMultiDeviceDockerRunArgs(t *testing.T) {
	req := ContainerRequest{
		Image:       glmTestImage,
		NetworkMode: "host",
		Ports:       map[string]string{"8000": "8000"},
		Labels: map[string]string{
			"io.yokai.deployment.role": "head",
			LabelServiceAddress:        "100.96.0.20",
			LabelServicePort:           "8000",
			LabelManaged:               "true",
		},
		GPUIDs:  "0",
		Devices: []string{"/dev/infiniband:/dev/infiniband"},
		CapAdd:  []string{"IPC_LOCK"},
		Runtime: config.RuntimeOptions{
			IPCMode:       "host",
			ShmSize:       "32g",
			Ulimits:       map[string]string{"memlock": "-1", "stack": "67108864"},
			RestartPolicy: config.RestartPolicyNo,
		},
		ExtraArgs: "sglang serve --tp-size 2 --node-rank 0 --host 100.96.0.20 --port 8000",
		Args:      []string{"--api-key=-leading-dash-secret"},
	}
	args := buildDockerRunArgs(req, "yokai-test-head")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--network host",
		"--label io.yokai.deployment.role=head",
		"--label io.yokai.service.address=100.96.0.20",
		"--label io.yokai.service.port=8000",
		"--device /dev/infiniband:/dev/infiniband",
		"--cap-add IPC_LOCK",
		"--ipc host",
		"--shm-size 32g",
		"--ulimit memlock=-1",
		"--gpus \"device=0\"",
		"--restart no",
		"--host 100.96.0.20 --port 8000",
		"--api-key=-leading-dash-secret",
		glmTestImage,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker argv missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "0.0.0.0") || strings.Contains(joined, "30000") || strings.Contains(joined, "-p ") {
		t.Fatalf("coordinated host-network argv must preserve the explicit endpoint without port publishing: %s", joined)
	}
}

func TestPinnedRuntimePatchDockerRunArgsPreserveBootstrapAsOneElement(t *testing.T) {
	req := pinnedRuntimePatchRequest()
	originalCommand := append(strings.Fields(req.ExtraArgs), req.Args...)
	if err := applyPinnedSGLangRuntimePatch(&req); err != nil {
		t.Fatal(err)
	}
	args := buildDockerRunArgs(req, "yokai-test-runtime-patch")
	imageIndex := -1
	for index, arg := range args {
		if arg == req.Image {
			imageIndex = index
			break
		}
	}
	if imageIndex < 0 {
		t.Fatalf("docker argv omitted image: %#v", args)
	}
	wantAfterImage := append([]string{"python3", "-c", req.Args[2]}, originalCommand...)
	if !reflect.DeepEqual(args[imageIndex+1:], wantAfterImage) {
		t.Fatalf("wrapped docker command changed:\n got %#v\nwant %#v", args[imageIndex+1:], wantAfterImage)
	}
	scriptCount := 0
	for _, arg := range args {
		if arg == req.Args[2] {
			scriptCount++
		}
	}
	if scriptCount != 1 || args[imageIndex+3] != req.Args[2] || args[imageIndex+4] != "sglang" {
		t.Fatalf("bootstrap script was not one argv element between the image prefix and original command: %#v", args[imageIndex:])
	}
}

func TestRunContainerRejectsRuntimePatchLabelOnNonSGLangBeforeDockerRun(t *testing.T) {
	binDir := t.TempDir()
	runMarker := filepath.Join(binDir, "docker-run")
	dockerPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nif [ \"$1\" = run ]; then : > \"" + runMarker + "\"; fi\nexit 1\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := runContainer(ContainerRequest{
		Image: "example.invalid/other-runtime", Model: bkc.GLM53FlashNVFP4Model, Ports: map[string]string{},
		ExtraArgs: "sglang serve --model-path " + bkc.GLM53FlashNVFP4Model + " --revision " + bkc.GLM53FlashNVFP4Revision + " --tp-size 2",
		Labels: map[string]string{
			LabelBKCID: bkc.GLM53FlashNVFP4DualGB10ID, LabelModelRevision: bkc.GLM53FlashNVFP4Revision,
			LabelImageDigest: bkc.GLM53FlashNVFP4ImageDigest, LabelRuntimePatch: bkc.GLM53FlashRuntimePatchSetLabel(),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "runtime patch provenance") {
		t.Fatalf("mislabeled non-SGLang image was not rejected: %v", err)
	}
	if _, statErr := os.Stat(runMarker); !os.IsNotExist(statErr) {
		t.Fatalf("Docker launch was attempted for a mislabeled non-SGLang image: %v", statErr)
	}
}

func TestLegacyDockerRunArgsPreserveUnlessStoppedDefault(t *testing.T) {
	args := buildDockerRunArgs(ContainerRequest{Image: "example/image", Ports: map[string]string{}}, "yokai-legacy")
	if !strings.Contains(strings.Join(args, " "), "--restart unless-stopped") {
		t.Fatalf("legacy restart default changed: %v", args)
	}
}

func TestFailedRunCleanupRemovesOnlyOwnedAmbiguousCandidate(t *testing.T) {
	request := ContainerRequest{Labels: map[string]string{
		LabelManaged:      "true",
		LabelOwnership:    OwnershipManaged,
		LabelDeploymentID: "dep-test",
		LabelGeneration:   "3",
		LabelRole:         "head",
	}}
	removed := ""
	deps := failedRunCleanupDeps{
		inspect: func(_ context.Context, name string) (string, string, map[string]string, error) {
			return strings.Repeat("a", 64), name, map[string]string{
				LabelManaged:      "true",
				LabelOwnership:    OwnershipManaged,
				LabelDeploymentID: "dep-test",
				LabelGeneration:   "3",
				LabelRole:         "head",
			}, nil
		},
		remove: func(_ context.Context, name string) error {
			removed = name
			return nil
		},
	}
	if !cleanupFailedManagedRun(context.Background(), request, "yokai-deployment-dep-test-g3-head", deps) || removed != strings.Repeat("a", 64) {
		t.Fatalf("owned ambiguous launch candidate was not cleaned up: removed=%q", removed)
	}
}

func TestFailedRunCleanupRemovesOwnedLegacyContainer(t *testing.T) {
	request := ContainerRequest{Labels: map[string]string{
		LabelManaged:     "true",
		LabelOwnership:   OwnershipManaged,
		LabelLaunchNonce: "launch-a",
	}}
	removed := ""
	deps := failedRunCleanupDeps{
		inspect: func(_ context.Context, name string) (string, string, map[string]string, error) {
			return strings.Repeat("f", 64), name, map[string]string{
				LabelManaged:     "true",
				LabelOwnership:   OwnershipManaged,
				LabelLaunchNonce: "launch-a",
			}, nil
		},
		remove: func(_ context.Context, id string) error {
			removed = id
			return nil
		},
	}
	if !cleanupFailedManagedRun(context.Background(), request, "yokai-legacy", deps) || removed != strings.Repeat("f", 64) {
		t.Fatalf("owned legacy launch artifact was not cleaned up: removed=%q", removed)
	}
}

func TestPrepareLegacyLaunchNonceOverridesCallerValueAndReachesDockerArgs(t *testing.T) {
	request := ContainerRequest{Image: "example/image", Labels: map[string]string{
		LabelManaged:     "true",
		LabelOwnership:   OwnershipManaged,
		LabelLaunchNonce: "caller-controlled",
	}}
	if err := prepareLegacyLaunchNonce(&request); err != nil {
		t.Fatal(err)
	}
	nonce := request.Labels[LabelLaunchNonce]
	if nonce == "caller-controlled" || len(nonce) != 32 || strings.Trim(nonce, "0123456789abcdef") != "" {
		t.Fatalf("unexpected generated launch nonce %q", nonce)
	}
	args := buildDockerRunArgs(request, "yokai-legacy")
	found := false
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--label" && args[index+1] == LabelLaunchNonce+"="+nonce {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("launch nonce did not reach docker argv: %v", args)
	}
}

func TestPrepareLegacyLaunchNonceLeavesGroupedProvenanceUnchanged(t *testing.T) {
	request := ContainerRequest{Labels: map[string]string{
		LabelManaged:      "true",
		LabelOwnership:    OwnershipManaged,
		LabelDeploymentID: "dep-test",
		LabelGeneration:   "3",
		LabelRole:         "head",
	}}
	if err := prepareLegacyLaunchNonce(&request); err != nil {
		t.Fatal(err)
	}
	if nonce := request.Labels[LabelLaunchNonce]; nonce != "" {
		t.Fatalf("grouped request received legacy launch nonce %q", nonce)
	}
}

func TestFailedRunCleanupPreservesLegacyNameCollisionAndPartialProvenance(t *testing.T) {
	requestBase := map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelLaunchNonce: "launch-a"}
	observedBase := map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelLaunchNonce: "launch-a"}
	cloneLabels := func(source map[string]string) map[string]string {
		cloned := make(map[string]string, len(source))
		for key, value := range source {
			cloned[key] = value
		}
		return cloned
	}
	tests := map[string]struct {
		requestLabels  map[string]string
		observedName   string
		observedLabels map[string]string
	}{
		"unmanaged": {
			requestLabels:  cloneLabels(requestBase),
			observedName:   "yokai-legacy",
			observedLabels: map[string]string{LabelManaged: "false", LabelOwnership: OwnershipManaged, LabelLaunchNonce: "launch-a"},
		},
		"observed ownership": {
			requestLabels:  cloneLabels(requestBase),
			observedName:   "yokai-legacy",
			observedLabels: map[string]string{LabelManaged: "true", LabelOwnership: OwnershipObserved, LabelLaunchNonce: "launch-a"},
		},
		"different name": {
			requestLabels:  cloneLabels(requestBase),
			observedName:   "unowned-collision",
			observedLabels: cloneLabels(observedBase),
		},
		"different launch": {
			requestLabels:  cloneLabels(requestBase),
			observedName:   "yokai-legacy",
			observedLabels: map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelLaunchNonce: "launch-old"},
		},
		"request ownership": {
			requestLabels:  map[string]string{LabelManaged: "true", LabelOwnership: OwnershipAdopted, LabelLaunchNonce: "launch-a"},
			observedName:   "yokai-legacy",
			observedLabels: cloneLabels(observedBase),
		},
		"partial grouped provenance": {
			requestLabels:  map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelDeploymentID: "dep-test"},
			observedName:   "yokai-legacy",
			observedLabels: map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelDeploymentID: "dep-test"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			removed := false
			deps := failedRunCleanupDeps{
				inspect: func(context.Context, string) (string, string, map[string]string, error) {
					return strings.Repeat("f", 64), test.observedName, test.observedLabels, nil
				},
				remove: func(context.Context, string) error { removed = true; return nil },
			}
			request := ContainerRequest{Labels: test.requestLabels}
			if cleanupFailedManagedRun(context.Background(), request, "yokai-legacy", deps) || removed {
				t.Fatal("cleanup removed a legacy collision without exact managed identity")
			}
		})
	}
}

func TestFailedRunCleanupPreservesUnownedNameCollision(t *testing.T) {
	request := ContainerRequest{Labels: map[string]string{
		LabelManaged:      "true",
		LabelOwnership:    OwnershipManaged,
		LabelDeploymentID: "dep-test",
		LabelGeneration:   "3",
		LabelRole:         "head",
	}}
	removed := false
	deps := failedRunCleanupDeps{
		inspect: func(_ context.Context, name string) (string, string, map[string]string, error) {
			return strings.Repeat("b", 64), name, map[string]string{LabelOwnership: OwnershipObserved}, nil
		},
		remove: func(context.Context, string) error {
			removed = true
			return nil
		},
	}
	if cleanupFailedManagedRun(context.Background(), request, "yokai-deployment-dep-test-g3-head", deps) || removed {
		t.Fatal("unowned same-name container was removed after docker run failure")
	}
}

func TestFailedRunCleanupRejectsEveryProvenanceMismatch(t *testing.T) {
	name := "yokai-deployment-dep-test-g3-head"
	request := ContainerRequest{Labels: map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelDeploymentID: "dep-test", LabelGeneration: "3", LabelRole: "head"}}
	base := map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelDeploymentID: "dep-test", LabelGeneration: "3", LabelRole: "head"}
	tests := map[string]func(map[string]string) string{
		"managed":    func(labels map[string]string) string { labels[LabelManaged] = "false"; return name },
		"ownership":  func(labels map[string]string) string { labels[LabelOwnership] = OwnershipObserved; return name },
		"deployment": func(labels map[string]string) string { labels[LabelDeploymentID] = "other"; return name },
		"generation": func(labels map[string]string) string { labels[LabelGeneration] = "4"; return name },
		"role":       func(labels map[string]string) string { labels[LabelRole] = "worker"; return name },
		"name":       func(map[string]string) string { return "unowned-collision" },
	}
	for mismatch, mutate := range tests {
		t.Run(mismatch, func(t *testing.T) {
			labels := make(map[string]string, len(base))
			for key, value := range base {
				labels[key] = value
			}
			observedName := mutate(labels)
			removed := false
			deps := failedRunCleanupDeps{
				inspect: func(context.Context, string) (string, string, map[string]string, error) {
					return strings.Repeat("c", 64), observedName, labels, nil
				},
				remove: func(context.Context, string) error { removed = true; return nil },
			}
			if cleanupFailedManagedRun(context.Background(), request, name, deps) || removed {
				t.Fatalf("%s mismatch was removed", mismatch)
			}
		})
	}
}

func TestFailedRunCleanupRetriesLateOwnedCandidate(t *testing.T) {
	request := ContainerRequest{Labels: map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelDeploymentID: "dep-test", LabelGeneration: "3", LabelRole: "head"}}
	attempts := 0
	removed := false
	deps := failedRunCleanupDeps{
		inspect: func(_ context.Context, name string) (string, string, map[string]string, error) {
			attempts++
			if attempts < 3 {
				return "", "", nil, errors.New("not materialized yet")
			}
			return strings.Repeat("d", 64), name, request.Labels, nil
		},
		remove: func(context.Context, string) error { removed = true; return nil },
	}
	if !cleanupFailedManagedRunEventually(context.Background(), request, "yokai-deployment-dep-test-g3-head", deps, 100*time.Millisecond, time.Millisecond) || !removed || attempts < 3 {
		t.Fatalf("late owned candidate was not removed: attempts=%d removed=%v", attempts, removed)
	}
}

func TestFailedRunCleanupDeadlineBoundsHungDockerOperations(t *testing.T) {
	request := ContainerRequest{Labels: map[string]string{
		LabelManaged: "true", LabelOwnership: OwnershipManaged,
		LabelDeploymentID: "dep-test", LabelGeneration: "3", LabelRole: "head",
	}}
	const candidateID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, test := range []struct {
		name string
		deps func(*string) failedRunCleanupDeps
	}{
		{
			name: "inspect",
			deps: func(_ *string) failedRunCleanupDeps {
				return failedRunCleanupDeps{
					inspect: func(ctx context.Context, _ string) (string, string, map[string]string, error) {
						<-ctx.Done()
						return "", "", nil, ctx.Err()
					},
					remove: func(context.Context, string) error {
						t.Fatal("remove called after timed-out inspect")
						return nil
					},
				}
			},
		},
		{
			name: "remove",
			deps: func(removed *string) failedRunCleanupDeps {
				return failedRunCleanupDeps{
					inspect: func(_ context.Context, name string) (string, string, map[string]string, error) {
						return candidateID, name, request.Labels, nil
					},
					remove: func(ctx context.Context, id string) error {
						*removed = id
						<-ctx.Done()
						return ctx.Err()
					},
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			removed := ""
			started := time.Now()
			if cleanupFailedManagedRunEventually(context.Background(), request, "yokai-deployment-dep-test-g3-head", test.deps(&removed), 50*time.Millisecond, time.Millisecond) {
				t.Fatal("timed-out cleanup reported success")
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Fatalf("hung %s exceeded cleanup budget: %s", test.name, elapsed)
			}
			if test.name == "remove" && removed != candidateID {
				t.Fatalf("cleanup targeted %q, want exact inspected ID %q", removed, candidateID)
			}
		})
	}
}

func TestDockerLifecycleCommandsHonorCancellation(t *testing.T) {
	for _, command := range []string{"inspect", "rm", "stop", "restart"} {
		t.Run(command, func(t *testing.T) {
			binDir := t.TempDir()
			markerPath := filepath.Join(binDir, command+"-started")
			dockerPath := filepath.Join(binDir, "docker")
			script := "#!/bin/sh\nif [ \"$1\" = \"" + command + "\" ]; then\n  : > \"" + markerPath + "\"\n  exec sleep 30\nfi\nexit 1\n"
			if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			started := time.Now()
			var err error
			switch command {
			case "inspect":
				_, _, _, err = inspectContainerIdentityWithContext(ctx, "candidate")
			case "rm":
				err = removeContainerWithContext(ctx, strings.Repeat("a", 64))
			case "stop":
				err = stopContainerWithContext(ctx, strings.Repeat("a", 64))
			case "restart":
				err = restartContainerWithContext(ctx, strings.Repeat("a", 64))
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("%s cancellation was not returned: %v", command, err)
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Fatalf("docker %s exceeded cancellation bound: %s", command, elapsed)
			}
			if _, err := os.Stat(markerPath); err != nil {
				t.Fatalf("docker %s did not start: %v", command, err)
			}
		})
	}
}

func TestRunContainerCancellationKillsDockerCLIAndCleansOwnedCandidate(t *testing.T) {
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nif [ \"$1\" = run ]; then exec sleep 5; fi\nexit 1\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	request := ContainerRequest{Image: "example.invalid/image", Name: "yokai-deployment-dep-test-g3-head", Labels: map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged, LabelDeploymentID: "dep-test", LabelGeneration: "3", LabelRole: "head"}}
	removed := false
	deps := failedRunCleanupDeps{
		inspect: func(_ context.Context, name string) (string, string, map[string]string, error) {
			return strings.Repeat("e", 64), name, request.Labels, nil
		},
		remove: func(context.Context, string) error { removed = true; return nil },
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, cancel)
	started := time.Now()
	if _, err := runContainerWithContext(ctx, request, deps); err == nil || !removed {
		t.Fatalf("canceled launch did not clean owned candidate: err=%v removed=%v", err, removed)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("docker CLI was not canceled promptly")
	}
}

func TestValidateImagePlatformCancellationBoundsManifestInspect(t *testing.T) {
	binDir := t.TempDir()
	markerPath := filepath.Join(binDir, "manifest-started")
	dockerPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nif [ \"$1\" = manifest ] && [ \"$2\" = inspect ]; then\n  : > \"" + markerPath + "\"\n  exec sleep 30\nfi\nexit 1\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := validateImagePlatform(ctx, "example.invalid/image")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("manifest cancellation was not returned: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("manifest process exceeded cancellation bound: %s", elapsed)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("manifest process did not start: %v", err)
	}
}

func TestValidatePulledImageArchitectureCancellationBoundsInspect(t *testing.T) {
	binDir := t.TempDir()
	markerPath := filepath.Join(binDir, "inspect-started")
	dockerPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nif [ \"$1\" = inspect ]; then\n  : > \"" + markerPath + "\"\n  exec sleep 30\nfi\nexit 1\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := validatePulledImageArchitecture(ctx, "example.invalid/image")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("architecture inspect cancellation was not returned: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("architecture inspect exceeded cancellation bound: %s", elapsed)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("architecture inspect did not start: %v", err)
	}
}

func TestValidateImagePlatformStillRejectsUnsupportedManifest(t *testing.T) {
	unsupportedArch := "arm64"
	if runtime.GOARCH == unsupportedArch {
		unsupportedArch = "amd64"
	}
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nif [ \"$1\" = manifest ] && [ \"$2\" = inspect ]; then\n  printf '%s\\n' '{\"schemaVersion\":2,\"architecture\":\"" + unsupportedArch + "\",\"os\":\"" + runtime.GOOS + "\"}'\n  exit 0\nfi\nexit 1\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := validateImagePlatform(context.Background(), "example.invalid/image"); err == nil || !strings.Contains(err.Error(), "does not support host platform") {
		t.Fatalf("unsupported manifest was not rejected: %v", err)
	}
}

func TestDockerRunFailureDoesNotExposeCommandOrDaemonOutput(t *testing.T) {
	const secret = "sentinel-rank-zero-secret"
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/bin/sh
if [ "$1" = "run" ]; then
  printf '%s\n' 'daemon echoed sentinel-rank-zero-secret' >&2
  exit 1
fi
exit 1
`
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := runContainer(ContainerRequest{
		Image: "example.invalid/image", Name: "safe-failure", Ports: map[string]string{},
		Args: []string{"--api-key=" + secret},
	})
	if err == nil {
		t.Fatal("expected docker run failure")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "--api-key") {
		t.Fatalf("loggable launch error exposed rank-0 command or daemon output: %v", err)
	}
}

func TestContainerInventoryScopesAndRedaction(t *testing.T) {
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/bin/sh
printf '%s\n' '{"ID":"managed123456","Names":"yokai-managed","Image":"safe/image","Status":"Up 2 minutes","Ports":"","CreatedAt":"2026-08-27 10:00:00 +0000 UTC","RunningFor":"2 minutes","Labels":"io.yokai.managed=true,io.yokai.ownership=managed","Env":"API_KEY=managed-secret"}'
printf '%s\n' '{"ID":"external12345","Names":"external-service","Image":"external/image","Status":"Exited (0) 1 minute ago","Ports":"","CreatedAt":"2026-08-27 10:00:00 +0000 UTC","RunningFor":"1 minute","Labels":"api_key=sentinel-secret,io.yokai.api_key=sentinel-secret,io.yokai.service.port=30000","Env":"API_KEY=sentinel-secret"}'
`
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	managed, err := listContainersScope(InventoryScopeManaged)
	if err != nil {
		t.Fatalf("managed inventory: %v", err)
	}
	if len(managed) != 1 || managed[0].Ownership != OwnershipManaged {
		t.Fatalf("unexpected managed inventory: %#v", managed)
	}
	all, err := listContainersScope(InventoryScopeAll)
	if err != nil {
		t.Fatalf("all inventory: %v", err)
	}
	if len(all) != 2 || all[1].Status != "stopped" || all[1].Ownership != OwnershipObserved {
		t.Fatalf("unexpected all inventory: %#v", all)
	}
	data, err := json.Marshal(all)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	lower := strings.ToLower(string(data))
	for _, forbidden := range []string{"api_key", "sentinel-secret", "\"env\""} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("all-scope inventory exposed %q: %s", forbidden, data)
		}
	}
}

func TestContainerInventoryAllSkipsMetricsWhileManagedPopulatesThem(t *testing.T) {
	probeCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeCalls++
		if r.URL.Path != "/metrics" {
			t.Fatalf("unexpected metrics path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("vllm:num_requests_running 1\n"))
	}))
	defer server.Close()
	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")

	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	line := fmt.Sprintf(`{"ID":"managed123456","Names":"yokai-managed","Image":"vllm/vllm-openai:latest","Status":"Up 2 minutes","Ports":"","CreatedAt":"2026-08-27 10:00:00 +0000 UTC","RunningFor":"2 minutes","Labels":"io.yokai.managed=true,io.yokai.ownership=managed,io.yokai.service.address=127.0.0.1,io.yokai.service.port=%s"}`, port)
	script := "#!/bin/sh\nprintf '%s\\n' '" + line + "'\n"
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	all, err := listContainersScope(InventoryScopeAll)
	if err != nil || len(all) != 1 {
		t.Fatalf("all inventory failed: containers=%#v err=%v", all, err)
	}
	if probeCalls != 0 || all[0].VLLMMetrics != nil {
		t.Fatalf("all-scope identity inventory scraped metrics: calls=%d metrics=%#v", probeCalls, all[0].VLLMMetrics)
	}

	managed, err := listContainersScope(InventoryScopeManaged)
	if err != nil || len(managed) != 1 || managed[0].VLLMMetrics == nil {
		t.Fatalf("managed inventory did not populate metrics: containers=%#v err=%v", managed, err)
	}
	if probeCalls != 1 || managed[0].VLLMMetrics.RequestsRunning != 1 {
		t.Fatalf("managed scrape mismatch: calls=%d metrics=%#v", probeCalls, managed[0].VLLMMetrics)
	}
}

func TestRuntimePatchLabelIsSafeInventoryProvenance(t *testing.T) {
	labels := sanitizeInventoryLabels(map[string]string{
		LabelRuntimePatch: bkc.GLM53FlashRuntimePatchSetLabel(),
		"api_key":         "must-not-survive",
	})
	if labels[LabelRuntimePatch] != bkc.GLM53FlashRuntimePatchSetLabel() || len(labels) != 1 {
		t.Fatalf("runtime patch provenance was not safely filtered: %#v", labels)
	}
}

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid name",
			input:    "my-container-123",
			expected: "my-container-123",
		},
		{
			name:     "uppercase letters",
			input:    "MyContainer",
			expected: "MyContainer",
		},
		{
			name:     "underscore and dots",
			input:    "my_container.test",
			expected: "my_container.test",
		},
		{
			name:     "spaces replaced with hyphens",
			input:    "my container name",
			expected: "my-container-name",
		},
		{
			name:     "special characters replaced",
			input:    "my@container#name!",
			expected: "my-container-name",
		},
		{
			name:     "leading and trailing hyphens removed",
			input:    "!@#container$%^",
			expected: "container",
		},
		{
			name:     "multiple consecutive invalid chars",
			input:    "my!!!container",
			expected: "my---container",
		},
		{
			name:     "empty after sanitization",
			input:    "!@#$%^&*()",
			expected: "unnamed",
		},
		{
			name:     "already empty",
			input:    "",
			expected: "unnamed",
		},
		{
			name:     "only hyphens",
			input:    "---",
			expected: "unnamed",
		},
		{
			name:     "unicode characters",
			input:    "côntainer-ñame",
			expected: "c-ntainer--ame",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		dockerStatus string
		expected     string
	}{
		{
			name:         "running container",
			dockerStatus: "Up 2 hours",
			expected:     "running",
		},
		{
			name:         "running with port info",
			dockerStatus: "Up 5 minutes, 0.0.0.0:8080->80/tcp",
			expected:     "running",
		},
		{
			name:         "exited container",
			dockerStatus: "Exited (0) 2 minutes ago",
			expected:     "stopped",
		},
		{
			name:         "exited with error",
			dockerStatus: "Exited (1) 1 hour ago",
			expected:     "stopped",
		},
		{
			name:         "created but not started",
			dockerStatus: "Created",
			expected:     "created",
		},
		{
			name:         "restarting",
			dockerStatus: "Restarting (1) 30 seconds ago",
			expected:     "restarting",
		},
		{
			name:         "dead",
			dockerStatus: "Dead",
			expected:     "dead",
		},
		{
			name:         "unknown status",
			dockerStatus: "SomeUnknownStatus",
			expected:     "unknown",
		},
		{
			name:         "empty status",
			dockerStatus: "",
			expected:     "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseStatus(tt.dockerStatus)
			if result != tt.expected {
				t.Errorf("parseStatus(%q) = %q, expected %q", tt.dockerStatus, result, tt.expected)
			}
		})
	}
}

func TestContainerRequestValidation(t *testing.T) {
	t.Parallel()

	req := ContainerRequest{
		Image:     "nginx:latest",
		Name:      "test-nginx",
		Model:     "TheBloke/test.gguf",
		Ports:     map[string]string{"80": "8080"},
		Env:       map[string]string{"TEST": "value"},
		GPUIDs:    "all",
		ExtraArgs: "--rm",
		Volumes:   map[string]string{"/host": "/container"},
	}

	if req.Image != "nginx:latest" {
		t.Errorf("expected image 'nginx:latest', got %s", req.Image)
	}
	if req.Name != "test-nginx" {
		t.Errorf("expected name 'test-nginx', got %s", req.Name)
	}
	if req.Model != "TheBloke/test.gguf" {
		t.Errorf("expected model 'TheBloke/test.gguf', got %s", req.Model)
	}
	if req.Ports["80"] != "8080" {
		t.Errorf("expected port mapping 80->8080, got %s", req.Ports["80"])
	}
	if req.Env["TEST"] != "value" {
		t.Errorf("expected env TEST=value, got %s", req.Env["TEST"])
	}
	if req.GPUIDs != "all" {
		t.Errorf("expected GPUIDs 'all', got %s", req.GPUIDs)
	}
	if req.ExtraArgs != "--rm" {
		t.Errorf("expected extra args '--rm', got %s", req.ExtraArgs)
	}
	if req.Volumes["/host"] != "/container" {
		t.Errorf("expected volume mapping /host->/container, got %s", req.Volumes["/host"])
	}
}

func TestContainerStructure(t *testing.T) {
	t.Parallel()

	container := Container{
		ID:     "abc123",
		Name:   "test-container",
		Image:  "nginx:latest",
		Status: "running",
		Ports:  map[string]string{"80": "8080"},
	}

	if container.ID != "abc123" {
		t.Errorf("expected ID 'abc123', got %s", container.ID)
	}
	if container.Name != "test-container" {
		t.Errorf("expected name 'test-container', got %s", container.Name)
	}
	if container.Status != "running" {
		t.Errorf("expected status 'running', got %s", container.Status)
	}
}

func TestContainerResponseStructure(t *testing.T) {
	t.Parallel()

	response := ContainerResponse{
		ID:     "xyz789",
		Status: "created",
	}

	if response.ID != "xyz789" {
		t.Errorf("expected ID 'xyz789', got %s", response.ID)
	}
	if response.Status != "created" {
		t.Errorf("expected status 'created', got %s", response.Status)
	}
}

func TestImagePullRequestStructure(t *testing.T) {
	t.Parallel()

	req := ImagePullRequest{
		Image: "ubuntu:latest",
	}

	if req.Image != "ubuntu:latest" {
		t.Errorf("expected image 'ubuntu:latest', got %s", req.Image)
	}
}

func TestDefaultArgsRespectUserOverrides(t *testing.T) {
	tests := []struct {
		name  string
		got   string
		wants []string
	}{
		{name: "vllm model equals form", got: withVLLMModelArg("--model=custom/repo", "default/repo"), wants: []string{"--model=custom/repo"}},
		{name: "sglang injects serve and model", got: withSGLangServeArgs("--tp-size 1", "Qwen/model"), wants: []string{"sglang serve --model-path Qwen/model", "--tp-size 1"}},
		{name: "sglang respects model override", got: withSGLangServeArgs("sglang serve --model-path custom/repo --tp-size 1", "default/repo"), wants: []string{"--model-path custom/repo", "--tp-size 1"}},
		{name: "llama model equals form", got: withLlamaModelArg("--model=/tmp/model.gguf", "foo/bar.gguf"), wants: []string{"--model=/tmp/model.gguf"}},
		{name: "host equals form", got: withHostArg("--host=127.0.0.1", "--host", "0.0.0.0"), wants: []string{"--host=127.0.0.1"}},
		{name: "tool parser equals form", got: withVLLMToolCallArgs("--tool-call-parser=hermes", "meta-llama/Llama-3.1-8B-Instruct"), wants: []string{"--enable-auto-tool-choice", "--tool-call-parser=hermes"}},
	}

	for _, tt := range tests {
		for _, want := range tt.wants {
			if !strings.Contains(tt.got, want) {
				t.Fatalf("%s: expected %q in %q", tt.name, want, tt.got)
			}
		}
	}
}

func TestScrapeSGLangMetrics(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`sglang:gen_throughput{model_name="Qwen3.8-27B"} 298.4
sglang:num_running_reqs{model_name="Qwen3.8-27B"} 2
sglang:num_queue_reqs{model_name="Qwen3.8-27B"} 1
sglang:prompt_tokens_total{model_name="Qwen3.8-27B",is_streaming="true"} 1200
sglang:generation_tokens_total{model_name="Qwen3.8-27B",is_streaming="true"} 900
sglang:cached_tokens_total{model_name="Qwen3.8-27B"} 300
sglang:time_to_first_token_seconds_bucket{model_name="Qwen3.8-27B",le="0.5"} 10
sglang:time_to_first_token_seconds_sum{model_name="Qwen3.8-27B"} 2.5
sglang:time_to_first_token_seconds_count{model_name="Qwen3.8-27B"} 10
`))
	}))
	defer server.Close()

	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	metrics, err := scrapeVLLMMetrics(port)
	if err != nil {
		t.Fatalf("scrape SGLang metrics: %v", err)
	}
	if metrics.Model != "Qwen3.8-27B" || metrics.GenerationTokPerSec != 298.4 {
		t.Fatalf("unexpected SGLang metrics: %#v", metrics)
	}
	if metrics.RequestsRunning != 2 || metrics.RequestsWaiting != 1 {
		t.Fatalf("unexpected request gauges: %#v", metrics)
	}
	if !metrics.HasTTFT || metrics.TTFTBuckets["0.5"] != 10 || metrics.TTFTCount != 10 {
		t.Fatalf("unexpected TTFT histogram: %#v", metrics)
	}
	if !metrics.HasSGLangNativeMetric || metrics.HasVLLMNativeMetric {
		t.Fatalf("unexpected native metric family detection: %#v", metrics)
	}
}

func TestApplyPluginsAddsAssetsMountsAndArgs(t *testing.T) {
	t.Setenv("YOKAI_PLUGIN_DIR", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plugin-body"))
	}))
	defer server.Close()

	plugin, ok := plugins.Lookup("vllm-reasoning-parser-super-v3")
	if !ok {
		t.Fatal("expected plugin catalog entry")
	}
	oldURL := plugin.Assets[0].URL
	plugin.Assets[0].URL = server.URL

	// shadow the catalog lookup with a temporary env-controlled URL by swapping after lookup use
	req := &ContainerRequest{
		Plugins: []string{"vllm-reasoning-parser-super-v3"},
		Env:     map[string]string{},
		Volumes: map[string]string{},
	}

	// direct write through ensurePluginAsset to avoid mutating package catalog; simulate asset fetch
	if err := ensurePluginAsset(plugin.ID, plugins.Asset{FileName: plugin.Assets[0].FileName, URL: server.URL}); err != nil {
		t.Fatalf("ensurePluginAsset failed: %v", err)
	}
	if err := applyPlugins(req); err == nil {
		// applyPlugins will re-download from the real URL in catalog; only assert if plugin already cached path exists
	} else if !strings.Contains(err.Error(), oldURL) {
		t.Fatalf("unexpected applyPlugins error: %v", err)
	}

	assetPath := plugins.AssetHostPath(plugin.ID, plugin.Assets[0].FileName)
	if _, err := os.Stat(assetPath); err != nil {
		t.Fatalf("expected plugin asset at %s: %v", assetPath, err)
	}
	if req.Volumes[assetPath] != "/plugins/super_v3_reasoning_parser.py" {
		t.Fatalf("expected plugin volume mount, got %#v", req.Volumes)
	}
	if !strings.Contains(req.ExtraArgs, "--reasoning-parser super_v3") {
		t.Fatalf("expected plugin extra args, got %q", req.ExtraArgs)
	}
}

// Integration tests that require Docker - skip in short mode
func TestListContainersIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker integration test in short mode")
	}

	t.Parallel()

	containers, err := listContainers()
	if err != nil {
		t.Logf("listContainers failed (expected if Docker not available): %v", err)
		return
	}

	if containers == nil {
		t.Log("containers is nil (expected if Docker unavailable or no yokai containers)")
	}

	t.Logf("Found %d containers", len(containers))

	for i, container := range containers {
		if container.ID == "" {
			t.Errorf("container %d has empty ID", i)
		}
		if container.Name == "" {
			t.Errorf("container %d has empty name", i)
		}
		if container.Image == "" {
			t.Errorf("container %d has empty image", i)
		}
		if container.Status == "" {
			t.Errorf("container %d has empty status", i)
		}
	}
}

func TestPullImageIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker integration test in short mode")
	}

	t.Parallel()

	err := pullImage("hello-world:latest")
	if err != nil {
		t.Logf("pullImage failed (expected if Docker not available): %v", err)
	} else {
		t.Log("Successfully pulled hello-world:latest")
	}
}

func TestContainerOperationsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker integration test in short mode")
	}

	t.Parallel()

	testContainerID := "nonexistent-container-id"

	err := stopContainer(testContainerID)
	if err == nil {
		t.Log("stopContainer on non-existent container unexpectedly succeeded")
	}

	err = restartContainer(testContainerID)
	if err == nil {
		t.Log("restartContainer on non-existent container unexpectedly succeeded")
	}

	err = removeContainer(testContainerID)
	t.Logf("removeContainer on non-existent container: %v", err)
}

func TestDockerOutputTail(t *testing.T) {
	if got := dockerOutputTail([]byte("")); got != "" {
		t.Fatalf("empty output = %q, want empty", got)
	}
	in := "line1\nline2\nline3"
	if got := dockerOutputTail([]byte(in)); got != in {
		t.Fatalf("short output mangled: %q", got)
	}
	var many []string
	for i := 0; i < 20; i++ {
		many = append(many, "l"+string(rune('a'+i%26)))
	}
	gotLines := strings.Split(dockerOutputTail([]byte(strings.Join(many, "\n"))), "\n")
	if len(gotLines) != 8 {
		t.Fatalf("expected 8 lines, got %d", len(gotLines))
	}
	if gotLines[0] != many[12] {
		t.Fatalf("expected first tail line %q, got %q", many[12], gotLines[0])
	}
	if gotLines[7] != many[19] {
		t.Fatalf("expected last tail line %q, got %q", many[19], gotLines[7])
	}
}

func TestLogDetail(t *testing.T) {
	if got := logDetail(""); got != "" {
		t.Fatalf("empty detail = %q, want empty", got)
	}
	if got := logDetail("boom"); got != ": boom" {
		t.Fatalf("logDetail = %q, want \": boom\"", got)
	}
}
