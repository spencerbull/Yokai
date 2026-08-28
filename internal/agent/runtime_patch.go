package agent

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/deployments"
)

type sglangRuntimePatchSpec struct {
	label           string
	path            string
	originalLine    string
	replacementLine string
	originalSHA256  string
	patchedSHA256   string
}

var glm53FlashRuntimePatchSpecs = []sglangRuntimePatchSpec{
	{
		label:           bkc.GLM53FlashRuntimePatchLabel,
		path:            bkc.GLM53FlashRuntimePatchPath,
		originalLine:    bkc.GLM53FlashRuntimePatchOldLine,
		replacementLine: bkc.GLM53FlashRuntimePatchNewLine,
		originalSHA256:  bkc.GLM53FlashRuntimePatchOldSHA,
		patchedSHA256:   bkc.GLM53FlashRuntimePatchNewSHA,
	},
	{
		label:           bkc.GLM53FlashDSAGB10TilePatchLabel,
		path:            bkc.GLM53FlashDSAGB10TilePatchPath,
		originalLine:    bkc.GLM53FlashDSAGB10TilePatchOldLine,
		replacementLine: bkc.GLM53FlashDSAGB10TilePatchNewLine,
		originalSHA256:  bkc.GLM53FlashDSAGB10TilePatchOldSHA,
		patchedSHA256:   bkc.GLM53FlashDSAGB10TilePatchNewSHA,
	},
}

func applyPinnedSGLangRuntimePatch(req *ContainerRequest) error {
	patchLabel := strings.TrimSpace(req.Labels[LabelRuntimePatch])
	if patchLabel == "" {
		if req.Labels[LabelBKCID] == bkc.GLM53FlashNVFP4DualGB10ID {
			return fmt.Errorf("pinned GLM-5.3-Flash BKC is missing its required runtime patch label")
		}
		return nil
	}
	if patchLabel != bkc.GLM53FlashRuntimePatchSetLabel() ||
		req.Labels[LabelBKCID] != bkc.GLM53FlashNVFP4DualGB10ID ||
		req.Image != bkc.GLM53FlashNVFP4Image ||
		req.Labels[LabelImageDigest] != bkc.GLM53FlashNVFP4ImageDigest ||
		req.Labels[LabelModelRevision] != bkc.GLM53FlashNVFP4Revision {
		return fmt.Errorf("runtime patch provenance does not match the pinned GLM-5.3-Flash BKC")
	}
	if req.Model != bkc.GLM53FlashNVFP4Model && req.Model != deployments.FixedLocalModelPath {
		return fmt.Errorf("runtime patch model does not match the pinned GLM-5.3-Flash model or fixed local snapshot")
	}

	command := append([]string(nil), strings.Fields(req.ExtraArgs)...)
	command = append(command, req.Args...)
	if len(command) < 2 || command[0] != "sglang" || command[1] != "serve" || argSequenceCount(command, "sglang", "serve") != 1 {
		return fmt.Errorf("pinned GLM-5.3-Flash runtime patch requires exactly one sglang serve command prefix")
	}
	if revision, count := tokenFlagValueCount(command, "--revision"); count != 1 || revision != bkc.GLM53FlashNVFP4Revision {
		return fmt.Errorf("pinned GLM-5.3-Flash runtime patch requires exactly one pinned model revision")
	}
	if modelPath, count := tokenFlagValueCount(command, "--model-path"); count != 1 || modelPath != req.Model {
		return fmt.Errorf("pinned GLM-5.3-Flash runtime patch requires exactly one model path matching the request")
	}
	if tpSize, count := tokenFlagValueCount(command, "--tp-size"); count != 1 || tpSize != "2" {
		return fmt.Errorf("pinned GLM-5.3-Flash DSA tile requires exactly one TP=2 setting")
	}

	script := buildSGLangRuntimePatchScript(glm53FlashRuntimePatchSpecs)
	if strings.ContainsAny(script, "\x00\r\n") {
		return fmt.Errorf("invalid pinned GLM-5.3-Flash runtime patch bootstrap")
	}
	req.ExtraArgs = ""
	req.Args = append([]string{"python3", "-c", script}, command...)
	return nil
}

