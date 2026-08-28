package deployments

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

var ErrNotFound = errors.New("deployment not found")

type storeDocument struct {
	Version     int          `json:"version"`
	Deployments []Deployment `json:"deployments"`
}

type Store struct {
	path  string
	mu    sync.RWMutex
	doc   storeDocument
	write func(string, storeDocument) (bool, error)
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, doc: storeDocument{Version: StoreVersion, Deployments: []Deployment{}}, write: writeStoreAtomic}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("reading deployments store: %w", err)
	}
	if err := json.Unmarshal(data, &s.doc); err != nil {
		return nil, fmt.Errorf("parsing deployments store: %w", err)
	}
	if s.doc.Version > StoreVersion || s.doc.Version < 0 {
		return nil, fmt.Errorf("unsupported deployments store version %d (supported: %d)", s.doc.Version, StoreVersion)
	}
	// Version zero (including a missing version field) is the pre-release
	// document shape. It is read as version one and rewritten only on the next
	// ordinary state transition.
	if s.doc.Version == 0 {
		s.doc.Version = StoreVersion
	}
	if s.doc.Deployments == nil {
		s.doc.Deployments = []Deployment{}
	}
	return s, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) List() []Deployment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := cloneDeployments(s.doc.Deployments)
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}

func (s *Store) Get(id string) (Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, deployment := range s.doc.Deployments {
		if deployment.ID == id {
			return cloneDeployment(deployment), nil
		}
	}
	return Deployment{}, ErrNotFound
}

func (s *Store) FindIdempotency(key string) (Deployment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, deployment := range s.doc.Deployments {
		if deployment.IdempotencyKey == key {
			return cloneDeployment(deployment), true
		}
	}
	return Deployment{}, false
}

func (s *Store) Put(deployment Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneDeployments(s.doc.Deployments)
	replaced := false
	for index := range next {
		if next[index].ID == deployment.ID {
			next[index] = cloneDeployment(deployment)
			replaced = true
			break
		}
	}
	if !replaced {
		next = append(next, cloneDeployment(deployment))
	}
	doc := storeDocument{Version: StoreVersion, Deployments: next}
	promoted, err := s.write(s.path, doc)
	if promoted {
		// Rename already made this document visible to future processes. Keep the
		// live process aligned even when the subsequent directory durability gate
		// fails and the caller must still receive that error.
		s.doc = doc
	}
	if err != nil {
		return err
	}
	if !promoted {
		return fmt.Errorf("deployments store write completed without promotion")
	}
	return nil
}

func writeStoreAtomic(path string, doc storeDocument) (bool, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false, fmt.Errorf("creating deployments directory: %w", err)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshaling deployments store: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, ".deployments-*.tmp")
	if err != nil {
		return false, fmt.Errorf("creating deployments temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		cleanup()
		return false, fmt.Errorf("setting deployments temp permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return false, fmt.Errorf("writing deployments store: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return false, fmt.Errorf("syncing deployments store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return false, fmt.Errorf("closing deployments store: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return false, fmt.Errorf("promoting deployments store: %w", err)
	}
	if err := syncStoreDirectory(dir, os.Open); err != nil {
		return true, err
	}
	return true, nil
}

func syncStoreDirectory(dir string, openDirectory func(string) (*os.File, error)) error {
	directory, err := openDirectory(dir)
	if err != nil {
		return fmt.Errorf("opening deployments directory for sync: %w", err)
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("syncing deployments directory: %w", err)
	}
	return nil
}

func cloneDeployment(src Deployment) Deployment {
	data, _ := json.Marshal(src)
	var dst Deployment
	_ = json.Unmarshal(data, &dst)
	dst.RequestHash = src.RequestHash
	return dst
}

func cloneDeployments(src []Deployment) []Deployment {
	dst := make([]Deployment, len(src))
	for index := range src {
		dst[index] = cloneDeployment(src[index])
	}
	return dst
}
