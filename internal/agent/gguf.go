package agent

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spencerbull/yokai/internal/hf"
)

// ggufHostDir is the directory on the agent host where downloaded GGUF shards
// are stored. It is bind-mounted into the container as /models.
const ggufHostDir = "/var/lib/yokai/models"

// ggufContainerDir is where downloaded shards appear inside the container.
const ggufContainerDir = "/models"

// modelDirName is the unsafe-character-stripped subdirectory used to namespace
// downloads per HuggingFace repo so two different repos don't collide on a
// common filename like "model-Q4_K_M.gguf".
var unsafeNameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func modelDirName(modelID string) string {
	name := unsafeNameChars.ReplaceAllString(modelID, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		name = "model"
	}
	return name
}

// ensureGGUFFiles downloads every shard in req.GGUFFiles from HuggingFace to
// the agent's local models directory, preserving the repo's relative
// directory structure so two files with the same basename in different
// folders (common in repos that split one quant per directory) do not
// overwrite each other. It returns the container-visible path to the primary
// shard (the one that should be passed to the inference runtime's --model
// flag).
//
// Already-downloaded shards are reused in place. Empty GGUFFiles is a no-op
// and returns an empty path.
func ensureGGUFFiles(req *ContainerRequest) (string, error) {
	if len(req.GGUFFiles) == 0 {
		return "", nil
	}
	if strings.TrimSpace(req.Model) == "" {
		return "", fmt.Errorf("gguf_files specified without a model repo id")
	}

	subdir := modelDirName(req.Model)
	hostBase := filepath.Join(ggufHostDir, subdir)
	if err := os.MkdirAll(hostBase, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", hostBase, err)
	}

	dl := hf.NewDownloader(req.HFToken)
	primaryRel := ""
	for i, filename := range req.GGUFFiles {
		rel, err := sanitizeRepoPath(filename)
		if err != nil {
			return "", err
		}
		if rel == "" {
			continue
		}
		dest := filepath.Join(hostBase, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", fmt.Errorf("creating %s: %w", filepath.Dir(dest), err)
		}
		log.Printf("GGUF download: %s/%s -> %s", req.Model, rel, dest)
		if err := dl.Download(req.Model, rel, dest); err != nil {
			return "", fmt.Errorf("downloading %s: %w", rel, err)
		}
		if i == 0 {
			primaryRel = rel
		}
	}

	if primaryRel == "" {
		return "", nil
	}
	// Container path uses forward slashes regardless of the agent host OS.
	return path.Join(ggufContainerDir, subdir, primaryRel), nil
}

// sanitizeRepoPath validates and normalizes a HuggingFace repo-relative path
// before it is joined against a host directory. Rejects empty strings,
// absolute paths, `..` segments, and Windows-style drive prefixes so a
// malicious deploy request cannot escape the agent's models directory.
func sanitizeRepoPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	// HuggingFace paths are always forward-slash; reject backslashes outright
	// rather than silently normalising them.
	if strings.ContainsRune(p, '\\') {
		return "", fmt.Errorf("invalid gguf path %q: contains backslash", p)
	}
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("invalid gguf path %q: must be relative", p)
	}
	clean := path.Clean(p)
	if clean == "." || clean == "" {
		return "", fmt.Errorf("invalid gguf path %q: empty after clean", p)
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid gguf path %q: escapes repo root", p)
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == ".." {
			return "", fmt.Errorf("invalid gguf path %q: contains '..'", p)
		}
	}
	return clean, nil
}

// ensureGGUFVolume guarantees the models volume is mounted at /models. This
// is shared across llama.cpp and GGUF-based vLLM deploys so the downloaded
// shards are visible inside the container.
func ensureGGUFVolume(volumes map[string]string) {
	for _, containerPath := range volumes {
		if containerPath == ggufContainerDir {
			return
		}
	}
	volumes[ggufHostDir] = ggufContainerDir
}
