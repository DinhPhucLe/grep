package quiz

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"cortisol-server/internal/cortex"
	"cortisol-server/internal/jobs"
)

type Submitter interface {
	Submit(context.Context, Request) (Response, error)
}

func NewHandler(queue Submitter, timeout time.Duration) http.HandlerFunc {
	return newHandler(queue, timeout, Request.Validate)
}

func newHandler[T, R any](queue interface {
	Submit(context.Context, T) (R, error)
}, timeout time.Duration, validate func(T) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
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
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var input T
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		err = decoder.Decode(&input)
		if err == nil {
			if trailing := decoder.Decode(new(any)); trailing != io.EOF {
				err = trailing
				if err == nil {
					err = errors.New("trailing JSON")
				}
			}
		}
		if err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				writeError(w, 413, "payload_too_large", "maximum request size is 1 MiB")
			} else {
				writeError(w, 400, "invalid_request", "expected one quiz JSON object with supported fields")
			}
			return
		}
		if err := validate(input); err != nil {
			writeError(w, 400, "invalid_request", err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		response, err := queue.Submit(ctx, input)
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			writeError(w, 504, "quiz_timeout", "quiz request timed out")
		case errors.Is(err, context.Canceled):
			return
		case errors.Is(err, jobs.ErrQueueFull):
			w.Header().Set("Retry-After", "1")
			writeError(w, 503, "queue_full", "quiz queue is full")
		case errors.Is(err, jobs.ErrClosed):
			writeError(w, 503, "queue_unavailable", "quiz queue is unavailable")
		case errors.Is(err, cortex.ErrUpstream):
			writeError(w, 502, "cortex_error", "Cortex could not complete the quiz request")
		case errors.Is(err, cortex.ErrInvalidResponse):
			writeError(w, 502, "cortex_invalid_response", "Cortex returned an invalid quiz")
		case err != nil:
			writeError(w, 500, "quiz_failed", "quiz request could not be completed")
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		}
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
