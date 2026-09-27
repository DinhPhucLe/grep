package quiz

import (
	"context"
	"encoding/json"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const answerUser = "66f600000000000000000001"

type memoryAnswers struct {
	mu      sync.Mutex
	records map[string]AnswerRecord
	err     error
}

func (m *memoryAnswers) Find(ctx context.Context, p, q, id string) (AnswerRecord, error) {
	if err := ctx.Err(); err != nil {
		return AnswerRecord{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return AnswerRecord{}, m.err
	}
	r, ok := m.records[p+q+id]
	if !ok {
		return AnswerRecord{}, ErrAnswerNotFound
	}
	return r, nil
}
func (m *memoryAnswers) Insert(ctx context.Context, r AnswerRecord) (AnswerRecord, error) {
	if err := ctx.Err(); err != nil {
		return AnswerRecord{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return AnswerRecord{}, m.err
	}
	key := r.UserID.Hex() + r.QuizID.Hex() + r.QuestionID
	if _, ok := m.records[key]; ok {
		return AnswerRecord{}, ErrAnswerConflict
	}
	m.records[key] = r
	return r, nil
}

type answerUsers struct {
	wrongProject bool
	exists       bool
	err          error
}

func (p answerUsers) OwnsProject(context.Context, string, string) (bool, error) {
	return p.exists && !p.wrongProject, p.err
}
func (p answerUsers) Exists(context.Context, string) (bool, error) { return p.exists, p.err }
func identifiedRequest() Request {
	r := exampleRequest()
	r.UserID = answerUser
	r.ProjectID = "66f600000000000000000003"
	r.ThreadID = "thread-1"
	r.TurnID = "turn-1"
	return r
}
func answerFixture(t *testing.T) (*AnswerStore, *memoryAnswers, *fakeCompleter, Response) {
	t.Helper()
	raw, _ := json.Marshal(exampleResult())
	client := &fakeCompleter{raw: string(raw)}
	repo := &memoryAnswers{records: map[string]AnswerRecord{}}
	s := NewAnswerStore(repo, answerUsers{exists: true}, NewService(client, "test-model"))
	r, err := s.Generate(context.Background(), identifiedRequest())
	if err != nil {
		t.Fatal(err)
	}
	return s, repo, client, r
}
func TestAnswerStoresAuthoritativeSnapshotWithoutGrading(t *testing.T) {
	s, repo, client, res := answerFixture(t)
	if res.QuizID == "" {
		t.Fatal("missing quiz ID")
	}
	res.Questions[0].Question = "client mutation"
	res.Questions[0].Evidence[0].StartLine = 999
	receipt, err := s.Submit(context.Background(), AnswerRequest{answerUser, res.QuizID, "q1", "generic errors"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := repo.Find(context.Background(), answerUser, res.QuizID, "q1")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "ungraded" || r.Status != "ungraded" || r.Question.Question != exampleResult().Questions[0].Question || r.Question.Evidence[0].StartLine != 3 || r.Request.Files[0].Content != exampleRequest().Files[0].Content || r.Model != "test-model" || r.PromptVersion != PromptVersion || r.ProjectID.Hex() != "66f600000000000000000003" || r.ThreadID != "thread-1" || r.TurnID != "turn-1" || r.CreatedAt.IsZero() {
		t.Fatalf("incorrect snapshot: %+v", r)
	}
	if client.calls != 1 {
		t.Fatal("answer called generator/grader")
	}
	for _, field := range []string{"user_id", "project_id", "thread_id", "turn_id"} {
		if strings.Contains(client.input, field) {
			t.Fatalf("metadata sent to provider: %s", field)
		}
	}
	raw, err := bson.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	doc := bson.Raw(raw)
	for _, field := range []string{"user_id", "project_id", "quiz_id"} {
		if doc.Lookup(field).Type != bson.TypeObjectID {
			t.Fatalf("%s must be a BSON ObjectID", field)
		}
	}
	requestDoc := doc.Lookup("request").Document()
	for _, field := range []string{"user_id", "project_id", "thread_id", "turn_id"} {
		if requestDoc.Lookup(field).Type != 0 {
			t.Fatalf("request snapshot contains root metadata %s", field)
		}
	}
	question := doc.Lookup("question").Document()
	if question.Lookup("gap_indices").Type == 0 || question.Lookup("evidence").Array().Index(0).Document().Lookup("start_line").Type == 0 {
		t.Fatal("nested BSON lost snake_case")
	}
	if doc.Lookup("grade").Type != 0 {
		t.Fatal("unexpected grade")
	}
}
func TestAnswerIdempotencySurvivesCacheLoss(t *testing.T) {
	s, repo, _, res := answerFixture(t)
	in := AnswerRequest{answerUser, res.QuizID, "q1", "original"}
	first, err := s.Submit(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewAnswerStore(repo, answerUsers{exists: true}, nil)
	again, err := restarted.Submit(context.Background(), in)
	if err != nil || again.ID != first.ID || !again.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("retry changed identity: %+v %v", again, err)
	}
	in.Answer = "changed"
	if _, err := restarted.Submit(context.Background(), in); !errors.Is(err, ErrAnswerConflict) {
		t.Fatalf("want conflict: %v", err)
	}
}
func TestAnswerRejectsUnknownOrExpiredContext(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*AnswerStore, *AnswerRequest)
		want   error
	}{
		{"user", func(s *AnswerStore, r *AnswerRequest) { s.users = answerUsers{} }, ErrUserNotFound},
		{"quiz", func(s *AnswerStore, r *AnswerRequest) { r.QuizID = bson.NewObjectID().Hex() }, ErrQuizNotFound},
		{"question", func(s *AnswerStore, r *AnswerRequest) { r.QuestionID = "q4" }, ErrQuestionNotFound},
		{"ownership", func(s *AnswerStore, r *AnswerRequest) { r.UserID = "66f600000000000000000002" }, ErrQuizNotFound},
		{"expiration", func(s *AnswerStore, r *AnswerRequest) {
			s.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
		}, ErrQuizNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, repo, _, res := answerFixture(t)
			r := AnswerRequest{answerUser, res.QuizID, "q1", "answer"}
			tc.change(s, &r)
			if _, err := s.Submit(context.Background(), r); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if len(repo.records) != 0 {
				t.Fatal("invalid answer persisted")
			}
		})
	}
}
func TestAnswerHTTPValidationAndStatuses(t *testing.T) {
	s, _, _, res := answerFixture(t)
	for _, tc := range []struct {
		answer, quiz, extra string
		status              int
	}{
		{"", res.QuizID, "", 400}, {"  ", res.QuizID, "", 400}, {strings.Repeat("a", 8001), res.QuizID, "", 400}, {"a", bson.NewObjectID().Hex(), "", 404}, {"a", res.QuizID, `,"question":{}`, 400}, {"a", res.QuizID, "", 200}, {"b", res.QuizID, "", 409},
	} {
		body := `{"user_id":"` + answerUser + `","quiz_id":"` + tc.quiz + `","question_id":"q1","answer":"` + tc.answer + `"` + tc.extra + `}`
		r := httptest.NewRequest("POST", "/quizzes/answers", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewAnswerHandler(s, time.Second)(w, r)
		if w.Code != tc.status {
			t.Fatalf("got %d %s want %d", w.Code, w.Body, tc.status)
		}
	}
}
func TestAnswerCancellationAndStorageFailure(t *testing.T) {
	s, repo, _, res := answerFixture(t)
	in := AnswerRequest{answerUser, res.QuizID, "q1", "answer"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Submit(ctx, in); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
	repo.err = errors.New("private db info")
	raw, _ := json.Marshal(in)
	r := httptest.NewRequest("POST", "/quizzes/answers", strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	NewAnswerHandler(s, time.Second)(w, r)
	if w.Code != 500 || strings.Contains(w.Body.String(), "private db info") {
		t.Fatalf("unsafe failure: %d %s", w.Code, w.Body)
	}
}
func TestIdentifiedGenerationRequiresMetadataAndUser(t *testing.T) {
	raw, _ := json.Marshal(exampleResult())
	client := &fakeCompleter{raw: string(raw)}
	s := NewAnswerStore(&memoryAnswers{}, answerUsers{}, NewService(client, "test"))
	r := identifiedRequest()
	if _, err := s.Generate(context.Background(), r); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
	r.ProjectID = ""
	if _, err := s.Generate(context.Background(), r); !errors.Is(err, ErrInvalidQuizMetadata) {
		t.Fatalf("missing metadata: %v", err)
	}
	if client.calls != 0 {
		t.Fatal("invalid metadata reached generator")
	}
	res, err := s.Generate(context.Background(), exampleRequest())
	if err != nil || res.QuizID != "" {
		t.Fatalf("legacy generation: %+v %v", res, err)
	}
}
func TestQuizCapacityPreservesActiveSnapshots(t *testing.T) {
	s, _, _, res := answerFixture(t)
	s.capacity = 1
	if _, err := s.Generate(context.Background(), identifiedRequest()); !errors.Is(err, ErrQuizCapacity) {
		t.Fatalf("capacity ignored: %v", err)
	}
	if _, err := s.Submit(context.Background(), AnswerRequest{answerUser, res.QuizID, "q1", "answer"}); err != nil {
		t.Fatalf("active quiz evicted: %v", err)
	}
}

func TestAnswerSnapshotDoesNotAliasOriginalRequest(t *testing.T) {
	raw, _ := json.Marshal(exampleResult())
	client := &fakeCompleter{raw: string(raw)}
	repo := &memoryAnswers{records: map[string]AnswerRecord{}}
	store := NewAnswerStore(repo, answerUsers{exists: true}, NewService(client, "model"))
	request := identifiedRequest()
	response, err := store.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.Files[0].Content = "mutated"
	request.Evaluation.Gaps[0].Description = "mutated"
	*request.Evaluation.AmbiguityScore = 0
	_, err = store.Submit(context.Background(), AnswerRequest{answerUser, response.QuizID, "q1", "answer"})
	if err != nil {
		t.Fatal(err)
	}
	record, _ := repo.Find(context.Background(), answerUser, response.QuizID, "q1")
	if record.Request.Files[0].Content == "mutated" || record.Request.Evaluation.Gaps[0].Description == "mutated" || *record.Request.Evaluation.AmbiguityScore != 0.7 {
		t.Fatal("request mutation changed authoritative snapshot")
	}
}
func TestAnswerConcurrentRetriesKeepOneImmutableRecord(t *testing.T) {
	store, repo, _, response := answerFixture(t)
	input := AnswerRequest{answerUser, response.QuizID, "q1", "answer"}
	results := make(chan AnswerReceipt, 16)
	errs := make(chan error, 16)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() { r, err := store.Submit(context.Background(), input); results <- r; errs <- err })
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	for r := range results {
		if id == "" {
			id = r.ID
		}
		if r.ID != id {
			t.Fatal("concurrent retry changed immutable record")
		}
	}
	if len(repo.records) != 1 {
		t.Fatalf("saved %d records", len(repo.records))
	}
}
func TestGenerationFailureAndExpirationReleaseCapacity(t *testing.T) {
	raw, _ := json.Marshal(exampleResult())
	client := &fakeCompleter{err: errors.New("generation failed")}
	store := NewAnswerStore(&memoryAnswers{}, answerUsers{exists: true}, NewService(client, "model"))
	store.capacity = 1
	if _, err := store.Generate(context.Background(), identifiedRequest()); err == nil {
		t.Fatal("generation unexpectedly succeeded")
	}
	client.err = nil
	client.raw = string(raw)
	first, err := store.Generate(context.Background(), identifiedRequest())
	if err != nil {
		t.Fatalf("failed request consumed capacity: %v", err)
	}
	store.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	next, err := store.Generate(context.Background(), identifiedRequest())
	if err != nil || next.QuizID == first.QuizID {
		t.Fatalf("expired quiz consumed capacity: %v", err)
	}
}
func TestAnswerMigrationAndUserFailuresAreSafe(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{ErrMigrationRequired, 503, "migration_required"}, {errors.New("private user DB info"), 500, "quiz_failed"}} {
		store, _, _, response := answerFixture(t)
		store.users = answerUsers{err: tc.err}
		input := AnswerRequest{answerUser, response.QuizID, "q1", "answer"}
		raw, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/quizzes/answers", strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewAnswerHandler(store, time.Second)(w, req)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "private") {
			t.Fatalf("unsafe error: %d %s", w.Code, w.Body)
		}
	}
}
func TestMongoAnswerRepositoryRequiresExistingCollection(t *testing.T) {
	empty := bson.D{{Key: "ok", Value: 1}, {Key: "cursor", Value: bson.D{{Key: "id", Value: int64(0)}, {Key: "ns", Value: "test.$cmd.listCollections"}, {Key: "firstBatch", Value: bson.A{}}}}}
	var commands []string
	opts := options.Client().SetMonitor(&event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) { commands = append(commands, e.CommandName) }})
	opts.Deployment = drivertest.NewMockDeployment(empty)
	client, err := mongo.Connect(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	repo := NewMongoAnswerRepository(client.Database("test"))
	if _, err := repo.Insert(context.Background(), AnswerRecord{}); !errors.Is(err, ErrMigrationRequired) {
		t.Fatalf("missing migration accepted: %v", err)
	}
	if len(commands) != 1 || commands[0] != "listCollections" {
		t.Fatalf("unexpected DB operations: %v", commands)
	}
}
func TestMongoAnswerRepositoryMapsDuplicateAndNotFound(t *testing.T) {
	collection := bson.D{{Key: "ok", Value: 1}, {Key: "cursor", Value: bson.D{{Key: "id", Value: int64(0)}, {Key: "ns", Value: "test.$cmd.listCollections"}, {Key: "firstBatch", Value: bson.A{bson.D{{Key: "name", Value: "quiz_answers"}, {Key: "type", Value: "collection"}}}}}}}
	duplicate := bson.D{{Key: "ok", Value: 1}, {Key: "writeErrors", Value: bson.A{bson.D{{Key: "index", Value: 0}, {Key: "code", Value: 11000}, {Key: "errmsg", Value: "duplicate"}}}}}
	empty := bson.D{{Key: "ok", Value: 1}, {Key: "cursor", Value: bson.D{{Key: "id", Value: int64(0)}, {Key: "ns", Value: "test.quiz_answers"}, {Key: "firstBatch", Value: bson.A{}}}}}
	opts := options.Client()
	opts.Deployment = drivertest.NewMockDeployment(collection, readyAnswerIndex(), duplicate, empty)
	client, err := mongo.Connect(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	repo := NewMongoAnswerRepository(client.Database("test"))
	if _, err := repo.Insert(context.Background(), AnswerRecord{ID: bson.NewObjectID()}); !errors.Is(err, ErrAnswerConflict) {
		t.Fatalf("duplicate error not mapped: %v", err)
	}
	if _, err := repo.Find(context.Background(), answerUser, "66f600000000000000000004", "q1"); !errors.Is(err, ErrAnswerNotFound) {
		t.Fatalf("missing answer not mapped: %v", err)
	}
}

func TestMongoAnswerRepositoryRequiresUniqueAnswerIndex(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index bson.D
	}{
		{"missing", nil},
		{"not unique", bson.D{{Key: "name", Value: "one_answer_per_user_question"}, {Key: "key", Value: bson.D{{Key: "user_id", Value: 1}, {Key: "quiz_id", Value: 1}, {Key: "question_id", Value: 1}}}}},
		{"wrong keys", bson.D{{Key: "name", Value: "one_answer_per_user_question"}, {Key: "unique", Value: true}, {Key: "key", Value: bson.D{{Key: "user_id", Value: 1}, {Key: "quiz_id", Value: 1}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collection := answerCursor("test.$cmd.listCollections", bson.D{{Key: "name", Value: "quiz_answers"}, {Key: "type", Value: "collection"}})
			indices := answerCursor("test.quiz_answers")
			if tc.index != nil {
				indices = answerCursor("test.quiz_answers", tc.index)
			}
			var commands []string
			opts := options.Client().SetMonitor(&event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) { commands = append(commands, e.CommandName) }})
			opts.Deployment = drivertest.NewMockDeployment(collection, indices)
			client, err := mongo.Connect(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Disconnect(context.Background())
			_, err = NewMongoAnswerRepository(client.Database("test")).Insert(context.Background(), AnswerRecord{})
			if !errors.Is(err, ErrMigrationRequired) {
				t.Fatalf("unsafe schema accepted: %v", err)
			}
			if len(commands) != 2 || commands[0] != "listCollections" || commands[1] != "listIndexes" {
				t.Fatalf("unexpected DB operations: %v", commands)
			}
		})
	}
}
func answerCursor(namespace string, documents ...bson.D) bson.D {
	batch := bson.A{}
	for _, doc := range documents {
		batch = append(batch, doc)
	}
	return bson.D{{Key: "ok", Value: 1}, {Key: "cursor", Value: bson.D{{Key: "id", Value: int64(0)}, {Key: "ns", Value: namespace}, {Key: "firstBatch", Value: batch}}}}
}
func readyAnswerIndex() bson.D {
	return answerCursor("test.quiz_answers", bson.D{{Key: "name", Value: "one_answer_per_user_question"}, {Key: "unique", Value: true}, {Key: "key", Value: bson.D{{Key: "user_id", Value: 1}, {Key: "quiz_id", Value: 1}, {Key: "question_id", Value: 1}}}})
}

func TestProjectOwnershipRequiredForGenerationAndSaving(t *testing.T) {
	store, repo, client, response := answerFixture(t)
	store.users = answerUsers{exists: true, wrongProject: true}
	if _, err := store.Generate(context.Background(), identifiedRequest()); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("generation accepted wrong project: %v", err)
	}
	if client.calls != 1 {
		t.Fatal("wrong project reached Snowflake")
	}
	if _, err := store.Submit(context.Background(), AnswerRequest{answerUser, response.QuizID, "q1", "answer"}); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("save accepted wrong project: %v", err)
	}
	if len(repo.records) != 0 {
		t.Fatal("saved answer for another user's project")
	}
}

func TestRetryChecksCurrentProjectOwnership(t *testing.T) {
	store, _, _, response := answerFixture(t)
	input := AnswerRequest{answerUser, response.QuizID, "q1", "answer"}
	if _, err := store.Submit(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	store.users = answerUsers{exists: true, wrongProject: true}
	if _, err := store.Submit(context.Background(), input); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("retry ignored changed ownership: %v", err)
	}
}
