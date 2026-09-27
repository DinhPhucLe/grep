package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type sessionCredentials struct {
	Token        string    `json:"token"`
	ExpiresAt    time.Time `json:"expiresAt"`
	UserID       string    `json:"userId"`
	Name         string    `json:"name"`
	GitHubLogin  string    `json:"githubLogin"`
	OrgID        string    `json:"organizationId"`
	OrgName      string    `json:"organizationName"`
	ServerURL    string    `json:"serverUrl"`
}

func credentialsPath() string {
	if p := os.Getenv("CORTISOL_CREDENTIALS"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cortisol", "credentials")
}

func loadCredentials() (sessionCredentials, error) {
	path := credentialsPath()
	if path == "" {
		return sessionCredentials{}, errors.New("no home directory")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionCredentials{}, err
	}
	var creds sessionCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return sessionCredentials{}, err
	}
	if creds.Token == "" {
		return sessionCredentials{}, errors.New("empty token")
	}
	if !creds.ExpiresAt.IsZero() && time.Now().After(creds.ExpiresAt) {
		return sessionCredentials{}, errors.New("session expired")
	}
	return creds, nil
}

func saveCredentials(creds sessionCredentials) error {
	path := credentialsPath()
	if path == "" {
		return errors.New("no home directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func clearCredentials() error {
	path := credentialsPath()
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
