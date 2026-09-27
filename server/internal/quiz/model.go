// Package quiz generates questions about consequential choices in Codex's code.
package quiz

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"cortisol-server/internal/evaluation"
)

const AmbiguityThreshold = evaluation.AmbiguityThreshold
const MaxQuestions = 4

// File contains the generated version of a file. Line references are one-based
// and relative to Content, which must be the complete file, not a diff.
type File struct {
	Path    string `json:"path" bson:"path"`
	Content string `json:"content" bson:"content"`
}

type Request struct {
	ParticipantID string               `json:"participant_id,omitempty" bson:"participant_id,omitempty"`
	ProjectID     string               `json:"project_id,omitempty" bson:"project_id,omitempty"`
	ThreadID      string               `json:"thread_id,omitempty" bson:"thread_id,omitempty"`
	TurnID        string               `json:"turn_id,omitempty" bson:"turn_id,omitempty"`
	Input         string               `json:"input" bson:"input"`
	Context       string               `json:"context,omitempty" bson:"context,omitempty"`
	Conversation  []evaluation.Message `json:"conversation,omitempty" bson:"conversation,omitempty"`
	Evaluation    evaluation.Body      `json:"evaluation" bson:"evaluation"`
	Files         []File               `json:"files" bson:"files"`
	// Zero/omitted defaults to four. Callers may lower the cap to any value from one to four.
	MaxQuestions int `json:"max_questions,omitempty" bson:"max_questions,omitempty"`
}

func (r Request) limit() int {
	if r.MaxQuestions == 0 {
		return MaxQuestions
	}
	return r.MaxQuestions
}

func (r Request) Validate() error {
	if err := validateQuizMetadata(r); err != nil {
		return err
	}
	if err := (evaluation.Request{Input: r.Input, Context: r.Context, Conversation: r.Conversation}).Validate(); err != nil {
		return err
	}
	if r.Evaluation.Validate() != nil || r.Evaluation.Verdict != "ambiguous" || r.Evaluation.AmbiguityScore == nil || !(*r.Evaluation.AmbiguityScore > AmbiguityThreshold) {
		return errors.New("quiz requires an ambiguous evaluation with ambiguity_score > 0.3 and consequential gaps")
	}
	if r.limit() < 1 || r.limit() > MaxQuestions {
		return errors.New("max_questions must be between 1 and 4 (default 4)")
	}
	if len(r.Files) == 0 || len(r.Files) > 30 {
		return errors.New("provide 1 to 30 generated files")
	}
	total := len(r.Input) + len(r.Context) + len(r.Evaluation.Summary)
	for _, message := range r.Conversation {
		total += len(message.Content)
	}
	for _, gap := range r.Evaluation.Gaps {
		total += len(gap.Description) + len(gap.Consequence)
	}
	seen := map[string]bool{}
	for _, file := range r.Files {
		if strings.TrimSpace(file.Path) == "" || len(file.Path) > 1024 || strings.ContainsAny(file.Path, "\\\r\n\x00:") || path.IsAbs(file.Path) || path.Clean(file.Path) != file.Path || file.Path == ".." || strings.HasPrefix(file.Path, "../") || file.Path == "." || seen[file.Path] {
			return errors.New("file paths must be unique normalized repository-relative paths using forward slashes")
		}
		seen[file.Path] = true
		if strings.TrimSpace(file.Content) == "" || len(file.Content) > 128000 {
			return errors.New("each file must contain 1 to 128000 bytes of nonblank generated code")
		}
		total += len(file.Path) + len(file.Content)
	}
	if total > 256000 {
		return errors.New("combined quiz text exceeds 256000 bytes")
	}
	return nil
}

type Evidence struct {
	FilePath  string `json:"file_path" bson:"file_path"`
	StartLine int    `json:"start_line" bson:"start_line"`
	EndLine   int    `json:"end_line" bson:"end_line"`
}

type Question struct {
	ID         string     `json:"id" bson:"id"`
	Topic      string     `json:"topic" bson:"topic"`
	Question   string     `json:"question" bson:"question"`
	GapIndices []int      `json:"gap_indices" bson:"gap_indices"`
	Evidence   []Evidence `json:"evidence" bson:"evidence"`
}

