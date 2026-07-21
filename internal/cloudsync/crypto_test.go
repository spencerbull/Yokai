package cloudsync

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"

	"github.com/spencerbull/yokai/internal/config"
)

const testPassphrase = "correct horse battery staple"

func testSnapshot() Snapshot {
	return Snapshot{
		Version: SnapshotVersion,
		Devices: []config.Device{{
			ID:         "finn",
			Host:       "100.64.0.2",
			AgentToken: "secret-agent-token",
			AgentPort:  7474,
		}},
	}
}

func TestEncryptDecryptRoundTripAndEnvelopeContract(t *testing.T) {
	t.Parallel()
	envelope, err := Encrypt(testSnapshot(), testPassphrase)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if err := validateEnvelope(envelope); err != nil {
		t.Fatalf("validateEnvelope() error = %v", err)
	}
	if len(envelope.Salt) != 22 || len(envelope.Nonce) != 16 {
		t.Fatalf("encoded salt/nonce lengths = %d/%d", len(envelope.Salt), len(envelope.Nonce))
	}
	if len(envelope.Ciphertext) < 56 || len(envelope.Ciphertext) > maxEncodedCiphertextSize {
		t.Fatalf("encoded ciphertext length = %d", len(envelope.Ciphertext))
	}
	if strings.Contains(envelope.Ciphertext, "secret-agent-token") {
		t.Fatal("Encrypt() exposed plaintext in ciphertext")
	}

	decrypted, err := Decrypt(envelope, testPassphrase)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if len(decrypted.Devices) != 1 || decrypted.Devices[0].AgentToken != "secret-agent-token" {
		t.Fatalf("Decrypt() = %#v", decrypted)
	}
}

func TestEncryptUsesFreshSaltNonceAndCiphertext(t *testing.T) {
	t.Parallel()
	first, err := Encrypt(testSnapshot(), testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encrypt(testSnapshot(), testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if first.Salt == second.Salt || first.Nonce == second.Nonce || first.Ciphertext == second.Ciphertext {
		t.Fatalf("repeated encryption reused random material: first=%#v second=%#v", first, second)
	}
}

func TestDecryptRejectsWrongPassphraseAndAuthenticatedFieldTampering(t *testing.T) {
	envelope, err := Encrypt(testSnapshot(), testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(envelope, "wrong passphrase value"); err == nil {
		t.Fatal("Decrypt() accepted wrong passphrase")
	}

	for _, field := range []string{"salt", "nonce", "ciphertext"} {
		t.Run(field, func(t *testing.T) {
			modified := envelope
			switch field {
			case "salt":
				modified.Salt = flipBase64Character(modified.Salt)
			case "nonce":
				modified.Nonce = flipBase64Character(modified.Nonce)
			case "ciphertext":
				modified.Ciphertext = flipBase64Character(modified.Ciphertext)
			}
			if _, err := Decrypt(modified, testPassphrase); err == nil {
				t.Fatalf("Decrypt() accepted modified %s", field)
			}
		})
	}
}

func flipBase64Character(value string) string {
	replacement := byte('A')
	if value[0] == replacement {
		replacement = 'B'
	}
	return string(replacement) + value[1:]
}

func TestEnvelopeValidationRejectsEveryMalformedField(t *testing.T) {
	valid, err := Encrypt(Snapshot{Version: SnapshotVersion, Devices: []config.Device{}}, testPassphrase)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*Envelope)
	}{
		{name: "version", mutate: func(value *Envelope) { value.Version = 2 }},
		{name: "cipher", mutate: func(value *Envelope) { value.Cipher = "plaintext" }},
		{name: "kdf", mutate: func(value *Envelope) { value.KDF = "argon2id" }},
		{name: "empty salt", mutate: func(value *Envelope) { value.Salt = "" }},
		{name: "short salt", mutate: func(value *Envelope) { value.Salt = value.Salt[:21] }},
		{name: "long salt", mutate: func(value *Envelope) { value.Salt += "A" }},
		{name: "padded salt", mutate: func(value *Envelope) { value.Salt = value.Salt[:21] + "=" }},
		{name: "noncanonical salt", mutate: func(value *Envelope) { value.Salt = value.Salt[:21] + "B" }},
		{name: "empty nonce", mutate: func(value *Envelope) { value.Nonce = "" }},
		{name: "short nonce", mutate: func(value *Envelope) { value.Nonce = value.Nonce[:15] }},
		{name: "long nonce", mutate: func(value *Envelope) { value.Nonce += "A" }},
		{name: "padded nonce", mutate: func(value *Envelope) { value.Nonce = value.Nonce[:15] + "=" }},
		{name: "empty ciphertext", mutate: func(value *Envelope) { value.Ciphertext = "" }},
		{name: "short ciphertext", mutate: func(value *Envelope) { value.Ciphertext = strings.Repeat("A", 55) }},
		{name: "invalid ciphertext length", mutate: func(value *Envelope) { value.Ciphertext = strings.Repeat("A", 57) }},
		{name: "padded ciphertext", mutate: func(value *Envelope) { value.Ciphertext = strings.Repeat("A", 55) + "=" }},
		{name: "oversized ciphertext", mutate: func(value *Envelope) { value.Ciphertext = strings.Repeat("A", maxEncodedCiphertextSize+1) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modified := valid
			test.mutate(&modified)
			if err := validateEnvelope(modified); err == nil {
				t.Fatal("validateEnvelope() accepted malformed envelope")
			}
			if _, err := Decrypt(modified, testPassphrase); err == nil {
				t.Fatal("Decrypt() accepted malformed envelope")
			}
		})
	}
}

func TestEnvelopeValidationAcceptsCiphertextBoundaries(t *testing.T) {
	t.Parallel()
	valid, err := Encrypt(Snapshot{Version: SnapshotVersion, Devices: []config.Device{}}, testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{base64.RawStdEncoding.EncodedLen(minCiphertextSize), maxEncodedCiphertextSize} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			envelope := valid
			envelope.Ciphertext = strings.Repeat("A", size)
			if err := validateEnvelope(envelope); err != nil {
				t.Fatalf("validateEnvelope() rejected ciphertext size %d: %v", size, err)
			}
		})
	}
}

