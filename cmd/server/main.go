package main

import (
	"log"
	"net/http"

	"cortisol-server/internal/health"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", health.Handler)

	log.Println("server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
