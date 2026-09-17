package launchauth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSignedAuthorizationVerifiesAndContainsNoAPIKey(t *testing.T) {
	publicKey, privateKey, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	request := testRequest("--api-key=sentinel-request-key")
	claims, err := NewClaims(now, request, "spark-a", "dep-1", 4, "yokai-deployment-dep-1-g4-head", "head", "qwen-bkc", "model-rev", "source-rev", "image-digest")
	if err != nil {
		t.Fatal(err)
	}
	token, err := Sign(privateKey, claims)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(token, "sentinel-request-key") {
		t.Fatal("authorization contains the request API key")
	}
	parts := strings.Split(token, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "sentinel-request-key") || strings.Contains(string(payload), "api-key") {
		t.Fatalf("authorization claims contain API-key material: %s", payload)
	}
	verified, err := Verify(publicKey, token, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if verified.TargetDeviceID != "spark-a" || verified.DeploymentID != "dep-1" || verified.Generation != 4 || verified.Role != "head" {
		t.Fatalf("unexpected verified claims: %#v", verified)
	}
}

func TestAuthorizationRejectsTamperingAndExpiration(t *testing.T) {
	publicKey, privateKey, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	claims, err := NewClaims(now, testRequest("--api-key=one"), "spark-a", "dep-1", 1, "candidate", "worker", "qwen-bkc", "model-rev", "source-rev", "image-digest")
	if err != nil {
		t.Fatal(err)
	}
	token, err := Sign(privateKey, claims)
	if err != nil {
		t.Fatal(err)
	}
	index := len(token) / 2
	replacement := byte('A')
	if token[index] == replacement {
		replacement = 'B'
	}
	tampered := token[:index] + string(replacement) + token[index+1:]
	if _, err := Verify(publicKey, tampered, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tampered authorization was not rejected: %v", err)
	}
	if _, err := Verify(publicKey, token, now.Add(Lifetime)); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired authorization was not rejected: %v", err)
	}
}

func TestAuthorizationRequiresSignedTargetDeviceAndRejectsTargetTampering(t *testing.T) {
	publicKey, privateKey, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	claims, err := NewClaims(now, testRequest("--api-key=one"), "spark-a", "dep-1", 1, "candidate", "worker", "qwen-bkc", "model-rev", "source-rev", "image-digest")
	if err != nil {
		t.Fatal(err)
	}
	token, err := Sign(privateKey, claims)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded["target_device_id"] = "spark-b"
	tamperedPayload, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString(tamperedPayload) + "." + parts[2]
	if _, err := Verify(publicKey, tampered, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("target-device tampering was not rejected: %v", err)
	}

	claims.TargetDeviceID = ""
	missingTarget, err := Sign(privateKey, claims)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(publicKey, missingTarget, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing signed target device was not rejected: %v", err)
	}
}

func TestCandidateDigestBindsCandidateButNotAPIKey(t *testing.T) {
	first := testRequest("--api-key=one")
	second := testRequest("--api-key=two")
	firstDigest, err := CandidateDigest(first)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := CandidateDigest(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatal("request-scoped API key changed the authorization digest")
	}
	second.Name = "different-candidate"
	differentDigest, err := CandidateDigest(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest == differentDigest {
		t.Fatal("candidate identity did not change the authorization digest")
	}
}

func TestReplayStoreSurvivesReconstruction(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "agent.json")
	now := time.Unix(1_800_000_000, 0)
	store := NewReplayStore(configPath)
	if err := store.Consume(strings.Repeat("a", 32), now.Add(Lifetime), now); err != nil {
		t.Fatal(err)
	}
	restartedStore := NewReplayStore(configPath)
	if err := restartedStore.Consume(strings.Repeat("a", 32), now.Add(Lifetime), now); !errors.Is(err, ErrReplay) {
		t.Fatalf("reconstructed store accepted a replay: %v", err)
	}
}

func TestSigningKeyFileIsStableAndPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coordinator-signing-key.json")
	first, err := LoadOrCreateSigningKey(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateSigningKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Equal(second) {
		t.Fatal("coordinator signing key changed across loads")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("signing key mode is %o, want 600", info.Mode().Perm())
	}
}

func TestPublicKeyFingerprintIsStableAndRejectsInvalidKeys(t *testing.T) {
	publicKey := ed25519.PublicKey(make([]byte, ed25519.PublicKeySize))
	fingerprint, err := PublicKeyFingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	const want = "sha256:66687aadf862bd776c8fc18b8e9f8e20089714856ee233b3902a591d0d5f2925"
	if fingerprint != want {
		t.Fatalf("public-key fingerprint=%q want %q", fingerprint, want)
	}
	if _, err := PublicKeyFingerprint(publicKey[:ed25519.PublicKeySize-1]); err == nil {
		t.Fatal("invalid Ed25519 public key produced a fingerprint")
	}
}

func testRequest(apiKeyArg string) Request {
	return Request{
		Image: "image@sha256:digest", Name: "candidate", Model: "model", Ports: map[string]string{"8888": "8888"},
		Env: map[string]string{"NCCL_NET": "IB"}, GPUIDs: "0", Args: []string{"serve", apiKeyArg},
		Volumes: map[string]string{"/host": "/container:ro"}, Labels: map[string]string{"io.yokai.bkc.id": "qwen-bkc"},
		SkipPull: true, NetworkMode: "host", Devices: []string{"/dev/infiniband:/dev/infiniband"}, CapAdd: []string{"SYS_NICE"},
	}
}
