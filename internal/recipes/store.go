package recipes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// StoreVersion is the on-disk document shape version.
const StoreVersion = 1

// StoreFile is the default store filename under the yokai config directory.
const StoreFile = "recipes.json"

var ErrNotFound = errors.New("recipe not found")

type storeDocument struct {
	Version int      `json:"version"`
	Recipes []Recipe `json:"recipes"`
}

// Store persists candidate recipes to a versioned JSON file using atomic
// writes, mirroring the deployments store pattern.
type Store struct {
	path  string
	mu    sync.RWMutex
	doc   storeDocument
	write func(string, storeDocument) (bool, error)
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, doc: storeDocument{Version: StoreVersion, Recipes: []Recipe{}}, write: writeStoreAtomic}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("reading recipes store: %w", err)
	}
	if err := json.Unmarshal(data, &s.doc); err != nil {
		return nil, fmt.Errorf("parsing recipes store: %w", err)
	}
	if s.doc.Version > StoreVersion || s.doc.Version < 0 {
		return nil, fmt.Errorf("unsupported recipes store version %d (supported: %d)", s.doc.Version, StoreVersion)
	}
	if s.doc.Recipes == nil {
		s.doc.Recipes = []Recipe{}
	}
	return s, nil
}

func (s *Store) Path() string { return s.path }

// List returns all candidate recipes, newest first.
func (s *Store) List() []Recipe {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := append([]Recipe(nil), s.doc.Recipes...)
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	return items
}

func (s *Store) Get(id string) (Recipe, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.doc.Recipes {
		if r.ID == id {
			return cloneRecipe(r), true
		}
	}
	return Recipe{}, false
}

// FindFingerprint returns any stored recipe with the given config fingerprint.
func (s *Store) FindFingerprint(fp string) (Recipe, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.doc.Recipes {
		if r.Fingerprint == fp {
			return cloneRecipe(r), true
		}
	}
	return Recipe{}, false
}

// Put upserts a recipe by ID.
func (s *Store) Put(r Recipe) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := append([]Recipe(nil), s.doc.Recipes...)
	replaced := false
	for i := range next {
		if next[i].ID == r.ID {
			next[i] = cloneRecipe(r)
			replaced = true
			break
		}
	}
	if !replaced {
		next = append(next, cloneRecipe(r))
	}
	doc := storeDocument{Version: StoreVersion, Recipes: next}
	promoted, err := s.write(s.path, doc)
	if promoted {
		s.doc = doc
	}
	if err != nil {
		return err
	}
	if !promoted {
		return fmt.Errorf("recipes store write completed without promotion")
	}
	return nil
}

func cloneRecipe(r Recipe) Recipe {
	r.Config.Env = cloneMap(r.Config.Env)
	r.Config.Volumes = cloneMap(r.Config.Volumes)
	r.Config.Plugins = append([]string(nil), r.Config.Plugins...)
	r.Config.TargetDevices = append([]string(nil), r.Config.TargetDevices...)
	r.Provenance.ReportedOn = append([]string(nil), r.Provenance.ReportedOn...)
	r.ValidatedOn = append([]string(nil), r.ValidatedOn...)
	return r
}

func cloneMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func writeStoreAtomic(path string, doc storeDocument) (bool, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(dir, ".recipes-*.tmp")
	if err != nil {
		return false, err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	// Same-filesystem rename makes the new document atomically visible.
	if err := os.Rename(tmpName, path); err != nil {
		return false, err
	}
	if err := syncDir(dir); err != nil {
		return true, err // promoted; durability gate failed
	}
	return true, nil
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}
