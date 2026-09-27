package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"time"

	"cortisol-server/internal/cortex"
)

type Evaluator interface {
	Evaluate(context.Context, Request) (Record, error)
}

func NewHandler(service Evaluator, timeout time.Duration) http.HandlerFunc {
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
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var input Request
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
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
		record, err := service.Evaluate(ctx, input)
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			writeError(w, 504, "evaluation_timeout", "evaluation timed out")
		case errors.Is(err, context.Canceled):
			return
		case errors.Is(err, cortex.ErrUpstream):
			var upstream *cortex.UpstreamError
			if errors.As(err, &upstream) {
				log.Printf("Cortex upstream failure: HTTP %d, Snowflake code %s", upstream.Status, upstream.Code)
				if upstream.Code == "390432" {
					writeError(w, 502, "cortex_network_policy_required", "Snowflake requires a network policy for the PAT user; allow the server's public outbound IP")
					return
				}
			} else {
				log.Printf("Cortex upstream failure")
			}
			writeError(w, 502, "cortex_error", "Cortex could not produce a valid evaluation")
		case errors.Is(err, cortex.ErrInvalidResponse):
			log.Printf("Cortex invalid response")
			writeError(w, 502, "cortex_invalid_response", "Cortex returned an invalid evaluation")
		case err != nil:
			writeError(w, 500, "evaluation_failed", "evaluation could not be completed")
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(record)
		}
	}
}
func requestError(w http.ResponseWriter, err error) {
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		writeError(w, 413, "payload_too_large", "maximum request size is 1 MiB")
		return
	}
	writeError(w, 400, "invalid_request", "expected one evaluation JSON object with supported fields")
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
