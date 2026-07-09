package cloudsync

import (
	"testing"

	"github.com/spencerbull/yokai/internal/config"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Parallel()
	snapshot := Snapshot{
		Version: config.ConfigVersion,
		Devices: []config.Device{{
			ID:         "finn",
			Host:       "100.64.0.2",
			AgentToken: "secret-agent-token",
			AgentPort:  7474,
		}},
	}

	envelope, err := Encrypt(snapshot, "correct horse battery staple")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if envelope.Ciphertext == "" || envelope.Ciphertext == "secret-agent-token" {
		t.Fatal("Encrypt() did not produce ciphertext")
	}
	decrypted, err := Decrypt(envelope, "correct horse battery staple")
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if len(decrypted.Devices) != 1 || decrypted.Devices[0].AgentToken != "secret-agent-token" {
		t.Fatalf("Decrypt() = %#v", decrypted)
	}
}

func TestDecryptRejectsWrongPassphraseAndTampering(t *testing.T) {
	t.Parallel()
	envelope, err := Encrypt(Snapshot{Version: config.ConfigVersion, Devices: []config.Device{}}, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(envelope, "wrong passphrase value"); err == nil {
		t.Fatal("Decrypt() accepted wrong passphrase")
	}

	envelope.Ciphertext = envelope.Ciphertext[:len(envelope.Ciphertext)-1] + "A"
	if _, err := Decrypt(envelope, "correct horse battery staple"); err == nil {
		t.Fatal("Decrypt() accepted modified ciphertext")
	}
}

func TestEncryptRequiresStrongPassphrase(t *testing.T) {
	t.Parallel()
	if _, err := Encrypt(Snapshot{}, "too-short"); err == nil {
		t.Fatal("Encrypt() accepted a short passphrase")
	}
}
