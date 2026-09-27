package quiz

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInvalidQuizReportsValidationReason(t *testing.T) {
	request := exampleRequest()
	result := exampleResult()
	result.Questions[0].Evidence[0].EndLine = 999
	raw, _ := json.Marshal(result)
	input, _ := json.Marshal(request)
	r := httptest.NewRequest("POST", "/quizzes", strings.NewReader(string(input)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	NewHandler(NewService(&fakeCompleter{raw: string(raw)}, "test"), time.Second)(w, r)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "invalid code reference") {
		t.Fatalf("lost validation reason: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "Invalid credentials") {
		t.Fatal("response exposed source code")
	}
}
