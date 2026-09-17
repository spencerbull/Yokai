package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spencerbull/yokai/internal/bkc"
)

type pageCacheReleaseReport struct {
	Attempted bool
	Expected  int
	Verified  int
	Files     int
	Bytes     int64
	Skipped   int
	Failure   string
}

var adviseDropPageCacheForLaunch = adviseDropPageCache
var releaseQwen38PageCacheForLaunch = releaseQwen38PageCache

func (report pageCacheReleaseReport) String() string {
	summary := fmt.Sprintf("verified %d of %d expected weight file(s); released %d B across %d file(s)", report.Verified, report.Expected, report.Bytes, report.Files)
	if report.Skipped != 0 {
		summary += fmt.Sprintf(", %d skipped", report.Skipped)
	}
	if report.Failure != "" {
		return "not completed: " + report.Failure + "; " + summary
	}
	return summary
}

func releaseQwen38PageCache(req ContainerRequest, advise func(*os.File) error) pageCacheReleaseReport {
	return releaseQwen38PageCacheWithObjects(req, qwen38SnapshotObjects, advise)
}

func releaseQwen38PageCacheWithObjects(req ContainerRequest, objects map[string]qwen38SnapshotObject, advise func(*os.File) error) pageCacheReleaseReport {
	report := pageCacheReleaseReport{Attempted: req.Labels[LabelBKCID] == bkc.Qwen38FlashNextNVFP4DualGB10ID}
	if !report.Attempted {
		return report
	}
	repository := ""
	for host, target := range req.Volumes {
		if target == bkc.Qwen38FlashNextContainerRoot+":ro" {
			if repository != "" {
				report.Failure = "multiple model repository mounts"
				return report
			}
			repository = filepath.Clean(host)
		}
	}
	if repository == "" || !filepath.IsAbs(repository) || filepath.Base(repository) != bkc.Qwen38FlashNextHFCacheDirectory || req.Labels[LabelModelRevision] != bkc.Qwen38FlashNextNVFP4Revision {
		report.Failure = "validated pinned snapshot mount is unavailable"
		return report
	}
	snapshot := filepath.Join(repository, "snapshots", bkc.Qwen38FlashNextNVFP4Revision)
	resolvedRoot, err := filepath.EvalSymlinks(repository)
	if err != nil {
		report.Failure = "cannot resolve validated pinned repository"
		return report
	}
	names := make([]string, 0, len(objects))
	for name := range objects {
		if !strings.HasSuffix(name, ".safetensors") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	report.Expected = len(names)
	if report.Expected == 0 {
		report.Failure = "pinned manifest contains no expected weight files"
		return report
	}
	type verifiedWeight struct {
		file *os.File
		size int64
	}
	verified := make([]verifiedWeight, 0, len(names))
	closeVerified := func() {
		for _, weight := range verified {
			_ = weight.file.Close()
		}
	}
	for _, name := range names {
		object := objects[name]
		resolved, err := resolveQwen38SnapshotObject(snapshot, resolvedRoot, name, object)
		if err != nil {
			closeVerified()
			report.Skipped = report.Expected
			report.Failure = err.Error()
			return report
		}
		file, err := openPageCacheBlob(resolved)
		if err != nil {
			closeVerified()
			report.Skipped = report.Expected
			report.Failure = fmt.Sprintf("open pinned weight object %s safely: %v", name, err)
			return report
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() != object.size {
			_ = file.Close()
			closeVerified()
			report.Skipped = report.Expected
			report.Failure = fmt.Sprintf("pinned weight object %s changed type or size before cache release", name)
			return report
		}
		if err := verifyQwen38SnapshotObjectFile(file, object); err != nil {
			_ = file.Close()
			closeVerified()
			report.Skipped = report.Expected
			report.Failure = fmt.Sprintf("pinned weight object %s hash mismatch before cache release: %v", name, err)
			return report
		}
		report.Verified++
		verified = append(verified, verifiedWeight{file: file, size: info.Size()})
	}
	for _, weight := range verified {
		adviseErr := advise(weight.file)
		closeErr := weight.file.Close()
		if adviseErr != nil || closeErr != nil {
			report.Skipped++
			continue
		}
		report.Files++
		report.Bytes += weight.size
	}
	return report
}
