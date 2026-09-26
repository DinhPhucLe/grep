package metrics_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cortisol-cli/metrics"
)

func awaitSnapshot(t *testing.T, updates <-chan metrics.Snapshot, matches func(metrics.Snapshot) bool) metrics.Snapshot {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case snapshot, ok := <-updates:
			if !ok {
				t.Fatal("watcher closed before expected update")
			}
			if matches(snapshot) {
				return snapshot
			}
		case <-deadline.C:
			t.Fatal("timed out waiting for metrics update")
		}
	}
}

func replaceFile(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.CreateTemp(filepath.Dir(path), "snapshot-*.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(file.Name()) })
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		t.Fatal(err)
	}
}

func TestWatchMissingInvalidAndReplacementSnapshots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := metrics.Watch(ctx, path, 2*time.Millisecond)
	awaitSnapshot(t, updates, func(s metrics.Snapshot) bool {
		return s.Score == nil && s.RiskLabel == "Waiting for metrics"
	})

	replaceFile(t, path, `{"score":0,"riskLabel":"Provided zero","trend":"stable"}`)
	awaitSnapshot(t, updates, func(s metrics.Snapshot) bool {
		return s.Score != nil && *s.Score == 0 && s.RiskLabel == "Provided zero" && s.Trend == "stable"
	})

	// Replacement snapshots do not inherit optional fields from the previous one.
	replaceFile(t, path, `{"score":73}`)
	awaitSnapshot(t, updates, func(s metrics.Snapshot) bool {
		return s.Score != nil && *s.Score == 73 && s.RiskLabel == "" && s.Trend == "" && s.Delta == nil
	})

	replaceFile(t, path, `{"score":`)
	awaitSnapshot(t, updates, func(s metrics.Snapshot) bool {
		return s.Score == nil && s.RiskLabel == "Invalid metrics" && s.Trend == "" && s.Delta == nil
	})

	replaceFile(t, path, `{"score":15,"trend":"rising","delta":-9}`)
	awaitSnapshot(t, updates, func(s metrics.Snapshot) bool {
		return s.Score != nil && *s.Score == 15 && s.Trend == "rising" && s.Delta != nil && *s.Delta == -9
	})

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	awaitSnapshot(t, updates, func(s metrics.Snapshot) bool {
		return s.Score == nil && s.RiskLabel == "Waiting for metrics"
	})
	cancel()
	awaitClosed(t, updates)
}

func TestWatchRejectsNonRegularPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := metrics.Watch(ctx, t.TempDir(), 2*time.Millisecond)
	awaitSnapshot(t, updates, func(s metrics.Snapshot) bool {
		return s.Score == nil && s.RiskLabel == "Metrics unavailable"
	})
}

func awaitClosed(t *testing.T, updates <-chan metrics.Snapshot) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case _, ok := <-updates:
			if !ok {
				return
			}
		case <-deadline.C:
			t.Fatal("watcher did not close after cancellation")
		}
	}
}

func TestWatchBoundedUnreadChannelAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")
	replaceFile(t, path, `{"score":42}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := metrics.Watch(ctx, path, time.Millisecond)
	if cap(updates) != 1 {
		t.Fatalf("channel capacity = %d; want one latest update", cap(updates))
	}

	// Leave the channel unread while the watcher publishes. Polling len does not
	// free a send slot, so a consumer cannot accidentally rescue a blocked send.
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for len(updates) == 0 {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("watcher never queued its first update")
		}
	}
	cancel()
	awaitClosed(t, updates)
}

func TestWatchAlreadyCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	updates := metrics.Watch(ctx, filepath.Join(t.TempDir(), "missing.json"), time.Millisecond)
	awaitClosed(t, updates)
}
