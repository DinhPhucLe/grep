package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func NewHandler(q *Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "use POST", http.StatusMethodNotAllowed)
			return
		}

		var input struct {
			Input string `json:"input"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "expected JSON with an input string (maximum 1 MiB)", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "expected one JSON object", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(input.Input) == "" {
			http.Error(w, "input must not be empty", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		result, err := q.Submit(ctx, input.Input)
		switch {
		case errors.Is(err, ErrQueueFull):
			// Job queue is full.
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		case errors.Is(err, context.DeadlineExceeded):
			// Job timed out.
			http.Error(w, "job timed out", http.StatusGatewayTimeout)
		case errors.Is(err, context.Canceled):
			// Job was canceled.
			return
		case err != nil:
			// Job failed with an unexpected error.
			http.Error(w, "job failed", http.StatusInternalServerError)
		default:
			log.Printf("job completed successfully with result: %s", result.Output)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
		}
	}
}
