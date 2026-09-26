package main

import (
	"log"
	"net/http"

	"cortisol-server/internal/health"
	"cortisol-server/internal/jobs"
)

func main() {
	queue := jobs.NewQueue(4, 100)
	mux := http.NewServeMux()

	mux.HandleFunc("/health", health.Handler)
	mux.HandleFunc("/jobs", jobs.NewHandler(queue))

	log.Println("server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
