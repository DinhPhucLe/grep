package main

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"cortisol-server/internal/quiz"
)

func TestGradeAfterLocalAnswerAndShowPartialExplanation(t *testing.T) {
	m := generatedFilesQuizModel(t, "")
	m.quiz.result = twoQuestions()
	m.quiz.phase = "question"
	m.quiz.panel = &conversationItem{kind: "quizPanel"}
	m.quiz.request.Files = []quiz.File{{Path: "main.go", Content: "one\ntwo\nthree\nfour\n"}}
	m.grader = func(_ context.Context, _ string, q quiz.Question, _ []quiz.File, answer string) (answerGrade, error) {
		if q.ID != "q1" || answer != "partly right" {
			t.Fatal("wrong question or answer graded")
		}
		return answerGrade{Accuracy: 0.5, Explanation: "The fallback case is missing."}, nil
	}
	cmd := m.quizEnter("partly right")
	if cmd == nil || m.quiz.phase != "grading" || !m.busy {
		t.Fatal("answer did not start individual grading")
	}
	if next := m.quizEnter(""); next != nil {
		t.Fatal("advanced while grading")
	}
	m.Update(cmd())
	if m.quiz.phase != "reveal" || !strings.Contains(m.quiz.panel.raw, "Accuracy: 0.50") || !strings.Contains(m.quiz.panel.raw, "fallback case is missing") {
		t.Fatalf("partial answer feedback missing: %s", m.quiz.panel.raw)
	}
}

func TestPerfectAnswerShowsScoreWithoutExplanation(t *testing.T) {
	m := generatedFilesQuizModel(t, "")
	m.quiz.result = twoQuestions()
	m.quiz.phase = "question"
	m.quiz.panel = &conversationItem{kind: "quizPanel"}
	m.grader = func(context.Context, string, quiz.Question, []quiz.File, string) (answerGrade, error) {
		return answerGrade{Accuracy: 1, Explanation: "Do not display this."}, nil
	}
	m.Update(m.quizEnter("complete")())
	if !strings.Contains(m.quiz.panel.raw, "Accuracy: 1.00") || strings.Contains(m.quiz.panel.raw, "Do not display") {
		t.Fatal("perfect answer displayed correction")
	}
}

func TestGradingFailureNeverInventsZeroAndCanAdvance(t *testing.T) {
	m := generatedFilesQuizModel(t, "")
	m.quiz.result = twoQuestions()
	m.quiz.phase = "question"
	m.quiz.panel = &conversationItem{kind: "quizPanel"}
	m.grader = func(context.Context, string, quiz.Question, []quiz.File, string) (answerGrade, error) {
		return answerGrade{}, errors.New("provider unavailable")
	}
	m.Update(m.quizEnter("my answer")())
	if m.quiz.phase != "reveal" || !strings.Contains(m.quiz.panel.raw, "Accuracy unavailable") || strings.Contains(m.quiz.panel.raw, "Accuracy: 0") {
		t.Fatal("grade failure fabricated a score")
	}
	if next := m.quizEnter(""); next == nil {
		t.Fatal("grade failure blocked next question")
	}
}

func TestGradeValidationRejectsInvalidScoresAndMissingCorrections(t *testing.T) {
	for _, result := range []answerGrade{{Accuracy: -0.1}, {Accuracy: 1.1}, {Accuracy: math.NaN()}, {Accuracy: 0.5}} {
		if result.Validate() == nil {
			t.Fatalf("invalid grade accepted: %+v", result)
		}
	}
	if (answerGrade{Accuracy: 1}).Validate() != nil {
		t.Fatal("perfect answer needs no explanation")
	}
}

func TestParseAnswerGradeRequiresBothFieldsAndNoExtraText(t *testing.T) {
	for _, raw := range []string{
		`{"accuracy":1}`,
		`{"explanation":""}`,
		`{"accuracy":null,"explanation":""}`,
		`{"accuracy":1,"explanation":null}`,
		`{"accuracy":1,"explanation":""} trailing`,
	} {
		if _, err := parseAnswerGrade([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid grade: %s", raw)
		}
	}
	if grade, err := parseAnswerGrade([]byte(`{"accuracy":0.5,"explanation":" Missing fallback. "}`)); err != nil || grade.Accuracy != 0.5 || grade.Explanation != "Missing fallback." {
		t.Fatalf("valid grade rejected: %+v, %v", grade, err)
	}
}
