package agent

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/launchauth"
)

var (
	errCoordinatorAuthorizationUnavailable = errors.New("coordinator launch authorization is not configured")
	errUnexpectedLaunchAuthorization       = errors.New("launch authorization is only valid for the coordinated Qwen3.8 BKC")
)

func authorizeCoordinatedQwenLaunch(req ContainerRequest, now time.Time, publicKey ed25519.PublicKey, localDeviceID string, replayStore *launchauth.ReplayStore) error {
	if !isQwen38CandidateRequest(req) {
		if strings.TrimSpace(req.LaunchAuthorization) != "" {
			return errUnexpectedLaunchAuthorization
		}
		return nil
	}
	if len(publicKey) != ed25519.PublicKeySize || strings.TrimSpace(localDeviceID) == "" || replayStore == nil {
		return errCoordinatorAuthorizationUnavailable
	}
	if strings.TrimSpace(req.LaunchAuthorization) == "" {
		return fmt.Errorf("coordinator launch authorization is required")
	}
	if req.Labels[LabelBKCID] != bkc.Qwen38FlashNextNVFP4DualGB10ID {
		return fmt.Errorf("coordinated Qwen3.8 BKC provenance is required")
	}
	if !hasCompleteManagedCandidateProvenance(req) {
		return fmt.Errorf("Qwen3.8 dual-node ranks must carry complete coordinated deployment provenance")
	}
	if req.HFToken != "" {
		return fmt.Errorf("coordinated Qwen3.8 launches do not accept an HF token")
	}
	generation, err := strconv.Atoi(req.Labels[LabelGeneration])
	if err != nil || generation <= 0 {
		return fmt.Errorf("coordinated deployment generation is invalid")
	}
	role := req.Labels[LabelRole]
	if role != bkc.MultiDeviceRoleHead && role != bkc.MultiDeviceRoleWorker {
		return fmt.Errorf("coordinated deployment role is invalid")
	}
	for _, key := range []string{LabelDeploymentID, LabelModelRevision, LabelSourceRevision, LabelImageDigest} {
		if strings.TrimSpace(req.Labels[key]) == "" {
			return fmt.Errorf("coordinated deployment provenance %s is required", key)
		}
	}

	claims, err := launchauth.Verify(publicKey, req.LaunchAuthorization, now)
	if err != nil {
		return err
	}
	if claims.TargetDeviceID != localDeviceID {
		return fmt.Errorf("launch authorization target device does not match this agent")
	}
	digest, err := launchauth.CandidateDigest(launchAuthorizationRequest(req))
	if err != nil {
		return err
	}
	expectedName := normalizedContainerName(req.Name)
	if claims.DeploymentID != req.Labels[LabelDeploymentID] ||
		claims.Generation != generation ||
		claims.CandidateName != expectedName ||
		claims.Role != role ||
		claims.BKCID != req.Labels[LabelBKCID] ||
		claims.Model != req.Model ||
		claims.ModelRevision != req.Labels[LabelModelRevision] ||
		claims.SourceRevision != req.Labels[LabelSourceRevision] ||
		claims.Image != req.Image ||
		claims.ImageDigest != req.Labels[LabelImageDigest] ||
		claims.CandidateDigest != digest {
		return fmt.Errorf("launch authorization does not match the exact candidate request")
	}
	if err := replayStore.Consume(claims.ID, time.Unix(claims.ExpiresAt, 0), now); err != nil {
		return err
	}
	return nil
}

func isQwen38CandidateRequest(req ContainerRequest) bool {
	if req.Labels[LabelBKCID] == bkc.Qwen38FlashNextNVFP4DualGB10ID || req.Image == bkc.Qwen38FlashNextNVFP4Image || req.Model == bkc.Qwen38FlashNextNVFP4Model || req.Model == bkc.Qwen38FlashNextContainerModel {
		return true
	}
	return req.Labels[LabelModelRevision] == bkc.Qwen38FlashNextNVFP4Revision ||
		req.Labels[LabelSourceRevision] == bkc.Qwen38FlashNextSourceRevision ||
		req.Labels[LabelImageDigest] == bkc.Qwen38FlashNextNVFP4ImageDigest ||
		req.Labels[LabelRuntimePatch] == bkc.Qwen38RuntimePatchSetLabel()
}

func launchAuthorizationRequest(req ContainerRequest) launchauth.Request {
	return launchauth.Request{
		Image: req.Image, Name: req.Name, Model: req.Model, GGUFVariant: req.GGUFVariant, GGUFFiles: req.GGUFFiles,
		HFToken: req.HFToken, Ports: req.Ports, Env: req.Env, GPUIDs: req.GPUIDs, ExtraArgs: req.ExtraArgs, Args: req.Args,
		Volumes: req.Volumes, Plugins: req.Plugins, Runtime: req.Runtime, SkipPull: req.SkipPull, Labels: req.Labels,
		NetworkMode: req.NetworkMode, Devices: req.Devices, CapAdd: req.CapAdd, Entrypoint: req.Entrypoint,
	}
}
