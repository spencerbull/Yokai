package cloudsync

import (
	"os"
	"testing"
)

func TestCredentialsUsePrivatePermissions(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	credentials := &Credentials{
		ProjectID: "yokai-test",
		APIKey:    "firebase-api-key",
		UID:       "firebase-user-id",
		Token:     StoredToken{IDToken: "id-token", RefreshToken: "refresh-token"},
	}
	if err := SaveCredentials(credentials); err != nil {
		t.Fatalf("SaveCredentials() error = %v", err)
	}
	path, err := CredentialsPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("credential permissions = %o, want 600", got)
	}
	loaded, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}
	if loaded.Token.RefreshToken != credentials.Token.RefreshToken {
		t.Fatal("LoadCredentials() did not preserve refresh token")
	}
}
