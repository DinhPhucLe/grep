package practice

import (
	"math"
	"testing"
	"time"

	"cortisol-server/internal/quiz"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestQuizAnswerEventPreservesIdentityAndGrade(t *testing.T) {
	for _, grade := range []float64{0, 0.5, 1} {
		answer := quiz.AnswerRecord{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), QuizID: bson.NewObjectID(), QuestionID: "q1", Graded: grade, CreatedAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
		org := bson.NewObjectID()
		event, err := quizAnswerEvent(answer, org)
		if err != nil {
			t.Fatal(err)
		}
		if event.ID != answer.ID || event.UserID != answer.UserID || event.OrganizationID != org || event.QuizID != answer.QuizID || event.QuestionID != "q1" || event.Grade == nil || *event.Grade != grade || event.StartedAt != answer.CreatedAt {
			t.Fatalf("lost answer provenance: %+v", event)
		}
		want := OutcomeFailedReveal
		if grade == 1 {
			want = OutcomeCorrect
		}
		if event.Outcome != want {
			t.Fatalf("grade %g: outcome %s", grade, event.Outcome)
		}
	}
}

func TestQuizAnswerEventRejectsInvalidSource(t *testing.T) {
	answer := quiz.AnswerRecord{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), QuizID: bson.NewObjectID(), QuestionID: "q1", Graded: 1, CreatedAt: time.Now()}
	for _, mutate := range []func(*quiz.AnswerRecord){
		func(a *quiz.AnswerRecord) { a.UserID = bson.NilObjectID },
		func(a *quiz.AnswerRecord) { a.CreatedAt = time.Time{} },
		func(a *quiz.AnswerRecord) { a.Graded = math.NaN() },
		func(a *quiz.AnswerRecord) { a.Graded = 1.1 },
	} {
		invalid := answer
		mutate(&invalid)
		if _, err := quizAnswerEvent(invalid, bson.NewObjectID()); err == nil {
			t.Fatal("accepted invalid answer")
		}
	}
}

func TestQuizAnswerDashboardDoesNotInventTimingOrCodebase(t *testing.T) {
	answer := quiz.AnswerRecord{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), QuizID: bson.NewObjectID(), QuestionID: "q1", Graded: 1, CreatedAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
	event, err := quizAnswerEvent(answer, bson.NewObjectID())
	if err != nil {
		t.Fatal(err)
	}
	view := BuildEmployeeView(EmployeeSubject{Year: 2026}, []Event{event})
	if len(view.ActivityCalendar.Days) != 1 || view.ActivityCalendar.Days[0].Count != 1 {
		t.Fatal("answer not counted")
	}
	for _, metric := range view.Metrics {
		if metric.ID == "median_active_answer_ms" && (metric.Status != "unknown" || metric.Display.Primary != "—") {
			t.Fatalf("invented duration: %+v", metric)
		}
	}
	org := BuildOrgView(OrgSubject{}, []Event{event})
	if len(org.CodebaseTreemaps) != 0 || len(org.Timeseries.Points) != 1 || org.Timeseries.Points[0].MedianActiveAnswerTimeMs != nil {
		t.Fatalf("invented source metadata: %+v", org)
	}
	legacy := validEvent()
	legacy.StartedAt = event.StartedAt
	mixed := BuildEmployeeView(EmployeeSubject{}, []Event{event, legacy})
	for _, metric := range mixed.Metrics {
		if metric.ID == "median_active_answer_ms" && metric.Display.Primary != "45.0s" {
			t.Fatalf("unknown duration diluted measured time: %+v", metric)
		}
	}
}
