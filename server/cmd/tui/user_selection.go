package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"cortisol-server/internal/user"
)

type userSelection struct {
	UserID string `json:"user_id"`
}

// Remember existing IDs per workspace. This does not create a user or project.
func loadUserSelection(configDir, workspace, userID string) (userSelection, error) {
	path := filepath.Join(configDir, "cortisol", "users.json")
	selections := map[string]userSelection{}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &selections); err != nil || selections == nil {
			return userSelection{}, fmt.Errorf("invalid user selections in %s", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return userSelection{}, err
	}
	key, err := filepath.Abs(workspace)
	if err != nil {
		return userSelection{}, err
	}
	selection := selections[key]
	explicit := userID != ""
	if explicit {
		selection = userSelection{UserID: userID}
	}
	if !explicit && selection == (userSelection{}) {
		return selection, nil
	}
	if !user.ValidID(selection.UserID) {
		return userSelection{}, fmt.Errorf("select an existing MongoDB user ID with --user-id; saved selections: %s", path)
	}
	if !explicit {
		return selection, nil
	}
	selections[key] = selection
	data, err = json.MarshalIndent(selections, "", "  ")
	if err != nil {
		return userSelection{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return userSelection{}, err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".users-*")
	if err != nil {
		return userSelection{}, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return userSelection{}, err
	}
	if err := file.Sync(); err != nil {
		return userSelection{}, err
	}
	if err := file.Close(); err != nil {
		return userSelection{}, err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return userSelection{}, err
	}
	return selection, nil
}
