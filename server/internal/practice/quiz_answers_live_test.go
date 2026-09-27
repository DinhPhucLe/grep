package practice

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"cortisol-server/internal/auth"
	"cortisol-server/internal/db"
	"cortisol-server/internal/quiz"
	"cortisol-server/internal/user"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Opt-in verification against configured MongoDB. Only materializes events from
// the selected user's existing answers; no fake answers or users are inserted.
func TestLiveQuizAnswerProjection(t *testing.T) {
	userHex := os.Getenv("CORTISOL_VERIFY_PRACTICE_USER")
	if userHex == "" {
		t.Skip("set CORTISOL_VERIFY_PRACTICE_USER to project and verify existing answers")
	}
	id, err := bson.ObjectIDFromHex(userHex)
	if err != nil {
		t.Fatal(err)
	}
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatal(err)
	}
	config, err := db.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	authConfig, err := auth.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	database, err := db.Connect(ctx, config)
	if err != nil {
		t.Fatal("could not connect to configured database")
	}
	defer db.Disconnect(database)
	projector := NewQuizAnswerProjector(database, authConfig.DefaultOrgID)
	first, err := projector.Backfill(ctx, id, true)
	if err != nil || first.Answers == 0 {
		t.Fatalf("backfill: %+v %v", first, err)
	}
	filter := bson.M{"source": SourceQuizAnswer, "user_id": id}
	before, err := database.Collection("practice_events").CountDocuments(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if before != int64(first.Answers) {
		t.Fatalf("%d answers produced %d events", first.Answers, before)
	}
	var answer quiz.AnswerRecord
	if err := database.Collection("quiz_answers").FindOne(ctx, bson.M{"user_id": id, "graded": bson.M{"$type": "number"}}).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	store := quiz.NewAnswerStore(quiz.NewMongoAnswerRepository(database), user.NewMongoRepository(database), nil, projector)
	request := quiz.AnswerRequest{UserID: userHex, QuizID: answer.QuizID.Hex(), QuestionID: answer.QuestionID, Answer: answer.Answer, Graded: &answer.Graded, Reasoning: answer.Reasoning}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, err := store.Submit(ctx, request)
			if err != nil {
				t.Error(err)
				return
			}
			if receipt.ID != answer.ID.Hex() || receipt.UserID != userHex {
				t.Error("retry changed answer identity")
			}
		}()
	}
	wg.Wait()
	if _, err := projector.Backfill(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	after, err := database.Collection("practice_events").CountDocuments(ctx, filter)
	if err != nil || after != before {
		t.Fatalf("retries changed count: before=%d after=%d err=%v", before, after, err)
	}
	var stored bson.M
	if err := database.Collection("practice_events").FindOne(ctx, bson.M{"_id": answer.ID}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if stored["user_id"] != id || stored["grade"] != answer.Graded {
		t.Fatalf("event identity/grade mismatch")
	}
	for _, field := range []string{"active_answer_time_ms", "project_id", "session_id", "file_path"} {
		if _, exists := stored[field]; exists {
			t.Fatalf("fabricated %s", field)
		}
	}
	svc := NewService(NewMongoMemberChecker(database), NewMongoEventRepository(database), NewMongoDirectory(database))
	view, err := svc.EmployeeView(ctx, userHex, PracticeLeadAndReveal, answer.CreatedAt.Year())
	if err != nil {
		t.Fatal(err)
	}
	if view.ActivityCalendar.Status != "available" || view.OutcomePie.Status != "available" {
		t.Fatal("dashboard did not include saved answers")
	}
	t.Logf("verified user=%s events=%d; 8 concurrent save retries and repeated backfill created no duplicates; dashboard=%s", userHex, after, view.OutcomePie.Display.Primary)
}
