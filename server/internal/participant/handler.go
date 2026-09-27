package participant

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"
)

func NewHandler(repo Repository, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeError(w, 405, "method_not_allowed", "use POST")
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			writeError(w, 415, "unsupported_media_type", "use application/json")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var input Registration
		if err := decoder.Decode(&input); err != nil {
			requestError(w, err)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			requestError(w, err)
			return
		}
		if err := input.Validate(); err != nil {
			writeError(w, 400, "invalid_request", err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		profile, err := repo.Register(ctx, input)
		switch {
		case errors.Is(err, ErrMigrationRequired):
			writeError(w, 503, "migration_required", "participant storage is not ready; a database migration is required")
		case errors.Is(err, context.DeadlineExceeded):
			writeError(w, 504, "registration_timeout", "participant registration timed out")
		case errors.Is(err, context.Canceled):
			return
		case err != nil:
			writeError(w, 500, "registration_failed", "participant registration could not be completed")
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(profile)
		}
	}
}

func requestError(w http.ResponseWriter, err error) {
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		writeError(w, 413, "payload_too_large", "maximum request size is 4 KiB")
		return
	}
	writeError(w, 400, "invalid_request", "expected one participant JSON object with supported fields")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
