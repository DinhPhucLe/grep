package practice

import (
	"context"
	"fmt"

	"cortisol-server/internal/quiz"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// QuizAnswerProjector writes one event per immutable answer. Reusing the answer's
// _id makes retries and backfills idempotent through MongoDB's existing _id index.
type QuizAnswerProjector struct {
	database     *mongo.Database
	defaultOrgID bson.ObjectID
}

func NewQuizAnswerProjector(database *mongo.Database, defaultOrgID bson.ObjectID) *QuizAnswerProjector {
	return &QuizAnswerProjector{database: database, defaultOrgID: defaultOrgID}
}

func quizAnswerEvent(answer quiz.AnswerRecord, orgID bson.ObjectID) (Event, error) {
	grade := answer.Graded
	event := Event{ID: answer.ID, Source: SourceQuizAnswer, UserID: answer.UserID,
		OrganizationID: orgID, QuizID: answer.QuizID, QuestionID: answer.QuestionID,
		Grade: &grade, Practice: PracticeLeadAndReveal, StartedAt: answer.CreatedAt,
		EndedAt: answer.CreatedAt, Outcome: OutcomeFailedReveal}
	if grade == 1 {
		event.Outcome = OutcomeCorrect
	}
	return event, event.Validate()
}

// Historical answers have no organization field. Prefer the configured login
// organization if the user is a member; otherwise use their sole membership.
// Ambiguous or missing membership leaves organization unknown, never fabricated.
func (p *QuizAnswerProjector) organization(ctx context.Context, userID bson.ObjectID) (bson.ObjectID, error) {
	cursor, err := p.database.Collection("organization_members").Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return bson.NilObjectID, err
	}
	defer cursor.Close(ctx)
	var memberships []struct {
		OrganizationID bson.ObjectID `bson:"organization_id"`
	}
	if err := cursor.All(ctx, &memberships); err != nil {
		return bson.NilObjectID, err
	}
	unique := map[bson.ObjectID]bool{}
	for _, member := range memberships {
		if member.OrganizationID == p.defaultOrgID && !p.defaultOrgID.IsZero() {
			return p.defaultOrgID, nil
		}
		if !member.OrganizationID.IsZero() {
			unique[member.OrganizationID] = true
		}
	}
	if len(unique) == 1 {
		for id := range unique {
			return id, nil
		}
	}
	return bson.NilObjectID, nil
}

func (p *QuizAnswerProjector) SyncAnswer(ctx context.Context, answer quiz.AnswerRecord) error {
	if _, err := quizAnswerEvent(answer, bson.NilObjectID); err != nil {
		return err
	}
	orgID, err := p.organization(ctx, answer.UserID)
	if err != nil {
		return err
	}
	event, err := quizAnswerEvent(answer, orgID)
	if err != nil {
		return err
	}
	// Only known fields are persisted: no fake session, project, attempts,
	// duration, or source locations for historical answers.
	doc := bson.M{"source": event.Source, "practice": event.Practice, "user_id": event.UserID,
		"quiz_id": event.QuizID, "question_id": event.QuestionID, "grade": *event.Grade,
		"started_at": event.StartedAt, "ended_at": event.EndedAt, "outcome": event.Outcome}
	if !orgID.IsZero() {
		doc["organization_id"] = orgID
	}
	filter := bson.M{"_id": event.ID, "source": SourceQuizAnswer, "user_id": event.UserID,
		"quiz_id": event.QuizID, "question_id": event.QuestionID}
	_, err = p.database.Collection("practice_events").UpdateOne(ctx, filter,
		bson.M{"$setOnInsert": doc}, options.UpdateOne().SetUpsert(true))
	if mongo.IsDuplicateKeyError(err) {
		// A concurrent identical upsert can lose the insert race. A collision
		// with a different event must remain an error rather than overwriting it.
		if existingErr := p.database.Collection("practice_events").FindOne(ctx, filter).Err(); existingErr == nil {
			return nil
		}
	}
	return err
}

type QuizAnswerBackfillResult struct {
	Answers int `json:"answers"`
	Skipped int `json:"skipped"`
}

// Backfill includes graded answers only. Dry runs validate and count without writes.
func (p *QuizAnswerProjector) Backfill(ctx context.Context, userID bson.ObjectID, apply bool) (QuizAnswerBackfillResult, error) {
	var result QuizAnswerBackfillResult
	filter := bson.M{"graded": bson.M{"$type": "number"}}
	if !userID.IsZero() {
		filter["user_id"] = userID
	}
	cursor, err := p.database.Collection("quiz_answers").Find(ctx, filter)
	if err != nil {
		return result, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var answer quiz.AnswerRecord
		if err := cursor.Decode(&answer); err != nil {
			return result, err
		}
		if _, err := quizAnswerEvent(answer, bson.NilObjectID); err != nil {
			result.Skipped++
			continue
		}
		if apply {
			if err := p.SyncAnswer(ctx, answer); err != nil {
				return result, fmt.Errorf("project answer %s: %w", answer.ID.Hex(), err)
			}
		}
		result.Answers++
	}
	if err := cursor.Err(); err != nil {
		return result, err
	}
	return result, nil
}
