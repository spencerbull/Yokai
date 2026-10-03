package launchauth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

var ErrReplay = errors.New("launch authorization was already used")

// ReplayStore consumes authorization IDs with an atomic create that survives agent restarts for the full replay window.
type ReplayStore struct {
	dir string
	mu  sync.Mutex
}

func NewReplayStore(configPath string) *ReplayStore {
	return &ReplayStore{dir: filepath.Join(filepath.Dir(configPath), "consumed-launch-authorizations")}
}

func (store *ReplayStore) Prepare() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.prepareLocked()
}

func (store *ReplayStore) Consume(id string, expiresAt time.Time, now time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.prepareLocked(); err != nil {
		return err
	}
	path := filepath.Join(store.dir, id)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrReplay
		}
		return fmt.Errorf("reserve launch authorization: %w", err)
	}
	if _, err := file.WriteString(strconv.FormatInt(expiresAt.Unix(), 10) + "\n"); err != nil {
		_ = file.Close()
		return fmt.Errorf("persist launch authorization reservation: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync launch authorization reservation: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close launch authorization reservation: %w", err)
	}
	if err := syncDirectory(store.dir); err != nil {
		return fmt.Errorf("sync launch authorization replay store: %w", err)
	}
	store.pruneExpired(now)
	return nil
}

func (store *ReplayStore) prepareLocked() error {
	if store.dir == "" {
		return fmt.Errorf("authorization replay store is not configured")
	}
	info, err := os.Lstat(store.dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(store.dir, 0700); err != nil {
			return fmt.Errorf("create authorization replay store: %w", err)
		}
		info, err = os.Lstat(store.dir)
	}
	if err != nil {
		return fmt.Errorf("inspect authorization replay store: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("authorization replay store is not a private directory")
	}
	if err := os.Chmod(store.dir, 0700); err != nil {
		return fmt.Errorf("secure authorization replay store: %w", err)
	}
	info, err = os.Lstat(store.dir)
	if err != nil {
		return fmt.Errorf("inspect authorization replay store: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return fmt.Errorf("authorization replay store is not a private directory")
	}
	return nil
}

func (store *ReplayStore) pruneExpired(now time.Time) {
	entries, err := os.ReadDir(store.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(store.dir, entry.Name()))
		if err != nil {
			continue
		}
		expiresUnix, err := strconv.ParseInt(stringTrimSpace(data), 10, 64)
		if err == nil && now.After(time.Unix(expiresUnix, 0).Add(Lifetime)) {
			_ = os.Remove(filepath.Join(store.dir, entry.Name()))
		}
	}
}

func stringTrimSpace(data []byte) string {
	start, end := 0, len(data)
	for start < end && (data[start] == ' ' || data[start] == '\n' || data[start] == '\r' || data[start] == '\t') {
		start++
	}
	for end > start && (data[end-1] == ' ' || data[end-1] == '\n' || data[end-1] == '\r' || data[end-1] == '\t') {
		end--
	}
	return string(data[start:end])
}
