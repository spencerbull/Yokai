package agent

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
)

func TestQwenPageCacheReleaseTargetsValidatedSnapshotAndReportsActualWork(t *testing.T) {
	repository, objects := qwenPageCacheFixture(t)
	req := pinnedQwen38PatchRequest(bkc.MultiDeviceRoleWorker)
	req.Volumes = map[string]string{repository: bkc.Qwen38FlashNextContainerRoot + ":ro"}
	calls := 0
	report := releaseQwen38PageCacheWithObjects(req, objects, func(file *os.File) error {
		calls++
		if filepath.Base(file.Name()) != objects["model.safetensors"].blob {
			t.Fatalf("page-cache release did not open the exact expected blob: %q", file.Name())
		}
		return nil
	})
	if !report.Attempted || report.Expected != 1 || report.Verified != 1 || calls != 1 || report.Files != 1 || report.Bytes != int64(len("weight-data")) || report.Skipped != 0 {
		t.Fatalf("page-cache release report is inaccurate: calls=%d report=%#v", calls, report)
	}
	if !strings.Contains(report.String(), "released 11 B across 1 file(s)") {
		t.Fatalf("page-cache audit summary omitted actual work: %q", report.String())
	}
}

func TestQwenPageCacheReleaseNeverTouchesUnexpectedSnapshotWeights(t *testing.T) {
	repository := filepath.Join(t.TempDir(), bkc.Qwen38FlashNextHFCacheDirectory)
	snapshot := filepath.Join(repository, "snapshots", bkc.Qwen38FlashNextNVFP4Revision)
	if err := os.MkdirAll(snapshot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "attacker.safetensors"), []byte("untrusted-weight"), 0600); err != nil {
		t.Fatal(err)
	}
	req := pinnedQwen38PatchRequest(bkc.MultiDeviceRoleWorker)
	req.Volumes = map[string]string{repository: bkc.Qwen38FlashNextContainerRoot + ":ro"}
	calls := 0
	report := releaseQwen38PageCache(req, func(*os.File) error {
		calls++
		return nil
	})
	if calls != 0 || report.Files != 0 {
		t.Fatalf("page-cache release operated on an unmanifested snapshot entry: calls=%d report=%#v", calls, report)
	}
	if report.Failure == "" {
		t.Fatalf("missing expected pinned weight objects were not reported: %#v", report)
	}
}

func TestQwenPageCacheReleaseRequiresConfinedVerifiedBlob(t *testing.T) {
	t.Run("symlink target confinement", func(t *testing.T) {
		repository := filepath.Join(t.TempDir(), bkc.Qwen38FlashNextHFCacheDirectory)
		snapshot := filepath.Join(repository, "snapshots", bkc.Qwen38FlashNextNVFP4Revision)
		if err := os.MkdirAll(snapshot, 0700); err != nil {
			t.Fatal(err)
		}
		content := []byte("weight-data")
		digest := fmt.Sprintf("%x", sha256.Sum256(content))
		outside := filepath.Join(t.TempDir(), digest)
		if err := os.WriteFile(outside, content, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(snapshot, "model.safetensors")); err != nil {
			t.Fatal(err)
		}
		assertPageCacheReleaseRefusesObject(t, repository, map[string]qwen38SnapshotObject{"model.safetensors": {blob: digest, size: int64(len(content))}})
	})
	for _, test := range []struct {
		name    string
		content []byte
	}{
		{name: "size", content: []byte("weight-data-expanded")},
		{name: "digest", content: []byte("tamper-data")},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, objects := qwenPageCacheFixture(t)
			blob := filepath.Join(repository, "blobs", objects["model.safetensors"].blob)
			if err := os.WriteFile(blob, test.content, 0600); err != nil {
				t.Fatal(err)
			}
			assertPageCacheReleaseRefusesObject(t, repository, objects)
		})
	}
	t.Run("type", func(t *testing.T) {
		repository, objects := qwenPageCacheFixture(t)
		blob := filepath.Join(repository, "blobs", objects["model.safetensors"].blob)
		if err := os.Remove(blob); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(blob, 0700); err != nil {
			t.Fatal(err)
		}
		assertPageCacheReleaseRefusesObject(t, repository, objects)
	})
}

