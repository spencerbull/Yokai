package cloudsync

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
)

func TestLoadCredentialsReportsLoggedOutState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := LoadCredentials()
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("LoadCredentials() error = %v", err)
	}
}

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

func TestConcurrentCredentialSavesRemainAtomic(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	const writers = 16
	errors := make(chan error, writers)
	var wait sync.WaitGroup
	for i := 0; i < writers; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			uid := fmt.Sprintf("user-%d", index)
			errors <- SaveCredentials(&Credentials{
				ProjectID: "yokai-test",
				APIKey:    "firebase-api-key",
				UID:       uid,
				Token:     StoredToken{IDToken: "id-" + uid, RefreshToken: "refresh-" + uid},
			})
		}(i)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("SaveCredentials() error = %v", err)
		}
	}

	loaded, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}
	if loaded.Token.IDToken != "id-"+loaded.UID || loaded.Token.RefreshToken != "refresh-"+loaded.UID {
		t.Fatalf("credentials contain mixed writes: %#v", loaded)
	}
}
