// Package cloudsync encrypts and synchronizes Yokai device configuration.
package cloudsync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"

	"github.com/spencerbull/yokai/internal/config"
)

const (
	// SnapshotVersion is independent from the full local configuration schema.
	// Increment it only when the portable device payload itself becomes incompatible.
	SnapshotVersion = 1
	envelopeVersion = 1
	keySize         = 32
	saltSize        = 16
	argonTime       = 3
	argonMemoryKiB  = 64 * 1024
	argonThreads    = 4
)

// Snapshot is the portable portion of Yokai configuration. Secrets contained
// in device records are protected by client-side encryption before upload.
type Snapshot struct {
	Version int             `json:"version"`
	Devices []config.Device `json:"devices"`
}

// Envelope is the encrypted representation stored by the cloud service.
type Envelope struct {
	Version    int    `json:"version" firestore:"version"`
	Cipher     string `json:"cipher" firestore:"cipher"`
	KDF        string `json:"kdf" firestore:"kdf"`
	Salt       string `json:"salt" firestore:"salt"`
	Nonce      string `json:"nonce" firestore:"nonce"`
	Ciphertext string `json:"ciphertext" firestore:"ciphertext"`
}

// Encrypt converts a device snapshot to an authenticated AES-256-GCM envelope
// using an Argon2id key derived from passphrase.
func Encrypt(snapshot Snapshot, passphrase string) (Envelope, error) {
	if len(passphrase) < 12 {
		return Envelope{}, fmt.Errorf("passphrase must contain at least 12 characters")
	}

	plaintext, err := json.Marshal(snapshot)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshaling device snapshot: %w", err)
	}

	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return Envelope{}, fmt.Errorf("generating encryption salt: %w", err)
	}

	key := argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemoryKiB, argonThreads, keySize)
	block, err := aes.NewCipher(key)
	if err != nil {
		return Envelope{}, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Envelope{}, fmt.Errorf("creating authenticated cipher: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Envelope{}, fmt.Errorf("generating encryption nonce: %w", err)
	}

	additionalData := []byte("yokai-device-config-v1")
	ciphertext := gcm.Seal(nil, nonce, plaintext, additionalData)
	return Envelope{
		Version:    envelopeVersion,
		Cipher:     "aes-256-gcm",
		KDF:        "argon2id-3-65536-4",
		Salt:       base64.RawStdEncoding.EncodeToString(salt),
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}, nil
}

// Decrypt authenticates and decrypts an envelope into a device snapshot.
func Decrypt(envelope Envelope, passphrase string) (Snapshot, error) {
	if envelope.Version != envelopeVersion || envelope.Cipher != "aes-256-gcm" || envelope.KDF != "argon2id-3-65536-4" {
		return Snapshot{}, fmt.Errorf("unsupported encrypted config format")
	}

	salt, err := decodeField("salt", envelope.Salt, saltSize)
	if err != nil {
		return Snapshot{}, err
	}
	nonce, err := decodeField("nonce", envelope.Nonce, 12)
	if err != nil {
		return Snapshot{}, err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) < 16 {
		return Snapshot{}, fmt.Errorf("invalid encrypted config ciphertext")
	}

	key := argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemoryKiB, argonThreads, keySize)
	block, err := aes.NewCipher(key)
	if err != nil {
		return Snapshot{}, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Snapshot{}, fmt.Errorf("creating authenticated cipher: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte("yokai-device-config-v1"))
	if err != nil {
		return Snapshot{}, fmt.Errorf("decrypting config: passphrase is incorrect or cloud data was modified")
	}

	var snapshot Snapshot
	if err := json.Unmarshal(plaintext, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("parsing decrypted config: %w", err)
	}
	if snapshot.Version != SnapshotVersion {
		return Snapshot{}, fmt.Errorf("unsupported device config version %d", snapshot.Version)
	}
	if snapshot.Devices == nil {
		snapshot.Devices = []config.Device{}
	}
	return snapshot, nil
}

func decodeField(name, value string, wantLen int) ([]byte, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || len(decoded) != wantLen {
		return nil, fmt.Errorf("invalid encrypted config %s", name)
	}
	return decoded, nil
}
