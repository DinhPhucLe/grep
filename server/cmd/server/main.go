package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"

	"cortisol-server/internal/db"
	"cortisol-server/internal/health"
	"cortisol-server/internal/jobs"

	"github.com/joho/godotenv"
)

func main() {
	// REPORT ERRORS AFTER RUN HAS RELEASED ITS DATABASE CONNECTION
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// LOAD ENVIRONMENT VARIABLES FROM .ENV FILE
	err := godotenv.Load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// SET UP MONGODB CLIENT OPTIONS
	config, err := db.ConfigFromEnv()
	if err != nil {
		return err
	}

	// CONNECT, PING, AND SELECT THE APPLICATION DATABASE
	database, err := db.Connect(context.Background(), config)
	if err != nil {
		return err
	}

	// RELEASE THE CLIENT WHEN RUN RETURNS
	defer func() {
		if err := db.Disconnect(database); err != nil {
			log.Printf("MongoDB disconnect failed: %v", err)
		}
	}()
	log.Printf("MongoDB connected; selected database %q", database.Name())

	// INITIALIZE JOB QUEUE AND HTTP SERVER
	queue := jobs.NewQueue(4, 100)
	mux := http.NewServeMux()

	mux.HandleFunc("/health", health.Handler)
	mux.HandleFunc("/jobs", jobs.NewHandler(queue))

	log.Println("server listening on :8080")
	return http.ListenAndServe(":8080", mux)
}
