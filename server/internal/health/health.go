package health

import (
	"log"
	"net/http"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	log.Printf("%s %s %d", r.Method, r.URL.Path, http.StatusOK)
}
