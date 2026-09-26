package metrics_test

import (
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	"cortisol-cli/metrics"
)

func number(v float64) *float64 { return &v }

func TestDecodePreservesProducerValues(t *testing.T) {
	tests := []struct {
		name, input string
		want        metrics.Snapshot
	}{
		{"missing values remain unknown", `{}`, metrics.Snapshot{}},
		{"explicit unknown", `{"score":null,"trend":"unknown"}`, metrics.Snapshot{Trend: "unknown"}},
		{"zero is known", `{"score":0,"delta":0}`, metrics.Snapshot{Score: number(0), Delta: number(0)}},
		{"no inferred label or trend", `{"score":99}`, metrics.Snapshot{Score: number(99)}},
		{"independent producer fields", `{"score":12.5,"riskLabel":"Producer label","trend":"falling","delta":8.25}`, metrics.Snapshot{Score: number(12.5), RiskLabel: "Producer label", Trend: "falling", Delta: number(8.25)}},
		{"control text is sanitized by renderer", `{"riskLabel":"hello\u001b[31m\nworld"}`, metrics.Snapshot{RiskLabel: "hello\x1b[31m\nworld"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := metrics.Decode(strings.NewReader(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			if !sameNumber(got.Score, tt.want.Score) || !sameNumber(got.Delta, tt.want.Delta) || got.RiskLabel != tt.want.RiskLabel || got.Trend != tt.want.Trend {
				t.Fatalf("Decode(%s) = %+v; want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func sameNumber(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func TestDecodeRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{
		``, `null`, `[]`, `17`, `"value"`, `{`,
		`{"score":-0.1}`, `{"score":100.1}`, `{"score":"42"}`,
		`{"delta":1e999}`, `{"score":1e999}`, `{"trend":"up"}`,
		`{"unexpected":true}`, `{} {}`, `{} null`, `{} trailing`,
		`{"riskLabel":"` + strings.Repeat("x", 1025) + `"}`,
	} {
		t.Run(input[:min(len(input), 60)], func(t *testing.T) {
			if _, err := metrics.Decode(strings.NewReader(input)); err == nil {
				t.Fatalf("Decode(%q) unexpectedly succeeded", input)
			}
		})
	}
}

func TestDecodeSizeBoundary(t *testing.T) {
	input := `{}` + strings.Repeat(" ", metrics.MaxSnapshotBytes-2)
	if _, err := metrics.Decode(strings.NewReader(input)); err != nil {
		t.Fatalf("exactly MaxSnapshotBytes should be allowed: %v", err)
	}
	if _, err := metrics.Decode(strings.NewReader(input + " ")); err == nil {
		t.Fatal("input exceeding MaxSnapshotBytes was accepted")
	}
	label := `{"riskLabel":"` + strings.Repeat("x", 1024) + `"}`
	if _, err := metrics.Decode(strings.NewReader(label)); err != nil {
		t.Fatalf("maximum permitted label should be allowed: %v", err)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func TestDecodePropagatesReadError(t *testing.T) {
	want := io.ErrUnexpectedEOF
	if _, err := metrics.Decode(errorReader{err: want}); !errors.Is(err, want) {
		t.Fatalf("Decode error = %v; want underlying %v", err, want)
	}
}

func TestValidateRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := (metrics.Snapshot{Score: number(value)}).Validate(); err == nil {
			t.Errorf("score %v was accepted", value)
		}
		if err := (metrics.Snapshot{Delta: number(value)}).Validate(); err == nil {
			t.Errorf("delta %v was accepted", value)
		}
	}
	for _, value := range []float64{0, 100} {
		if err := (metrics.Snapshot{Score: number(value)}).Validate(); err != nil {
			t.Errorf("valid score %v rejected: %v", value, err)
		}
	}
}

func TestCloneOwnsNumericValues(t *testing.T) {
	source := metrics.Snapshot{Score: number(42), Delta: number(-3), RiskLabel: "External", Trend: "falling"}
	cloned := source.Clone()
	*source.Score = 80
	*source.Delta = 5
	if *cloned.Score != 42 || *cloned.Delta != -3 || cloned.RiskLabel != "External" || cloned.Trend != "falling" {
		t.Fatalf("clone changed when producer mutated original: %+v", cloned)
	}
	if got := (metrics.Snapshot{}).Clone(); got.Score != nil || got.Delta != nil {
		t.Fatalf("cloning unknown values created numbers: %+v", got)
	}
}
