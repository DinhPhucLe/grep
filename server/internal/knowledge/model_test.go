package knowledge

import (
	"encoding/json"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"math"
	"strings"
	"testing"
	"time"
)

func validInput() Input {
	return Input{Source: "conversation:local-1", Topic: "Deployment", Content: "  Keep this\r\nexact text — 你好\n"}
}

func TestInputValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Input)
		want   error
	}{
		{"plain text", func(v *Input) {}, nil},
		{"synthetic vector", func(v *Input) { v.Embedding = []float64{1, 0}; v.EmbeddingModel = "synthetic/test" }, nil},
		{"blank source", func(v *Input) { v.Source = "  " }, ErrInvalid},
		{"blank topic", func(v *Input) { v.Topic = "\n" }, ErrInvalid},
		{"blank content", func(v *Input) { v.Content = " \t" }, ErrInvalid},
		{"invalid UTF8", func(v *Input) { v.Content = string([]byte{255}) }, ErrInvalid},
		{"source limit", func(v *Input) { v.Source = strings.Repeat("a", 1024) }, nil},
		{"source too long", func(v *Input) { v.Source = strings.Repeat("a", 1025) }, ErrTooLarge},
		{"topic limit", func(v *Input) { v.Topic = strings.Repeat("é", 128) }, nil},
		{"topic too long", func(v *Input) { v.Topic = strings.Repeat("é", 129) }, ErrTooLarge},
		{"content limit", func(v *Input) { v.Content = strings.Repeat("é", 512*1024) }, nil},
		{"content too long", func(v *Input) { v.Content = strings.Repeat("é", 512*1024+1) }, ErrTooLarge},
		{"vector without model", func(v *Input) { v.Embedding = []float64{1} }, ErrInvalid},
		{"model without vector", func(v *Input) { v.EmbeddingModel = "synthetic/test" }, ErrInvalid},
		{"empty vector", func(v *Input) { v.Embedding = []float64{} }, ErrInvalid},
		{"empty vector with model", func(v *Input) { v.Embedding = []float64{}; v.EmbeddingModel = "synthetic/test" }, ErrInvalid},
		{"blank model", func(v *Input) { v.Embedding = []float64{1}; v.EmbeddingModel = " " }, ErrInvalid},
		{"long model", func(v *Input) { v.Embedding = []float64{1}; v.EmbeddingModel = strings.Repeat("a", 257) }, ErrTooLarge},
		{"zero vector", func(v *Input) { v.Embedding = []float64{0, 0}; v.EmbeddingModel = "synthetic/test" }, ErrInvalid},
		{"NaN", func(v *Input) { v.Embedding = []float64{math.NaN()}; v.EmbeddingModel = "synthetic/test" }, ErrInvalid},
		{"infinity", func(v *Input) { v.Embedding = []float64{math.Inf(1)}; v.EmbeddingModel = "synthetic/test" }, ErrInvalid},
		{"negative infinity", func(v *Input) { v.Embedding = []float64{math.Inf(-1)}; v.EmbeddingModel = "synthetic/test" }, ErrInvalid},
		{"tiny finite", func(v *Input) {
			v.Embedding = []float64{math.SmallestNonzeroFloat64}
			v.EmbeddingModel = "synthetic/test"
		}, nil},
		{"large finite", func(v *Input) { v.Embedding = []float64{math.MaxFloat64}; v.EmbeddingModel = "synthetic/test" }, nil},
		{"vector limit", func(v *Input) {
			v.Embedding = make([]float64, 4096)
			v.Embedding[0] = 1
			v.EmbeddingModel = "synthetic/test"
		}, nil},
		{"vector too long", func(v *Input) {
			v.Embedding = make([]float64, 4097)
			v.Embedding[0] = 1
			v.EmbeddingModel = "synthetic/test"
		}, ErrTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := validInput()
			tc.change(&v)
			if err := v.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("got %v; want %v", err, tc.want)
			}
		})
	}
}

func TestRecordRoundTrip(t *testing.T) {
	for _, embedded := range []bool{false, true} {
		input := validInput()
		if embedded {
			input.Embedding = []float64{1, -0.5}
			input.EmbeddingModel = "synthetic/test"
		}
		original := Record{ID: bson.NewObjectID(), ProjectID: bson.NewObjectID(), Source: input.Source, Topic: input.Topic, Content: input.Content, Embedding: input.Embedding, EmbeddingModel: input.EmbeddingModel, CreatedAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
		for _, format := range []string{"bson", "json"} {
			t.Run(format+map[bool]string{false: "/text", true: "/vector"}[embedded], func(t *testing.T) {
				var data []byte
				var err error
				var got Record
				if format == "bson" {
					data, err = bson.Marshal(original)
					if err == nil {
						err = bson.Unmarshal(data, &got)
					}
					raw := bson.Raw(data)
					if raw.Lookup("_id").Type != bson.TypeObjectID || raw.Lookup("project_id").Type != bson.TypeObjectID || raw.Lookup("created_at").Type != bson.TypeDateTime {
						t.Fatal("wrong BSON types")
					}
					if !embedded && (raw.Lookup("embedding").Type != 0 || raw.Lookup("embedding_model").Type != 0) {
						t.Fatal("optional fields should be omitted")
					}
				} else {
					data, err = json.Marshal(original)
					if err == nil {
						err = json.Unmarshal(data, &got)
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				if got.Content != original.Content || got.Source != original.Source || got.Topic != original.Topic || got.ID != original.ID || !got.CreatedAt.Equal(original.CreatedAt) {
					t.Fatal("record changed")
				}
				if embedded && (len(got.Embedding) != 2 || got.Embedding[1] != -0.5 || got.EmbeddingModel != "synthetic/test") {
					t.Fatal("embedding changed")
				}
			})
		}
	}
}
