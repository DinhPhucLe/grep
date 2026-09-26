package evaluation

import (
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Message struct {
	Role    string `json:"role" bson:"role"`
	Content string `json:"content" bson:"content"`
}
type Request struct {
	Input        string    `json:"input" bson:"input"`
	Context      string    `json:"context,omitempty" bson:"context,omitempty"`
	Conversation []Message `json:"conversation,omitempty" bson:"conversation,omitempty"`
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.Input) == "" {
		return errors.New("input must not be empty")
	}
	if len(r.Input) > 32000 || len(r.Context) > 128000 || len(r.Conversation) > 50 {
		return errors.New("input, context, or conversation exceeds the allowed size")
	}
	total := len(r.Input) + len(r.Context)
	for _, m := range r.Conversation {
		if (m.Role != "user" && m.Role != "assistant") || strings.TrimSpace(m.Content) == "" {
			return errors.New("conversation entries require a user/assistant role and nonempty content")
		}
		total += len(m.Content)
	}
	if total > 256000 {
		return errors.New("combined evaluation text exceeds 256000 bytes")
	}
	return nil
}

type Gap struct {
	Description string `json:"description" bson:"description"`
	Consequence string `json:"consequence" bson:"consequence"`
}

type Body struct {
	Verdict        string  `json:"verdict" bson:"verdict"`
	Summary        string  `json:"summary" bson:"summary"`
	AmbiguityScore float64 `json:"ambiguity_score" bson:"ambiguity_score"`
	Gaps           []Gap   `json:"gaps" bson:"gaps"`
}

func (b Body) Validate() error {
	if (b.Verdict != "clear" && b.Verdict != "ambiguous") || strings.TrimSpace(b.Summary) == "" || b.Gaps == nil || len(b.Gaps) > 10 || b.AmbiguityScore < 0 || b.AmbiguityScore > 1 {
		return errors.New("invalid evaluation fields")
	}
	if (b.Verdict == "clear" && len(b.Gaps) != 0) || (b.Verdict == "ambiguous" && len(b.Gaps) == 0) {
		return errors.New("verdict does not match gaps")
	}
	for _, gap := range b.Gaps {
		if strings.TrimSpace(gap.Description) == "" || strings.TrimSpace(gap.Consequence) == "" {
			return errors.New("invalid gap")
		}
	}
	return nil
}

type Record struct {
	ID            bson.ObjectID `json:"id" bson:"_id"`
	CreatedAt     time.Time     `json:"created_at" bson:"created_at"`
	Model         string        `json:"model" bson:"model"`
	RubricVersion string        `json:"rubric_version" bson:"rubric_version"`
	Request       Request       `json:"-" bson:"request"`
	Evaluation    Body          `json:"evaluation" bson:"evaluation"`
}
