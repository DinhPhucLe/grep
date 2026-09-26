package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// CONNECTION CONFIGURATION
// Config separates the cluster connection URI from the application database name.
type Config struct {
	URI      string
	Database string
}

// ConfigFromEnv reads variables already loaded by the calling command.
// MONGODB_URI can contain complete credentials or Atlas credential placeholders.
func ConfigFromEnv() (Config, error) {
	config := Config{
		URI:      os.Getenv("MONGODB_URI"),
		Database: os.Getenv("MONGODB_DATABASE"),
	}
	if strings.TrimSpace(config.URI) == "" {
		return Config{}, errors.New("MONGODB_URI is required")
	}
	if strings.TrimSpace(config.Database) == "" {
		return Config{}, errors.New("MONGODB_DATABASE is required")
	}

	// REPLACE ATLAS PLACEHOLDERS WITH URL-ESCAPED CREDENTIALS
	if strings.Contains(config.URI, "<db_username>") {
		username := os.Getenv("DB_USERNAME")
		if username == "" {
			return Config{}, errors.New("DB_USERNAME is required for the URI placeholder")
		}
		config.URI = strings.ReplaceAll(config.URI, "<db_username>", url.User(username).String())
	}
	if strings.Contains(config.URI, "<db_password>") {
		password := os.Getenv("DB_PASSWORD")
		if password == "" {
			return Config{}, errors.New("DB_PASSWORD is required for the URI placeholder")
		}
		escapedPassword := strings.TrimPrefix(url.UserPassword("", password).String(), ":")
		config.URI = strings.ReplaceAll(config.URI, "<db_password>", escapedPassword)
	}
	return config, nil
}

// CONNECT AND VERIFY ACCESS TO THE CLUSTER
// Connect returns a database handle backed by a shared, pooled MongoDB client.
// Selecting a database does not create collections or apply migrations.
func Connect(ctx context.Context, config Config) (*mongo.Database, error) {
	if strings.TrimSpace(config.URI) == "" || strings.TrimSpace(config.Database) == "" {
		return nil, errors.New("MongoDB URI and database name are required")
	}
	client, err := mongo.Connect(options.Client().ApplyURI(config.URI))
	if err != nil {
		// Driver errors may contain connection details; keep credentials out of logs.
		return nil, errors.New("cannot initialize MongoDB client: check MONGODB_URI")
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		// CLEAN UP THE CLIENT IF THE CONNECTION CHECK FAILS
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = client.Disconnect(cleanupCtx)
		return nil, fmt.Errorf("ping MongoDB: %w", err)
	}

	// SELECT THE APPLICATION DATABASE
	return client.Database(config.Database), nil
}

// CLOSE THE SHARED CLIENT WITH A FRESH CLEANUP TIMEOUT
func Disconnect(database *mongo.Database) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return database.Client().Disconnect(ctx)
}
