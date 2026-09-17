package launchauth

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type signingKeyFile struct {
	Version    int    `json:"version"`
	PrivateKey string `json:"private_key"`
}

func LoadOrCreateSigningKey(path string) (ed25519.PrivateKey, error) {
	key, err := loadSigningKey(path)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create coordinator key directory: %w", err)
	}
	_, generated, err := GenerateKey()
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(signingKeyFile{Version: version, PrivateKey: EncodePrivateKey(generated)})
	if err != nil {
		return nil, fmt.Errorf("marshal coordinator signing key: %w", err)
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return loadSigningKey(path)
		}
		return nil, fmt.Errorf("create coordinator signing key: %w", err)
	}
	writeErr := func() error {
		if _, err := file.Write(data); err != nil {
			return err
		}
		return file.Sync()
	}()
	closeErr := file.Close()
	if writeErr != nil {
		return nil, fmt.Errorf("write coordinator signing key: %w", writeErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close coordinator signing key: %w", closeErr)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("sync coordinator key directory: %w", err)
	}
	return generated, nil
}

func loadSigningKey(path string) (ed25519.PrivateKey, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("coordinator signing key must be a regular file accessible only by its owner")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var stored signingKeyFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil {
		return nil, fmt.Errorf("parse coordinator signing key: %w", err)
	}
	if stored.Version != version {
		return nil, fmt.Errorf("coordinator signing key has unsupported version %d", stored.Version)
	}
	return DecodePrivateKey(stored.PrivateKey)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}