func TestQwenPageCacheReleaseIsBestEffortButAuditsSkippedFiles(t *testing.T) {
	repository, objects := qwenPageCacheFixture(t)
	req := pinnedQwen38PatchRequest(bkc.MultiDeviceRoleHead)
	req.Volumes = map[string]string{repository: bkc.Qwen38FlashNextContainerRoot + ":ro"}
	report := releaseQwen38PageCacheWithObjects(req, objects, func(*os.File) error { return errors.New("unsupported") })
	if !report.Attempted || report.Expected != 1 || report.Verified != 1 || report.Files != 0 || report.Bytes != 0 || report.Skipped != 1 {
		t.Fatalf("best-effort failure was reported as success: %#v", report)
	}
	if !strings.Contains(report.String(), "1 skipped") {
		t.Fatalf("best-effort failure was silent: %q", report.String())
	}
}

func TestQwenPageCacheReleaseRunsBeforeDockerStart(t *testing.T) {
	repository := filepath.Join(t.TempDir(), bkc.Qwen38FlashNextHFCacheDirectory)
	snapshot := filepath.Join(repository, "snapshots", bkc.Qwen38FlashNextNVFP4Revision)
	if err := os.MkdirAll(snapshot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "model.safetensors"), []byte("weight-data"), 0600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "fadvise-ran")
	dockerPath := filepath.Join(binDir, "docker")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = manifest ] && [ "$2" = inspect ]; then
  printf '%%s\n' '{"schemaVersion":2,"architecture":"%s","os":"%s"}'
  exit 0
fi
if [ "$1" = inspect ]; then
  printf '%%s\n' '%s'
  exit 0
fi
if [ "$1" = run ]; then
  test -f %q || exit 8
  printf '%%064d\n' 0
  exit 0
fi
exit 9
`, runtime.GOARCH, runtime.GOOS, runtime.GOARCH, marker)
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	originalEnsure := ensureQwen38RuntimeAssetsForLaunch
	originalAdvise := adviseDropPageCacheForLaunch
	originalRelease := releaseQwen38PageCacheForLaunch
	originalTenants := inspectGPUComputeTenantsForLaunch
	t.Cleanup(func() {
		ensureQwen38RuntimeAssetsForLaunch = originalEnsure
		adviseDropPageCacheForLaunch = originalAdvise
		releaseQwen38PageCacheForLaunch = originalRelease
		inspectGPUComputeTenantsForLaunch = originalTenants
	})
	ensureQwen38RuntimeAssetsForLaunch = func(context.Context) (string, error) { return t.TempDir(), nil }
	releaseQwen38PageCacheForLaunch = func(ContainerRequest, func(*os.File) error) pageCacheReleaseReport {
		if err := os.WriteFile(marker, []byte("ran"), 0600); err != nil {
			t.Fatal(err)
		}
		return pageCacheReleaseReport{Attempted: true, Expected: 1, Verified: 1, Files: 1, Bytes: 11}
	}
	inspectGPUComputeTenantsForLaunch = func(context.Context) ([]gpuComputeTenant, error) { return nil, nil }
	req := pinnedQwen38PatchRequest(bkc.MultiDeviceRoleWorker)
	req.Name = "qwen-worker"
	req.Volumes = map[string]string{repository: bkc.Qwen38FlashNextContainerRoot + ":ro"}
	response, err := runContainerWithContext(context.Background(), req, liveFailedRunCleanupDeps)
	if err != nil || response.Status != "created" {
		t.Fatalf("Qwen launch did not run audited page-cache release before Docker: response=%#v err=%v", response, err)
	}
}

func TestQwenLaunchRejectsUnsafePageCacheTargetsBeforeDockerRun(t *testing.T) {
	binDir := t.TempDir()
	runMarker := filepath.Join(binDir, "docker-run")
	dockerPath := filepath.Join(binDir, "docker")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = manifest ] && [ "$2" = inspect ]; then
  printf '%%s\n' '{"schemaVersion":2,"architecture":"%s","os":"%s"}'
  exit 0
fi
if [ "$1" = inspect ]; then
  printf '%%s\n' '%s'
  exit 0
fi
if [ "$1" = run ]; then
  : > %q
  printf '%%064d\n' 0
  exit 0
fi
exit 9
`, runtime.GOARCH, runtime.GOOS, runtime.GOARCH, runMarker)
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	originalEnsure := ensureQwen38RuntimeAssetsForLaunch
	originalRelease := releaseQwen38PageCacheForLaunch
	originalTenants := inspectGPUComputeTenantsForLaunch
	t.Cleanup(func() {
		ensureQwen38RuntimeAssetsForLaunch = originalEnsure
		releaseQwen38PageCacheForLaunch = originalRelease
		inspectGPUComputeTenantsForLaunch = originalTenants
	})
	ensureQwen38RuntimeAssetsForLaunch = func(context.Context) (string, error) { return t.TempDir(), nil }
	inspectGPUComputeTenantsForLaunch = func(context.Context) ([]gpuComputeTenant, error) { return nil, nil }
	releaseQwen38PageCacheForLaunch = func(ContainerRequest, func(*os.File) error) pageCacheReleaseReport {
		return pageCacheReleaseReport{Attempted: true, Expected: 11, Skipped: 11, Failure: "pinned weight target escaped expected blobs directory"}
	}
	req := pinnedQwen38PatchRequest(bkc.MultiDeviceRoleWorker)
	req.Name = "qwen-worker"
	_, err := runContainerWithContext(context.Background(), req, liveFailedRunCleanupDeps)
	if err == nil || !strings.Contains(err.Error(), "page-cache release targets") {
		t.Fatalf("unsafe page-cache precondition did not fail launch: %v", err)
	}
	if _, err := os.Stat(runMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Docker ran after unsafe page-cache validation: %v", err)
	}
}

