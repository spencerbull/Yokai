package agent

import (
	"fmt"
	"log"
	"os"
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
// the agent's local models directory. It returns the container-visible path
// to the primary shard (the one that should be passed to the inference
// runtime's --model flag).
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
	for _, filename := range req.GGUFFiles {
		clean := strings.TrimSpace(filename)
		if clean == "" {
			continue
		}
		dest := filepath.Join(hostBase, filepath.Base(clean))
		log.Printf("GGUF download: %s/%s -> %s", req.Model, clean, dest)
		if err := dl.Download(req.Model, clean, dest); err != nil {
			return "", fmt.Errorf("downloading %s: %w", clean, err)
		}
	}

	primary := filepath.Base(strings.TrimSpace(req.GGUFFiles[0]))
	return filepath.Join(ggufContainerDir, subdir, primary), nil
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
