package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mongodb"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// VALIDATE MIGRATION FILES BEFORE MAKING DATABASE CHANGES
func CheckMigrations(directory string) error {
	files, err := filepath.Glob(filepath.Join(directory, "*.up.json"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no up migrations found in %s", directory)
	}

	for _, up := range files {
		down := up[:len(up)-len("up.json")] + "down.json"
		for _, path := range []string{up, down} {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var commands []map[string]json.RawMessage
			if err := json.Unmarshal(data, &commands); err != nil {
				return fmt.Errorf("invalid migration %s: %w", path, err)
			}
			if len(commands) == 0 {
				return fmt.Errorf("migration %s must contain commands", path)
			}
			for _, command := range commands {
				if len(command) == 0 {
					return fmt.Errorf("migration %s contains an empty command", path)
				}
			}
		}
	}

	// LET GOLANG-MIGRATE CHECK FILE NAMES AND DUPLICATE VERSIONS
	source, err := iofs.New(os.DirFS(directory), ".")
	if err != nil {
		return err
	}
	defer source.Close()
	_, err = source.First()
	return err
}

// APPLY ONLY PENDING UP MIGRATIONS USING GOLANG-MIGRATE
// The migration library currently requires the v1 MongoDB driver. The HTTP
// server continues using v2; client types are not shared between the drivers.
func Migrate(config Config, directory string) (uint, error) {
	if err := CheckMigrations(directory); err != nil {
		return 0, err
	}
	source, err := iofs.New(os.DirFS(directory), ".")
	if err != nil {
		return 0, err
	}
	defer source.Close()

	// CONNECT WITHOUT PUTTING CREDENTIALS IN COMMAND ARGUMENTS
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(config.URI).
		SetConnectTimeout(10*time.Second).SetServerSelectionTimeout(10*time.Second).
		SetTimeout(30*time.Second))
	if err != nil {
		return 0, errors.New("cannot initialize migration client: check MongoDB configuration")
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = client.Disconnect(cleanupCtx)
	}()
	if err := client.Ping(ctx, nil); err != nil {
		return 0, fmt.Errorf("connect to MongoDB for migrations: %w", err)
	}

	// USE THE CONFIGURED DATABASE AND ENABLE MIGRATION LOCKING
	driver, err := mongodb.WithInstance(client, &mongodb.Config{
		DatabaseName: config.Database,
		Locking:      mongodb.Locking{Enabled: true},
	})
	if err != nil {
		return 0, fmt.Errorf("initialize migration tracking: %w", err)
	}
	runner, err := migrate.NewWithInstance("iofs", source, "mongodb", driver)
	if err != nil {
		return 0, err
	}
	// SOURCE AND CLIENT ARE CLOSED BY THE DEFERS ABOVE, INCLUDING ERROR PATHS
	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("apply migrations (inspect partial changes before retrying a dirty version): %w", err)
	}

	// REPORT THE APPLIED VERSION WITHOUT AUTOMATICALLY FORCING OR ROLLING BACK
	version, dirty, err := runner.Version()
	if err != nil {
		return 0, err
	}
	if dirty {
		return version, fmt.Errorf("migration version %d is dirty", version)
	}
	return version, nil
}