func TestDecryptRejectsInvalidPlaintextAndSnapshotVersion(t *testing.T) {
	valid, err := Encrypt(Snapshot{Version: SnapshotVersion, Devices: []config.Device{}}, testPassphrase)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name      string
		plaintext string
	}{
		{name: "invalid JSON", plaintext: "this is deliberately not valid JSON"},
		{name: "unsupported snapshot", plaintext: `{"version":2,"devices":[]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			modified := reencryptPlaintext(t, valid, []byte(test.plaintext), testPassphrase)
			if _, err := Decrypt(modified, testPassphrase); err == nil {
				t.Fatal("Decrypt() accepted invalid plaintext")
			}
		})
	}
}

func reencryptPlaintext(t *testing.T, envelope Envelope, plaintext []byte, passphrase string) Envelope {
	t.Helper()
	salt, err := base64.RawStdEncoding.DecodeString(envelope.Salt)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	key := argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemoryKiB, argonThreads, keySize)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Ciphertext = base64.RawStdEncoding.EncodeToString(gcm.Seal(nil, nonce, plaintext, []byte("yokai-device-config-v1")))
	return envelope
}

func TestEncryptValidatesSnapshotAndPassphraseCharacters(t *testing.T) {
	t.Parallel()
	if _, err := Encrypt(Snapshot{Version: 0}, testPassphrase); err == nil {
		t.Fatal("Encrypt() accepted an unsupported snapshot version")
	}

	for _, test := range []struct {
		name       string
		passphrase string
		wantErr    bool
	}{
		{name: "11 ASCII characters", passphrase: "12345678901", wantErr: true},
		{name: "12 ASCII characters", passphrase: "123456789012"},
		{name: "11 Unicode characters", passphrase: strings.Repeat("🔒", 11), wantErr: true},
		{name: "12 Unicode characters", passphrase: strings.Repeat("🔒", 12)},
	} {
		t.Run(test.name, func(t *testing.T) {
			envelope, err := Encrypt(Snapshot{Version: SnapshotVersion}, test.passphrase)
			if (err != nil) != test.wantErr {
				t.Fatalf("Encrypt() error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil {
				snapshot, err := Decrypt(envelope, test.passphrase)
				if err != nil || snapshot.Devices == nil || len(snapshot.Devices) != 0 {
					t.Fatalf("empty snapshot round trip = %#v, %v", snapshot, err)
				}
			}
		})
	}
}

func FuzzDecryptRejectsArbitraryEnvelopeWithoutPanicking(f *testing.F) {
	f.Add(1, "aes-256-gcm", "argon2id-3-65536-4", "salt", "nonce", "ciphertext")
	f.Fuzz(func(t *testing.T, version int, cipherName, kdf, salt, nonce, ciphertext string) {
		_, _ = Decrypt(Envelope{
			Version:    version,
			Cipher:     cipherName,
			KDF:        kdf,
			Salt:       salt,
			Nonce:      nonce,
			Ciphertext: ciphertext,
		}, testPassphrase)
	})
}
