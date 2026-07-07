package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const HistoryFile = "history.json"
const MaxHistoryItems = 20

// History stores recently used images and models across deploys.
type History struct {
	Images []string `json:"images"` // most-recent first
	Models []string `json:"models"` // most-recent first
}

// HistoryPath returns the full path to history.json.
func HistoryPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, HistoryFile), nil
}

// LoadHistory reads the history from disk. Returns empty history if file doesn't exist.
func LoadHistory() (*History, error) {
	path, err := HistoryPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &History{}, nil
		}
		return nil, fmt.Errorf("reading history: %w", err)
	}

	var h History
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("parsing history: %w", err)
	}

	return &h, nil
}

// SaveHistory writes the history to disk atomically.
func SaveHistory(h *History) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	path, err := HistoryPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling history: %w", err)
	}
	data = append(data, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("writing history: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("renaming history: %w (cleanup tmp failed: %v)", err, removeErr)
		}
		return fmt.Errorf("renaming history: %w", err)
	}

	return nil
}

// AddImage prepends an image to the history, deduplicating and capping at MaxHistoryItems.
func (h *History) AddImage(image string) {
	if image == "" {
		return
	}
	h.Images = prependUnique(h.Images, image, MaxHistoryItems)
}

// AddModel prepends a model to the history, deduplicating and capping at MaxHistoryItems.
func (h *History) AddModel(model string) {
	if model == "" {
		return
	}
	h.Models = prependUnique(h.Models, model, MaxHistoryItems)
}

// prependUnique adds value to the front of the slice, removes any existing
// duplicate, and caps the length at max.
func prependUnique(items []string, value string, max int) []string {
	// Remove existing occurrence
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if item != value {
			filtered = append(filtered, item)
		}
	}

	// Prepend
	result := append([]string{value}, filtered...)

	// Cap
	if len(result) > max {
		result = result[:max]
	}

	return result
}
