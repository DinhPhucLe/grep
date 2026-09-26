package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"cortisol-server/internal/health"
	"cortisol-server/internal/jobs"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func main() {
	// LOAD ENVIRONMENT VARIABLES FROM .ENV FILE
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// SET UP MONGODB CLIENT OPTIONS
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		log.Fatal("MONGODB_URI is required")
	}

	opts := options.Client().ApplyURI(
		fmt.Sprintf(
			uri,
			os.Getenv("DB_USERNAME"),
			os.Getenv("DB_PASSWORD"),
		),
	)

	client, err := mongo.Connect(opts)
	if err != nil {
		panic(err)
	}

	// CREATE DATABASE INSTANCE
	databaseName := os.Getenv("MONGODB_DATABASE")
	if databaseName == "" {
		log.Fatal("MONGODB_DATABASE is required")
	}
	// db := client.Database(databaseName)

	defer func() {
		if err = client.Disconnect(context.TODO()); err != nil {
			panic(err)
		}
	}()

	// Send a ping to confirm a successful connection
	if err := client.Ping(context.TODO(), readpref.Primary()); err != nil {
		panic(err)
	}
	fmt.Println("Pinged your deployment. You successfully connected to MongoDB!")

	// INITIALIZE JOB QUEUE AND HTTP SERVER
	queue := jobs.NewQueue(4, 100)
	mux := http.NewServeMux()

	mux.HandleFunc("/health", health.Handler)
	mux.HandleFunc("/jobs", jobs.NewHandler(queue))

	log.Println("server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