func buildSGLangRuntimePatchScript(specs []sglangRuntimePatchSpec) string {
	quote := strconv.Quote
	requiredSharedMemory := strconv.Itoa(bkc.GLM53FlashTileSharedMemoryBytes)
	statements := []string{
		"import hashlib,os,pathlib,sys",
		"digest=lambda value:hashlib.sha256(value).hexdigest()",
		"line_count=lambda value,line:value.splitlines().count(line)",
	}
	for index, spec := range specs {
		suffix := strconv.Itoa(index)
		statements = append(statements,
			"p"+suffix+"=pathlib.Path("+quote(spec.path)+")",
			"old"+suffix+"=b"+quote(spec.originalLine),
			"new"+suffix+"=b"+quote(spec.replacementLine),
			"data"+suffix+"=p"+suffix+".read_bytes()",
			"current"+suffix+"=digest(data"+suffix+")",
			"original_ok"+suffix+"=current"+suffix+"=="+quote(spec.originalSHA256)+" and line_count(data"+suffix+",old"+suffix+")==1 and line_count(data"+suffix+",new"+suffix+")==0",
			"patched_ok"+suffix+"=current"+suffix+"=="+quote(spec.patchedSHA256)+" and line_count(data"+suffix+",new"+suffix+")==1 and line_count(data"+suffix+",old"+suffix+")==0",
			"(original_ok"+suffix+" or patched_ok"+suffix+") or sys.exit("+quote("refusing unexpected SGLang runtime patch source: "+spec.label)+")",
			"candidate"+suffix+"=data"+suffix+".replace(old"+suffix+",new"+suffix+",1) if original_ok"+suffix+" else data"+suffix,
			"(digest(candidate"+suffix+")=="+quote(spec.patchedSHA256)+" and line_count(candidate"+suffix+",new"+suffix+")==1 and line_count(candidate"+suffix+",old"+suffix+")==0) or sys.exit("+quote("refusing unexpected SGLang runtime patch result: "+spec.label)+")",
		)
	}
	statements = append(statements,
		"import torch",
		"properties=torch.cuda.get_device_properties(0)",
		"limit=getattr(properties,'shared_memory_per_block_optin',None)",
		"type(limit) is int or sys.exit("+quote("unable to read the GB10 opt-in shared-memory limit")+")",
		"sys.stderr.write("+quote("GLM-5.3-Flash TP=2 tile=32/1/128 shared-memory required="+requiredSharedMemory+" observed=")+"+str(limit)+"+quote("\n")+")",
		"sys.stderr.flush()",
		"limit>="+requiredSharedMemory+" or sys.exit(1)",
	)
	for index, spec := range specs {
		suffix := strconv.Itoa(index)
		statements = append(statements,
			"written"+suffix+"=p"+suffix+".write_bytes(candidate"+suffix+") if original_ok"+suffix+" else len(candidate"+suffix+")",
			"written"+suffix+"==len(candidate"+suffix+") or sys.exit("+quote("incomplete SGLang runtime patch write: "+spec.label)+")",
		)
	}
	for index, spec := range specs {
		suffix := strconv.Itoa(index)
		statements = append(statements,
			"verified"+suffix+"=p"+suffix+".read_bytes()",
			"(digest(verified"+suffix+")=="+quote(spec.patchedSHA256)+" and line_count(verified"+suffix+",new"+suffix+")==1 and line_count(verified"+suffix+",old"+suffix+")==0) or sys.exit("+quote("SGLang runtime patch verification failed: "+spec.label)+")",
		)
	}
	statements = append(statements,
		"len(sys.argv)>1 or sys.exit('missing SGLang command')",
		"os.execvp(sys.argv[1],sys.argv[1:])",
	)
	return strings.Join(statements, ";")
}

func argSequenceCount(args []string, sequence ...string) int {
	count := 0
	for start := 0; start+len(sequence) <= len(args); start++ {
		matched := true
		for offset := range sequence {
			if args[start+offset] != sequence[offset] {
				matched = false
				break
			}
		}
		if matched {
			count++
		}
	}
	return count
}

func tokenFlagValueCount(args []string, flag string) (string, int) {
	value := ""
	count := 0
	for index := 0; index < len(args); index++ {
		switch {
		case args[index] == flag:
			count++
			if index+1 < len(args) {
				value = args[index+1]
				index++
			}
		case strings.HasPrefix(args[index], flag+"="):
			count++
			value = strings.TrimPrefix(args[index], flag+"=")
		}
	}
	return value, count
}
