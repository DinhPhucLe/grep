package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"cortisol-server/internal/timing"
)

func TestQueueRetainsRequestTimingContext(t *testing.T) {
	var output bytes.Buffer
	ctx := timing.ForRequest(timing.New(context.Background(), timing.JSONSink(&output)), "quizzes/start")
	queue := NewQueue(1, 1, func(context.Context, int) (int, error) { return 1, nil })
	defer queue.Close()
	if _, err := queue.Submit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var waiting, working timing.Event
	if decoder.Decode(&waiting) != nil || decoder.Decode(&working) != nil {
		t.Fatal("missing queue timings")
	}
	if waiting.Stage != "queue.wait" || working.Stage != "queue.work" || waiting.RequestID == "" || waiting.RequestID != working.RequestID || waiting.Operation != "quizzes/start" {
		t.Fatalf("lost correlation: %+v %+v", waiting, working)
	}
}
