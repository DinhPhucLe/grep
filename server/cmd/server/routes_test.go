package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalAPIRoutesNeedNoCredentials(t *testing.T) {
	mux := http.NewServeMux()
	endpoint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	registerRoutes(mux, endpoint, endpoint)
	for _, route := range []string{"/evaluations", "/quizzes"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, route, nil))
		if w.Code != http.StatusNoContent {
			t.Errorf("%s: status %d", route, w.Code)
		}
	}
}

func TestRemovedExperimentalRoutesAreUnavailable(t *testing.T) {
	mux := http.NewServeMux()
	endpoint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("removed endpoint reached application") })
	registerRoutes(mux, endpoint, endpoint)
	for _, route := range []string{"/quiz-answers", "/jobs", "/auth/login", "/auth/register"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, route, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: got %d", route, w.Code)
		}
	}
}
