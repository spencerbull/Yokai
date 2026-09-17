package launchauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

const (
	tokenPrefix = "yokai-launch-v1"
	audience    = "yokai-agent-coordinated-qwen-launch"
	version     = 1

	// Lifetime is shorter than the launch RPC because the agent consumes authorization before slow image or checkpoint work starts.
	Lifetime = 2 * time.Minute
	maxSkew  = 15 * time.Second
)

var (
	ErrInvalid = errors.New("invalid launch authorization")
	ErrExpired = errors.New("expired launch authorization")
)

// Request is the launch payload covered by CandidateDigest; API-key argv values are normalized so the authorization stores no API key or reusable representation of one.
type Request struct {
	Image       string                `json:"image"`
	Name        string                `json:"name"`
	Model       string                `json:"model"`
	GGUFVariant string                `json:"gguf_variant,omitempty"`
	GGUFFiles   []string              `json:"gguf_files,omitempty"`
	HFToken     string                `json:"hf_token,omitempty"`
	Ports       map[string]string     `json:"ports"`
	Env         map[string]string     `json:"env"`
	GPUIDs      string                `json:"gpu_ids"`
	ExtraArgs   string                `json:"extra_args"`
	Args        []string              `json:"args,omitempty"`
	Volumes     map[string]string     `json:"volumes"`
	Plugins     []string              `json:"plugins"`
	Runtime     config.RuntimeOptions `json:"runtime"`
	SkipPull    bool                  `json:"skip_pull,omitempty"`
	Labels      map[string]string     `json:"labels,omitempty"`
	NetworkMode string                `json:"network_mode,omitempty"`
	Devices     []string              `json:"devices,omitempty"`
	CapAdd      []string              `json:"cap_add,omitempty"`
	Entrypoint  string                `json:"entrypoint,omitempty"`
}

type Claims struct {
	Version         int    `json:"version"`
	Audience        string `json:"audience"`
	ID              string `json:"id"`
	TargetDeviceID  string `json:"target_device_id"`
	DeploymentID    string `json:"deployment_id"`
	Generation      int    `json:"generation"`
	CandidateName   string `json:"candidate_name"`
	Role            string `json:"role"`
	BKCID           string `json:"bkc_id"`
	Model           string `json:"model"`
	ModelRevision   string `json:"model_revision"`
	SourceRevision  string `json:"source_revision"`
	Image           string `json:"image"`
	ImageDigest     string `json:"image_digest"`
	CandidateDigest string `json:"candidate_digest"`
	IssuedAt        int64  `json:"issued_at"`
	ExpiresAt       int64  `json:"expires_at"`
}

func NewClaims(now time.Time, request Request, targetDeviceID, deploymentID string, generation int, candidateName, role, bkcID, modelRevision, sourceRevision, imageDigest string) (Claims, error) {
	if strings.TrimSpace(targetDeviceID) == "" {
		return Claims{}, fmt.Errorf("%w: target device identity is required", ErrInvalid)
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return Claims{}, fmt.Errorf("generate authorization ID: %w", err)
	}
	digest, err := CandidateDigest(request)
	if err != nil {
		return Claims{}, err
	}
	now = now.UTC()
	return Claims{
		Version: version, Audience: audience, ID: hex.EncodeToString(idBytes), TargetDeviceID: targetDeviceID, DeploymentID: deploymentID,
		Generation: generation, CandidateName: candidateName, Role: role, BKCID: bkcID, Model: request.Model,
		ModelRevision: modelRevision, SourceRevision: sourceRevision, Image: request.Image, ImageDigest: imageDigest,
		CandidateDigest: digest, IssuedAt: now.Unix(), ExpiresAt: now.Add(Lifetime).Unix(),
	}, nil
}

