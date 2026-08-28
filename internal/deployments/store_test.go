package deployments

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncStoreDirectoryReturnsOpenFailure(t *testing.T) {
	want := errors.New("injected directory-open failure")
	err := syncStoreDirectory(t.TempDir(), func(string) (*os.File, error) {
		return nil, want
	})
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "opening deployments directory for sync") {
		t.Fatalf("directory-open failure was not returned: %v", err)
	}
}

func TestStoreAtomicRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), StoreFile)
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	want := Deployment{
		ID: "dep-1", BKCID: "recipe", IdempotencyKey: "key-1", RequestHash: "hash-1", State: StatePending,
		RuntimePatches: []RuntimePatchProvenance{
			{Label: "patch-v1", SourcePath: "/source-1.py", OriginalSHA256: "old-1", PatchedSHA256: "new-1"},
			{Label: "patch-v2", SourcePath: "/source-2.py", OriginalSHA256: "old-2", PatchedSHA256: "new-2"},
		},
		Rollback:  &RollbackResult{Attempted: true, LogTails: []RankLogTail{{Role: "worker", Rank: 1, Status: "captured", Tail: "scheduler exception", CapturedAt: time.Unix(1, 0).UTC()}}},
		CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(),
	}
	if err := store.Put(want); err != nil {
		t.Fatalf("put deployment: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("unexpected fixed temp artifact: %v", err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	got, err := reopened.Get(want.ID)
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if got.RequestHash != want.RequestHash || got.State != want.State || len(got.RuntimePatches) != 2 || got.RuntimePatches[0].Label != "patch-v1" || got.RuntimePatches[1].Label != "patch-v2" || got.Rollback == nil || len(got.Rollback.LogTails) != 1 || got.Rollback.LogTails[0].Tail != "scheduler exception" {
		t.Fatalf("round-trip mismatch: %#v", got)
	}
}

func TestStoreReadsLegacySingularRuntimePatchWithoutInventingPatchSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), StoreFile)
	legacy := `{"version":1,"deployments":[{"id":"legacy","state":"stopped","runtime_patch":{"label":"loader-only-v1","source_path":"/loader.py","original_sha256":"old","patched_sha256":"new"}}]}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open legacy store: %v", err)
	}
	got, err := store.Get("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RuntimePatches) != 1 || got.RuntimePatches[0].Label != "loader-only-v1" {
		t.Fatalf("legacy patch was misrepresented: %#v", got.RuntimePatches)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"runtime_patches":[{"label":"loader-only-v1"`) || strings.Contains(string(encoded), `"runtime_patch":`) {
		t.Fatalf("legacy API shape did not migrate honestly: %s", encoded)
	}
}

func TestStoreKeepsMemoryAlignedAfterPromotedDurabilityFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), StoreFile)
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("injected directory sync failure")
	store.write = func(_ string, _ storeDocument) (bool, error) { return true, wantErr }
	want := Deployment{ID: "promoted", State: StatePending}
	if err := store.Put(want); !errors.Is(err, wantErr) {
		t.Fatalf("durability error was not returned: %v", err)
	}
	got, err := store.Get(want.ID)
	if err != nil || got.State != want.State {
		t.Fatalf("in-memory document did not follow promoted document: got=%#v err=%v", got, err)
	}
}

func TestStoreFailsClosedOnFutureVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), StoreFile)
	if err := os.WriteFile(path, []byte(`{"version":2,"deployments":[]}`), 0600); err != nil {
		t.Fatalf("write future store: %v", err)
	}
	_, err := OpenStore(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported deployments store version 2") {
		t.Fatalf("expected future-version failure, got %v", err)
	}
}

func TestStoreAcceptsVersionZeroAsLegacyVersionOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), StoreFile)
	if err := os.WriteFile(path, []byte(`{"deployments":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(path)
	if err != nil || len(store.List()) != 0 {
		t.Fatalf("legacy version-zero store was not accepted: store=%#v err=%v", store, err)
	}
}
