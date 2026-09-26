// Package metrics defines the presentation values supplied by a metric producer.
// It intentionally contains no metric, risk, or trend calculations.
package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
)

// Snapshot is a complete replacement for the terminal's status display.
// A nil Score means unknown, including before the first producer update.
// Trend and Delta describe actual history supplied by the producer. RiskLabel
// can override the terminal presentation adapter's temporary default label.
type Snapshot struct {
	Score     *float64 `json:"score"`
	RiskLabel string   `json:"riskLabel"`
	Trend     string   `json:"trend"`
	Delta     *float64 `json:"delta,omitempty"`
}

const MaxSnapshotBytes = 64 * 1024

// Validate checks display input without assigning meaning to the values.
func (s Snapshot) Validate() error {
	if s.Score != nil && (math.IsNaN(*s.Score) || math.IsInf(*s.Score, 0) || *s.Score < 0 || *s.Score > 100) {
		return errors.New("score must be null or a finite number from 0 to 100")
	}
	if s.Delta != nil && (math.IsNaN(*s.Delta) || math.IsInf(*s.Delta, 0)) {
		return errors.New("delta must be a finite number")
	}
	switch s.Trend {
	case "", "unknown", "rising", "falling", "stable":
	default:
		return errors.New("trend must be unknown, rising, falling, stable, or empty")
	}
	if len(s.RiskLabel) > 1024 {
		return errors.New("riskLabel must be at most 1024 bytes")
	}
	return nil
}

// Decode reads exactly one bounded JSON object. Missing values stay unknown.
func Decode(r io.Reader) (Snapshot, error) {
	var s Snapshot
	data, err := io.ReadAll(io.LimitReader(r, MaxSnapshotBytes+1))
	if err != nil {
		return s, fmt.Errorf("read metrics: %w", err)
	}
	if len(data) > MaxSnapshotBytes {
		return s, errors.New("metrics snapshot exceeds 64 KiB")
	}
	data = bytes.TrimSpace(data)
	// encoding/json accepts null for structs; require an object explicitly.
	if len(data) == 0 || data[0] != '{' {
		return s, errors.New("metrics snapshot must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&s); err != nil {
		return Snapshot{}, fmt.Errorf("decode metrics: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Snapshot{}, errors.New("expected exactly one metrics snapshot")
	}
	if err := s.Validate(); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}

// Clone detaches pointer fields so a queued snapshot owns its display values.
func (s Snapshot) Clone() Snapshot {
	if s.Score != nil {
		v := *s.Score
		s.Score = &v
	}
	if s.Delta != nil {
		v := *s.Delta
		s.Delta = &v
	}
	return s
}
