package metrics

import (
	"context"
	"os"
	"time"
)

// Watch polls a producer-owned JSON snapshot file without touching agent stdin.
// Only the latest snapshot is retained; a slow consumer never blocks the poller.
// Missing/invalid input clears the display instead of showing a stale score.
// Producers should replace the file atomically to avoid partial writes.
func Watch(ctx context.Context, path string, interval time.Duration) <-chan Snapshot {
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	updates := make(chan Snapshot, 1)
	go func() {
		defer close(updates)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			s := readFile(path)
			select {
			case <-ctx.Done():
				return
			default:
			}
			select {
			case updates <- s:
			default:
				select {
				case <-updates:
				default:
				}
				select {
				case updates <- s:
				case <-ctx.Done():
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return updates
}

func readFile(path string) Snapshot {
	info, err := os.Stat(path)
	if err != nil {
		return Snapshot{RiskLabel: "Waiting for metrics"}
	}
	if !info.Mode().IsRegular() {
		return Snapshot{RiskLabel: "Metrics unavailable"}
	}
	f, err := os.Open(path)
	if err != nil {
		return Snapshot{RiskLabel: "Waiting for metrics"}
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return Snapshot{RiskLabel: "Metrics unavailable"}
	}
	s, err := Decode(f)
	if err != nil {
		return Snapshot{RiskLabel: "Invalid metrics"}
	}
	return s
}
