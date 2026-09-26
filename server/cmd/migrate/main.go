package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"cortisol-server/internal/db"
	"github.com/joho/godotenv"
)

func main() {
	// REPORT ERRORS AFTER MIGRATION RESOURCES HAVE BEEN RELEASED
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// CONFIGURE FILE LOCATIONS RELATIVE TO THE CURRENT DIRECTORY
	envFile := flag.String("env", ".env", "environment file to load")
	directory := flag.String("path", "internal/db/migrations", "directory containing JSON migration files")
	check := flag.Bool("check", false, "validate migration files without connecting to MongoDB")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: this command only applies up migrations")
	}
	if err := db.CheckMigrations(*directory); err != nil {
		return err
	}
	if *check {
		log.Println("Migration files are valid; no database connection was made")
		return nil
	}

	// LOAD THE SAME URI, CREDENTIAL PLACEHOLDERS, AND DATABASE AS THE SERVER
	if err := godotenv.Load(*envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	config, err := db.ConfigFromEnv()
	if err != nil {
		return err
	}

	// APPLY PENDING MIGRATIONS TO ATLAS AND PRINT THE RESULTING VERSION
	log.Printf("Applying migrations to database %q", config.Database)
	version, err := db.Migrate(config, *directory)
	if err != nil {
		return err
	}
	log.Printf("Database %q is at migration version %d", config.Database, version)
	return nil
}
