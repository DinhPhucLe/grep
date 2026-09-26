// Package cli launches Codex with an optional externally driven status line.
package cli

import (
	"context"
	"cortisol-cli/internal/runner"
)

// Options controls the child process and the stream of calculated display values.
// Set Gauge to true to enable the live status row. Nil streams use os.Stdin,
// os.Stdout and os.Stderr. Updates is optional: without it the display is unknown.
type Options = runner.Options

// Run returns the wrapped command's exit code after restoring terminal state.
// Canceling ctx terminates the wrapped command. The caller owns the Updates
// channel and its producer; send immutable snapshots, and cancel that producer
// when Run returns. Run never calculates metrics or consumes telemetry.
func Run(ctx context.Context, options Options) (int, error) {
	return runner.Run(ctx, options)
}