type Result struct {
	Questions []Question `json:"questions" bson:"questions"`
	// Nonempty only when the supplied implementation supports no grounded quiz.
	NoQuestionsReason string `json:"no_questions_reason" bson:"no_questions_reason"`
}

// Validate checks grounding references and cardinality, not semantic correctness.
// Whether questions accurately explain code remains an LLM quality limitation.
func (r Result) Validate(request Request) error {
	return r.validate(request, true)
}

func (r Result) validate(request Request, requireIndependent bool) error {
	if r.Questions == nil || len(r.Questions) > request.limit() {
		return errors.New("invalid question count")
	}
	if len(r.Questions) == 0 {
		if strings.TrimSpace(r.NoQuestionsReason) == "" || len(r.NoQuestionsReason) > 2000 {
			return errors.New("missing no-questions reason")
		}
		return nil
	}
	if r.NoQuestionsReason != "" {
		return errors.New("unexpected no-questions reason")
	}
	lineCounts := map[string]int{}
	for _, file := range request.Files {
		lineCounts[file.Path] = strings.Count(strings.TrimSuffix(file.Content, "\n"), "\n") + 1
	}
	seenQuestions, seenTopics := map[string]bool{}, map[string]bool{}
	var priorEvidence []Evidence
	for n, question := range r.Questions {
		text, topic := strings.ToLower(strings.TrimSpace(question.Question)), strings.ToLower(strings.TrimSpace(question.Topic))
		if question.ID != fmt.Sprintf("q%d", n+1) || text == "" || topic == "" || len(question.Question) > 2000 || len(question.Topic) > 200 || seenQuestions[text] || seenTopics[topic] {
			return errors.New("invalid or duplicate question")
		}
		seenQuestions[text], seenTopics[topic] = true, true
		if len(question.GapIndices) == 0 || len(question.GapIndices) > len(request.Evaluation.Gaps) || len(question.Evidence) == 0 || len(question.Evidence) > 6 {
			return errors.New("missing or excessive grounding references")
		}
		seenGaps := map[int]bool{}
		for _, gap := range question.GapIndices {
			if gap < 0 || gap >= len(request.Evaluation.Gaps) || seenGaps[gap] {
				return errors.New("invalid gap reference")
			}
			seenGaps[gap] = true
		}
		for refIndex, ref := range question.Evidence {
			lineCount, exists := lineCounts[ref.FilePath]
			if !exists {
				return fmt.Errorf("invalid code reference: question %d evidence %d names an unknown file", n+1, refIndex+1)
			}
			if ref.StartLine < 1 || ref.EndLine < ref.StartLine || ref.EndLine > lineCount {
				return fmt.Errorf("invalid code reference: question %d evidence %d has range %d..%d; expected 1 <= start <= end within 1..%d", n+1, refIndex+1, ref.StartLine, ref.EndLine, lineCount)
			}
			for _, prior := range priorEvidence {
				if requireIndependent && evidenceOverlaps(ref, prior) {
					return errors.New("questions must use independently revealable, non-overlapping code ranges")
				}
			}
		}
		priorEvidence = append(priorEvidence, question.Evidence...)
	}
	return nil
}

func evidenceOverlaps(a, b Evidence) bool {
	return a.FilePath == b.FilePath && a.StartLine <= b.EndLine && b.StartLine <= a.EndLine
}

// Keep the model's order and wording, dropping later questions whose answers
// would already be exposed by an earlier reveal. Call only after validating
// every generated question's structure and grounding, including dropped ones.
func (r Result) independentQuestions() Result {
	kept := make([]Question, 0, len(r.Questions))
	var revealed []Evidence
questions:
	for _, q := range r.Questions {
		for _, ref := range q.Evidence {
			for _, prior := range revealed {
				if evidenceOverlaps(ref, prior) {
					continue questions
				}
			}
		}
		q.ID = fmt.Sprintf("q%d", len(kept)+1)
		kept = append(kept, q)
		revealed = append(revealed, q.Evidence...)
	}
	r.Questions = kept
	return r
}
