package main

import (
	"context"
	"os"

	"cortisol-server/internal/practice"
)

// emitPracticeInstance posts a completed practice instance when CORTISOL_SERVER_URL is set.
func emitPracticeInstance(ctx context.Context, payload practice.IngestPayload) error {
	base := os.Getenv("CORTISOL_SERVER_URL")
	if base == "" {
		return nil
	}
	_, err := (&practice.Client{BaseURL: base}).PostEvent(ctx, payload)
	return err
}
