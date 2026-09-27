package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUnratedConversationGoesDirectlyToCodex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/evaluations" {
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(201)
		fmt.Fprint(w, `{"id":"66f600000000000000000001","evaluation":{"verdict":"not_applicable","summary":"Confirmation","ambiguity_score":null,"gaps":[]}}`)
	}))
	defer server.Close()
	for _, prompt := range []string{"yes", "go ahead", "thanks", "explain that", "individually"} {
		m := newModel(nil, t.TempDir(), uiOptions{EvaluationServer: server.URL})
		m.connected, m.threadID = true, "thread"
		m.draft.SetValue(prompt)
		_, evaluate := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		_, send := m.Update(evaluate())
		if send == nil || m.quiz != nil || m.evaluating {
			t.Fatalf("conversation was blocked: %s", m.status)
		}
		if msg := send().(callDoneMsg); msg.method != "turn/start" {
			t.Fatal(msg)
		}
		if len(m.items) != 1 || m.items[0].kind != "userMessage" || m.items[0].raw != prompt {
			t.Fatal("conversation got an evaluation/quiz card or changed text")
		}
		m.Update(event("turn/completed", `{"threadId":"thread","turn":{"id":"turn","status":"completed"}}`))
		if m.quizActive() {
			t.Fatal("normal conversation started a quiz")
		}
	}
}
