package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cortisol-server/internal/participant"
)

// loadParticipantID preserves one anonymous identity for this configuration
// directory. Hard-link publication prevents simultaneous launches from replacing
// one another's identity or observing a partially written file.
func loadParticipantID(configDir string) (string, error) {
	dir := filepath.Join(configDir, "cortisol")
	path := filepath.Join(dir, "participant.json")
	read := func() (string, error) {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer file.Close()
		decoder := json.NewDecoder(io.LimitReader(file, 4097))
		decoder.DisallowUnknownFields()
		var identity struct {
			ID string `json:"id"`
		}
		if err := decoder.Decode(&identity); err != nil || !participant.ValidID(identity.ID) {
			return "", fmt.Errorf("invalid participant identity in %s; restore this file from backup or move it aside explicitly to create a new identity", path)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return "", fmt.Errorf("invalid participant identity in %s; restore this file from backup or move it aside explicitly to create a new identity", path)
		}
		return identity.ID, nil
	}
	if strings.TrimSpace(configDir) == "" {
		return "", errors.New("participant configuration directory is empty")
	}
	if id, err := read(); err == nil {
		return id, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read participant identity: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create participant directory: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return "", fmt.Errorf("secure participant directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".participant-*")
	if err != nil {
		return "", fmt.Errorf("create participant identity: %w", err)
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	id := participant.NewID()
	if err := json.NewEncoder(temp).Encode(struct {
		ID string `json:"id"`
	}{id}); err != nil {
		return "", fmt.Errorf("write participant identity: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return "", fmt.Errorf("sync participant identity: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close participant identity: %w", err)
	}
	if err := os.Link(temp.Name(), path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return read()
		}
		return "", fmt.Errorf("publish participant identity: %w", err)
	}
	return id, nil
}
