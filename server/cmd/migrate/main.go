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
	force := flag.Int("force", -1, "reset a dirty migration to the immediately preceding version without running migrations")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: this command only applies up migrations")
	}
	if err := db.CheckMigrations(*directory); err != nil {
		return err
	}
	if *check {
		if *force >= 0 {
			return fmt.Errorf("-check and -force cannot be used together")
		}
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
	if *force >= 0 {
		log.Printf("Resetting dirty migration state to version %d in database %q", *force, config.Database)
		if err := db.ForceDirtyVersion(config, *directory, uint(*force)); err != nil {
			return err
		}
		log.Printf("Dirty migration state reset to version %d; run this command again to apply pending migrations", *force)
		return nil
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
