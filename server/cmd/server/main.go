package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"cortisol-server/internal/cortex"
	"cortisol-server/internal/db"
	"cortisol-server/internal/evaluation"
	"cortisol-server/internal/health"
	"cortisol-server/internal/jobs"
	"cortisol-server/internal/quiz"
	"cortisol-server/internal/timing"

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
	cortexConfig, err := cortex.ConfigFromEnv()
	if err != nil {
		return err
	}
	cortexClient, err := cortex.NewClient(cortexConfig)
	if err != nil {
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
	service := evaluation.NewService(cortexClient, evaluation.NewMongoRepository(database), cortexConfig.Model)
	mux := http.NewServeMux()

	mux.HandleFunc("/health", health.Handler)
	handler := evaluation.NewHandler(service, cortexConfig.Timeout)
	mux.HandleFunc("/evaluations", timing.HTTP("evaluations", handler))
	mux.HandleFunc("/jobs", timing.HTTP("jobs", handler))
	quizService := quiz.NewService(cortexClient, cortexConfig.Model)
	quizGenerator := quiz.NewGenerator(quizService)
	defer quizGenerator.Close()
	mux.HandleFunc("/quizzes", timing.HTTP("quizzes", quiz.NewHandler(quizGenerator, cortexConfig.Timeout)))
	answerQueue := jobs.NewQueue(4, 100, quizService.Grade)
	defer answerQueue.Close()
	mux.HandleFunc("/quiz-answers", timing.HTTP("quiz-answers", quiz.NewAnswerHandler(answerQueue, cortexConfig.Timeout)))

	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: cortexConfig.Timeout + 15*time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("server listening on %s", address)
	return server.ListenAndServe()
}
