package practice

import (
	"errors"
	"math"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	PracticeLeadAndReveal = "lead_and_reveal"
	OutcomeCorrect        = "correct"
	OutcomeFailedReveal   = "failed_reveal"
	SourceQuizAnswer      = "quiz_answers"
)

// Event is one practice instance. Quiz-answer events represent one saved question.
type Event struct {
	Source             string        `json:"source,omitempty" bson:"source,omitempty"`
	QuizID             bson.ObjectID `json:"quiz_id,omitempty" bson:"quiz_id,omitempty"`
	QuestionID         string        `json:"question_id,omitempty" bson:"question_id,omitempty"`
	Grade              *float64      `json:"grade,omitempty" bson:"grade,omitempty"`
	ID                 bson.ObjectID `json:"id" bson:"_id"`
	Practice           string        `json:"practice" bson:"practice"`
	OrganizationID     bson.ObjectID `json:"organization_id" bson:"organization_id"`
	UserID             bson.ObjectID `json:"user_id" bson:"user_id"`
	SessionID          bson.ObjectID `json:"session_id" bson:"session_id"`
	ProjectID          bson.ObjectID `json:"project_id" bson:"project_id"`
	StartedAt          time.Time     `json:"started_at" bson:"started_at"`
	EndedAt            time.Time     `json:"ended_at" bson:"ended_at"`
	TotalDurationMs    int64         `json:"total_duration_ms" bson:"total_duration_ms"`
	ActiveAnswerTimeMs int64         `json:"active_answer_time_ms" bson:"active_answer_time_ms"`
	Attempts           int           `json:"attempts" bson:"attempts"`
	Outcome            string        `json:"outcome" bson:"outcome"`
	PointsDelta        *int          `json:"points_delta" bson:"points_delta"`
	RepoName           string        `json:"repo_name" bson:"repo_name"`
	RepoOrg            string        `json:"repo_org" bson:"repo_org"`
	FilePath           string        `json:"file_path" bson:"file_path"`
	Module             string        `json:"module" bson:"module"`
	StartLine          int           `json:"start_line" bson:"start_line"`
	EndLine            int           `json:"end_line" bson:"end_line"`
	AnswerQuality      *string       `json:"answer_quality,omitempty" bson:"answer_quality,omitempty"`
	Relevance          *string       `json:"relevance,omitempty" bson:"relevance,omitempty"`
}

func (e Event) Validate() error {
	if e.Practice != PracticeLeadAndReveal {
		return errors.New("practice must be lead_and_reveal")
	}
	if e.Source == SourceQuizAnswer {
		if e.ID.IsZero() || e.UserID.IsZero() || e.QuizID.IsZero() || e.QuestionID == "" || e.StartedAt.IsZero() || !e.EndedAt.Equal(e.StartedAt) {
			return errors.New("quiz answer identity and submission timestamp are required")
		}
		if e.Grade == nil || math.IsNaN(*e.Grade) || math.IsInf(*e.Grade, 0) || *e.Grade < 0 || *e.Grade > 1 {
			return errors.New("quiz answer grade must be between 0 and 1")
		}
		if (*e.Grade == 1 && e.Outcome != OutcomeCorrect) || (*e.Grade < 1 && e.Outcome != OutcomeFailedReveal) {
			return errors.New("quiz answer outcome must match grade")
		}
		return nil
	}
	if e.OrganizationID.IsZero() || e.UserID.IsZero() || e.SessionID.IsZero() || e.ProjectID.IsZero() {
		return errors.New("organization_id, user_id, session_id, and project_id are required")
	}
	if e.EndedAt.Before(e.StartedAt) {
		return errors.New("ended_at must not be before started_at")
	}
	if e.TotalDurationMs < 0 || e.ActiveAnswerTimeMs < 0 {
		return errors.New("durations must be non-negative")
	}
	if e.Attempts < 1 || e.Attempts > 2 {
		return errors.New("attempts must be 1 or 2")
	}
	if e.Outcome != OutcomeCorrect && e.Outcome != OutcomeFailedReveal {
		return errors.New("outcome must be correct or failed_reveal")
	}
	if strings.TrimSpace(e.RepoName) == "" || strings.TrimSpace(e.FilePath) == "" || strings.TrimSpace(e.Module) == "" {
		return errors.New("repo_name, file_path, and module are required")
	}
	if e.StartLine < 1 || e.EndLine < e.StartLine {
		return errors.New("start_line and end_line must form a valid range")
	}
	if e.AnswerQuality != nil && strings.TrimSpace(*e.AnswerQuality) == "" {
		return errors.New("answer_quality must not be blank when set")
	}
	if e.Relevance != nil && strings.TrimSpace(*e.Relevance) == "" {
		return errors.New("relevance must not be blank when set")
	}
	return nil
}
