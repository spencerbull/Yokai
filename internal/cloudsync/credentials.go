package cloudsync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

const credentialsFile = "cloud-auth.json"

// Credentials contains the Firebase project and refresh token. The file is
// local-only, mode 0600, and is never part of a synced snapshot.
type Credentials struct {
	ProjectID string      `json:"project_id"`
	APIKey    string      `json:"api_key"`
	UID       string      `json:"uid"`
	Email     string      `json:"email,omitempty"`
	Token     StoredToken `json:"token"`
}

// StoredToken contains the Firebase ID and refresh tokens used for Firestore.
type StoredToken struct {
	IDToken      string    `json:"id_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

// CredentialsPath returns the local cloud credential path.
func CredentialsPath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, credentialsFile), nil
}

// LoadCredentials loads local Firebase credentials.
func LoadCredentials() (*Credentials, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not logged in; run 'yokai cloud login' first")
		}
		return nil, fmt.Errorf("reading cloud credentials: %w", err)
	}
	var credentials Credentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return nil, fmt.Errorf("parsing cloud credentials: %w", err)
	}
	if credentials.ProjectID == "" || credentials.APIKey == "" || credentials.UID == "" || credentials.Token.RefreshToken == "" {
		return nil, fmt.Errorf("cloud credentials are incomplete; log in again")
	}
	return &credentials, nil
}

// SaveCredentials atomically writes Firebase credentials with user-only access.
func SaveCredentials(credentials *Credentials) error {
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("securing config dir: %w", err)
	}
	path := filepath.Join(dir, credentialsFile)
	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cloud credentials: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("writing cloud credentials: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("saving cloud credentials: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("securing cloud credentials: %w", err)
	}
	return nil
}

// RemoveCredentials removes the local Firebase credential file.
func RemoveCredentials() error {
	path, err := CredentialsPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing cloud credentials: %w", err)
	}
	return nil
}