func CandidateDigest(request Request) (string, error) {
	request.Args = append([]string(nil), request.Args...)
	for index, arg := range request.Args {
		if strings.HasPrefix(arg, "--api-key=") {
			request.Args[index] = "--api-key=<request-scoped>"
		}
	}
	// Qwen coordinated launches never need an HF token, so keep credential-derived values out of signed tokens.
	request.HFToken = ""
	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal authorized candidate: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func Sign(privateKey ed25519.PrivateKey, claims Claims) (string, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("%w: coordinator signing key has invalid length", ErrInvalid)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal launch authorization: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	message := tokenPrefix + "." + encoded
	signature := ed25519.Sign(privateKey, []byte(message))
	return message + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func Verify(publicKey ed25519.PublicKey, token string, now time.Time) (Claims, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return Claims{}, fmt.Errorf("%w: coordinator verification key is not configured", ErrInvalid)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != tokenPrefix {
		return Claims{}, fmt.Errorf("%w: malformed token", ErrInvalid)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: malformed payload", ErrInvalid)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != ed25519.SignatureSize {
		return Claims{}, fmt.Errorf("%w: malformed signature", ErrInvalid)
	}
	if !ed25519.Verify(publicKey, []byte(parts[0]+"."+parts[1]), signature) {
		return Claims{}, fmt.Errorf("%w: signature verification failed", ErrInvalid)
	}

	var claims Claims
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return Claims{}, fmt.Errorf("%w: invalid claims", ErrInvalid)
	}
	if claims.Version != version || claims.Audience != audience || len(claims.ID) != 32 {
		return Claims{}, fmt.Errorf("%w: unsupported claims", ErrInvalid)
	}
	if _, err := hex.DecodeString(claims.ID); err != nil {
		return Claims{}, fmt.Errorf("%w: invalid authorization ID", ErrInvalid)
	}
	if strings.TrimSpace(claims.TargetDeviceID) == "" || claims.Generation <= 0 || claims.DeploymentID == "" || claims.CandidateName == "" || claims.Role == "" ||
		claims.BKCID == "" || claims.Model == "" || claims.ModelRevision == "" || claims.SourceRevision == "" ||
		claims.Image == "" || claims.ImageDigest == "" || len(claims.CandidateDigest) != sha256.Size*2 {
		return Claims{}, fmt.Errorf("%w: incomplete candidate claims", ErrInvalid)
	}
	if _, err := hex.DecodeString(claims.CandidateDigest); err != nil {
		return Claims{}, fmt.Errorf("%w: invalid candidate digest", ErrInvalid)
	}
	if claims.IssuedAt <= 0 || claims.ExpiresAt <= claims.IssuedAt || time.Duration(claims.ExpiresAt-claims.IssuedAt)*time.Second > Lifetime {
		return Claims{}, fmt.Errorf("%w: invalid lifetime", ErrInvalid)
	}
	now = now.UTC()
	if now.Before(time.Unix(claims.IssuedAt, 0).Add(-maxSkew)) {
		return Claims{}, fmt.Errorf("%w: issued in the future", ErrInvalid)
	}
	if !now.Before(time.Unix(claims.ExpiresAt, 0)) {
		return Claims{}, ErrExpired
	}
	return claims, nil
}

func GenerateKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate coordinator signing key: %w", err)
	}
	return publicKey, privateKey, nil
}

func EncodePublicKey(publicKey ed25519.PublicKey) string {
	return base64.RawStdEncoding.EncodeToString(publicKey)
}

func DecodePublicKey(encoded string) (ed25519.PublicKey, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("coordinator public key is invalid")
	}
	return ed25519.PublicKey(decoded), nil
}

// PublicKeyFingerprint returns a stable non-secret identifier for an Ed25519 verifier key.
func PublicKeyFingerprint(publicKey ed25519.PublicKey) (string, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return "", fmt.Errorf("coordinator public key is invalid")
	}
	digest := sha256.Sum256(publicKey)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func EncodePrivateKey(privateKey ed25519.PrivateKey) string {
	return base64.RawStdEncoding.EncodeToString(privateKey)
}

func DecodePrivateKey(encoded string) (ed25519.PrivateKey, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(decoded) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("coordinator private key is invalid")
	}
	return ed25519.PrivateKey(decoded), nil
}
