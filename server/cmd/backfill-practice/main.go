// backfill-practice projects saved graded quiz answers into practice_events.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"cortisol-server/internal/auth"
	"cortisol-server/internal/db"
	"cortisol-server/internal/practice"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	envFile := flag.String("env", ".env", "environment file")
	apply := flag.Bool("apply", false, "write events (default: validate and count only)")
	user := flag.String("user-id", "", "limit backfill to this user; empty includes all users")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	var userID bson.ObjectID
	if *user != "" {
		var err error
		userID, err = bson.ObjectIDFromHex(*user)
		if err != nil {
			return errors.New("invalid user-id")
		}
	}
	if err := godotenv.Load(*envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	config, err := db.ConfigFromEnv()
	if err != nil {
		return err
	}
	authConfig, err := auth.ConfigFromEnv()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	database, err := db.Connect(ctx, config)
	if err != nil {
		return err
	}
	defer db.Disconnect(database)
	result, err := practice.NewQuizAnswerProjector(database, authConfig.DefaultOrgID).Backfill(ctx, userID, *apply)
	if err != nil {
		return err
	}
	mode := "dry-run"
	if *apply {
		mode = "applied"
	}
	fmt.Printf("%s: %d graded answers, %d invalid records skipped; database=%s\n", mode, result.Answers, result.Skipped, database.Name())
	return nil
}
