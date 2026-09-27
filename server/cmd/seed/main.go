package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"cortisol-server/internal/db"
	"cortisol-server/internal/db/seed"
	"github.com/joho/godotenv"
)

func main() {
	// REPORT ERRORS AFTER DATABASE CLEANUP
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// LOAD CONFIGURATION FROM THE SAME ENVIRONMENT FILE AS THE SERVER
	envFile := flag.String("env", ".env", "environment file to load")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected arguments; use -env to select an environment file")
	}
	if err := godotenv.Load(*envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	config, err := db.ConfigFromEnv()
	if err != nil {
		return err
	}

	// CONNECT WITH A BOUNDED COMMAND LIFETIME AND ALWAYS RELEASE THE CLIENT
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	database, err := db.Connect(ctx, config)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Disconnect(database); err != nil {
			log.Printf("MongoDB disconnect failed: %v", err)
		}
	}()

	// INSERT THE LINKED DEVELOPMENT RECORDS AND REPORT WHAT CHANGED
	log.Printf("Seeding demo data into database %q", database.Name())
	result, err := seed.Seed(ctx, database)
	if err != nil {
		return fmt.Errorf("seed stopped after %d inserts (%d existing): %w", result.Inserted, result.Existing, err)
	}
	log.Printf("Seed complete: %d inserted, %d already existed", result.Inserted, result.Existing)

	cast := seed.DemoCastIDs()
	log.Printf("Demo cast (use these IDs in practice API / dashboard routes):")
	log.Printf("  NovaPay org:      %s", cast.NovaPayOrgID.Hex())
	log.Printf("  AtlasHealth org:  %s", cast.AtlasHealthOrgID.Hex())
	log.Printf("  Alex Rivera:      %s  (NovaPay, high performer)", cast.AlexRiveraID.Hex())
	log.Printf("  Jordan Kim:       %s  (NovaPay, improving)", cast.JordanKimID.Hex())
	log.Printf("  Sam Okonkwo:      %s  (AtlasHealth, high volume)", cast.SamOkonkwoID.Hex())
	log.Printf("Employee view: GET /api/v1/dashboard/people/{userId}/practices/lead_and_reveal?year=2026")
	log.Printf("Org view:      GET /api/v1/dashboard/organizations/{orgId}/practices/lead_and_reveal")
	return nil
}
