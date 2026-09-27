package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"cortisol-server/internal/practice"
)

func TestEmitPracticeInstancePostsWhenConfigured(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(practice.Event{Practice: practice.PracticeLeadAndReveal})
	}))
	defer server.Close()
	t.Setenv("CORTISOL_SERVER_URL", server.URL)
	if err := emitPracticeInstance(context.Background(), practice.IngestPayload{
		Practice: practice.PracticeLeadAndReveal,
	}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected POST")
	}
}

func TestEmitPracticeInstanceSkipsWithoutURL(t *testing.T) {
	_ = os.Unsetenv("CORTISOL_SERVER_URL")
	if err := emitPracticeInstance(context.Background(), practice.IngestPayload{}); err != nil {
		t.Fatal(err)
	}
}