func TestQwenLaunchRechecksIdleGPUImmediatelyBeforeDockerRun(t *testing.T) {
	binDir := t.TempDir()
	runMarker := filepath.Join(binDir, "docker-run")
	dockerPath := filepath.Join(binDir, "docker")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = manifest ] && [ "$2" = inspect ]; then
  printf '%%s\n' '{"schemaVersion":2,"architecture":"%s","os":"%s"}'
  exit 0
fi
if [ "$1" = inspect ]; then
  printf '%%s\n' '%s'
  exit 0
fi
if [ "$1" = run ]; then
  : > %q
  printf '%%064d\n' 0
  exit 0
fi
exit 9
`, runtime.GOARCH, runtime.GOOS, runtime.GOARCH, runMarker)
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	originalEnsure := ensureQwen38RuntimeAssetsForLaunch
	originalRelease := releaseQwen38PageCacheForLaunch
	originalTenants := inspectGPUComputeTenantsForLaunch
	t.Cleanup(func() {
		ensureQwen38RuntimeAssetsForLaunch = originalEnsure
		releaseQwen38PageCacheForLaunch = originalRelease
		inspectGPUComputeTenantsForLaunch = originalTenants
	})
	ensureQwen38RuntimeAssetsForLaunch = func(context.Context) (string, error) { return t.TempDir(), nil }
	releaseQwen38PageCacheForLaunch = func(ContainerRequest, func(*os.File) error) pageCacheReleaseReport {
		return pageCacheReleaseReport{Attempted: true, Expected: 1, Verified: 1, Files: 1, Bytes: 11}
	}
	inspectGPUComputeTenantsForLaunch = func(context.Context) ([]gpuComputeTenant, error) {
		return []gpuComputeTenant{{PID: 991}}, nil
	}
	req := pinnedQwen38PatchRequest(bkc.MultiDeviceRoleHead)
	req.Name = "qwen-head"
	_, err := runContainerWithContext(context.Background(), req, liveFailedRunCleanupDeps)
	if err == nil || !strings.Contains(err.Error(), "active compute tenant") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("launch-time GPU tenant check failed closed unsafely: %v", err)
	}
	if _, err := os.Stat(runMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Docker ran despite a newly active compute tenant: %v", err)
	}
}

func TestQwenLaunchAdmissionSerializesFinalGPUCheckThroughDockerRun(t *testing.T) {
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = manifest ] && [ "$2" = inspect ]; then
  printf '%%s\n' '{"schemaVersion":2,"architecture":"%s","os":"%s"}'
  exit 0
fi
if [ "$1" = inspect ]; then
  printf '%%s\n' '%s'
  exit 0
fi
if [ "$1" = run ]; then
  printf '%%064d\n' 0
  exit 0
fi
exit 9
`, runtime.GOARCH, runtime.GOOS, runtime.GOARCH)
	if err := os.WriteFile(dockerPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	originalEnsure := ensureQwen38RuntimeAssetsForLaunch
	originalRelease := releaseQwen38PageCacheForLaunch
	originalTenants := inspectGPUComputeTenantsForLaunch
	t.Cleanup(func() {
		ensureQwen38RuntimeAssetsForLaunch = originalEnsure
		releaseQwen38PageCacheForLaunch = originalRelease
		inspectGPUComputeTenantsForLaunch = originalTenants
	})
	ensureQwen38RuntimeAssetsForLaunch = func(context.Context) (string, error) { return t.TempDir(), nil }
	releaseQwen38PageCacheForLaunch = func(ContainerRequest, func(*os.File) error) pageCacheReleaseReport {
		return pageCacheReleaseReport{Attempted: true, Expected: 1, Verified: 1, Files: 1, Bytes: 11}
	}
	checks := make(chan struct{}, 2)
	releaseChecks := make(chan struct{}, 2)
	inspectGPUComputeTenantsForLaunch = func(context.Context) ([]gpuComputeTenant, error) {
		checks <- struct{}{}
		<-releaseChecks
		return nil, nil
	}
	launch := func(name string) <-chan error {
		done := make(chan error, 1)
		go func() {
			req := pinnedQwen38PatchRequest(bkc.MultiDeviceRoleWorker)
			req.Name = name
			_, err := runContainerWithContext(context.Background(), req, liveFailedRunCleanupDeps)
			done <- err
		}()
		return done
	}
	first := launch("qwen-worker-one")
	select {
	case <-checks:
	case <-time.After(time.Second):
		t.Fatal("first launch did not reach final GPU check")
	}
	second := launch("qwen-worker-two")
	secondEntered := false
	select {
	case <-checks:
		secondEntered = true
	case <-time.After(50 * time.Millisecond):
	}
	releaseChecks <- struct{}{}
	if !secondEntered {
		select {
		case <-checks:
		case <-time.After(time.Second):
			t.Fatal("second launch did not enter after first admission completed")
		}
	}
	releaseChecks <- struct{}{}
	if err := <-first; err != nil {
		t.Fatalf("first launch failed: %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("second launch failed: %v", err)
	}
	if secondEntered {
		t.Fatal("concurrent Qwen launch entered the final GPU check before the first docker run completed")
	}
}

func qwenPageCacheFixture(t *testing.T) (string, map[string]qwen38SnapshotObject) {
	t.Helper()
	repository := filepath.Join(t.TempDir(), bkc.Qwen38FlashNextHFCacheDirectory)
	snapshot := filepath.Join(repository, "snapshots", bkc.Qwen38FlashNextNVFP4Revision)
	blobs := filepath.Join(repository, "blobs")
	if err := os.MkdirAll(snapshot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(blobs, 0700); err != nil {
		t.Fatal(err)
	}
	content := []byte("weight-data")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	if err := os.WriteFile(filepath.Join(blobs, digest), content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "blobs", digest), filepath.Join(snapshot, "model.safetensors")); err != nil {
		t.Fatal(err)
	}
	return repository, map[string]qwen38SnapshotObject{"model.safetensors": {blob: digest, size: int64(len(content))}}
}

func assertPageCacheReleaseRefusesObject(t *testing.T, repository string, objects map[string]qwen38SnapshotObject) {
	t.Helper()
	req := pinnedQwen38PatchRequest(bkc.MultiDeviceRoleWorker)
	req.Volumes = map[string]string{repository: bkc.Qwen38FlashNextContainerRoot + ":ro"}
	calls := 0
	report := releaseQwen38PageCacheWithObjects(req, objects, func(*os.File) error {
		calls++
		return nil
	})
	if calls != 0 || report.Files != 0 || report.Failure == "" || report.Skipped != report.Expected {
		t.Fatalf("unsafe weight object reached page-cache release: calls=%d report=%#v", calls, report)
	}
}
