// Package quiz recognizes, validates and summarizes quiz envelopes — the
// `type: "quiz"` shape that rides an assignment's opaque `content_json` field
// alongside ordinary Tiptap documents (`type: "doc"`).
//
// The Andamio app is the authority for the shared rules. Recognize mirrors
// its isQuizContentEnvelope guard and Validate mirrors validateQuizDefinition:
// same control flow, same issue codes, same wording where practical. The
// exact upstream revision these mirror, and the three checks the CLI adds on
// top (malformed-prompt, malformed-help, malformed-intro), are recorded in
// testdata/quiz/SOURCE.md; a rule change in the app is re-mirrored by hand
// and pinned by the fixtures under testdata/quiz.
//
// Recognition and validity are deliberately separate, as in the app: an
// envelope with an unsupported version is still quiz-shaped (it must not be
// treated as a Tiptap doc), it is just not a valid quiz.
//
// The package has no dependency on cmd or on any other internal package.
package quiz

import (
	"encoding/json"
	"fmt"
	"math"
)

// Kind is the recognized shape of a content_json value.
type Kind int

const (
	// NotObject is any JSON value that is not an object: array, string,
	// number, boolean or null.
	NotObject Kind = iota
	// Doc is a Tiptap document (`type: "doc"`).
	Doc
	// Quiz is a quiz-shaped envelope: `type: "quiz"`, numeric `version`,
	// array `questions`. Validity is a separate question — see Validate.
	Quiz
	// Other is an object that is neither a doc nor quiz-shaped (including a
	// quiz-evidence envelope and a `type: "quiz"` object missing `questions`).
	Other
)

func (k Kind) String() string {
	switch k {
	case NotObject:
		return "not an object"
	case Doc:
		return "doc"
	case Quiz:
		return "quiz"
	case Other:
		return "other"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// SupportedVersion is the only quiz envelope version the CLI accepts.
const SupportedVersion = 1

// Issue codes. The first ten reuse the app's QuizDefinitionIssueCode names
// exactly; the last three are CLI-additional checks; CodeNotAQuiz is a guard
// for callers that hand Validate something Recognize would not call a quiz.
const (
	CodeUnsupportedVersion       = "unsupported-version"
	CodeEmptyQuestions           = "empty-questions"
	CodeInvalidThreshold         = "invalid-threshold"
	CodeThresholdExceedsQuestion = "threshold-exceeds-questions"
	CodeMissingCorrectValue      = "missing-correct-value"
	CodeDanglingCorrectValue     = "dangling-correct-value"
	CodeDuplicateOptionValues    = "duplicate-option-values"
	CodeDuplicateQuestionIDs     = "duplicate-question-ids"
	CodeTooFewOptions            = "too-few-options"
	CodeMalformedQuestion        = "malformed-question"

	CodeMalformedPrompt = "malformed-prompt"
	CodeMalformedHelp   = "malformed-help"
	CodeMalformedIntro  = "malformed-intro"

	CodeNotAQuiz = "not-a-quiz"
)

// AllCodes lists every issue code Validate can emit.
var AllCodes = []string{
	CodeUnsupportedVersion,
	CodeEmptyQuestions,
	CodeInvalidThreshold,
	CodeThresholdExceedsQuestion,
	CodeMissingCorrectValue,
	CodeDanglingCorrectValue,
	CodeDuplicateOptionValues,
	CodeDuplicateQuestionIDs,
	CodeTooFewOptions,
	CodeMalformedQuestion,
	CodeMalformedPrompt,
	CodeMalformedHelp,
	CodeMalformedIntro,
	CodeNotAQuiz,
}

// Issue is one violated rule. QuestionID is set when the rule applies to a
// specific question.
type Issue struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	QuestionID string `json:"question_id,omitempty"`
}

// String renders the issue as one line suitable for stderr.
func (i Issue) String() string {
	if i.QuestionID != "" {
		return fmt.Sprintf("%s [question %q]: %s", i.Code, i.QuestionID, i.Message)
	}
	return fmt.Sprintf("%s: %s", i.Code, i.Message)
}

// Summary is the scriptable digest of a quiz envelope.
type Summary struct {
	QuestionCount int      `json:"question_count"`
	PassThreshold int      `json:"pass_threshold"`
	QuestionIDs   []string `json:"question_ids"`
}

// Recognize decodes raw JSON and classifies its shape. The error is non-nil
// only when raw is not valid JSON. env is the decoded object for Doc, Quiz and
// Other, and nil for NotObject.
func Recognize(raw []byte) (Kind, map[string]interface{}, error) {
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return NotObject, nil, fmt.Errorf("invalid JSON: %w", err)
	}
	env, ok := value.(map[string]interface{})
	if !ok {
		return NotObject, nil, nil
	}
	return recognizeObject(env), env, nil
}

// recognizeObject mirrors isQuizContentEnvelope, with a doc branch first.
func recognizeObject(env map[string]interface{}) Kind {
	switch env["type"] {
	case "doc":
		return Doc
	case "quiz":
		if isQuizShaped(env) {
			return Quiz
		}
	}
	return Other
}

func isQuizShaped(env map[string]interface{}) bool {
	if env["type"] != "quiz" {
		return false
	}
	if _, ok := env["version"].(float64); !ok {
		return false
	}
	_, ok := env["questions"].([]interface{})
	return ok
}

// Parse is Recognize followed by Validate when the value is quiz-shaped. It is
// the one-call path for commands: kind tells the caller how to route, issues
// is empty for a valid quiz (and always empty for non-quiz kinds).
func Parse(raw []byte) (env map[string]interface{}, kind Kind, issues []Issue, err error) {
	kind, env, err = Recognize(raw)
	if err != nil {
		return nil, kind, nil, err
	}
	if kind == Quiz {
		issues = Validate(env)
	}
	return env, kind, issues, nil
}

// Validate mirrors the app's validateQuizDefinition and adds the three
// CLI-additional checks. It collects every issue it can find — it never stops
// at the first and never panics on malformed elements. env is expected to
// have been recognized as Quiz; anything else yields a single not-a-quiz
// issue rather than a false pass.
func Validate(env map[string]interface{}) []Issue {
	issues := []Issue{}

	if env == nil || !isQuizShaped(env) {
		return append(issues, Issue{
			Code:    CodeNotAQuiz,
			Message: `Not a quiz envelope — expected an object with type "quiz", a numeric version and a questions array.`,
		})
	}

	version, ok := integral(env["version"])
	if !ok || version != SupportedVersion {
		issues = append(issues, Issue{
			Code:    CodeUnsupportedVersion,
			Message: fmt.Sprintf("Envelope version %s is not supported (expected %d).", formatNumber(env["version"]), SupportedVersion),
		})
	}

	// CLI-additional: intro, when present and non-null, must be a Tiptap doc.
	// Checked before the empty-questions early return so it is always reported.
	if intro, present := env["intro"]; present && intro != nil {
		introObj, isObj := intro.(map[string]interface{})
		if !isObj || introObj["type"] != "doc" {
			issues = append(issues, Issue{
				Code:    CodeMalformedIntro,
				Message: `intro must be a Tiptap document (an object with type "doc"), or null/absent.`,
			})
		}
	}

	rawQuestions := env["questions"].([]interface{})
	if len(rawQuestions) == 0 {
		return append(issues, Issue{
			Code:    CodeEmptyQuestions,
			Message: "The quiz has no questions.",
		})
	}

	threshold, ok := integral(env["passThreshold"])
	if !ok || threshold < 1 {
		issues = append(issues, Issue{
			Code:    CodeInvalidThreshold,
			Message: "passThreshold must be a positive whole number.",
		})
	} else if threshold > len(rawQuestions) {
		issues = append(issues, Issue{
			Code:    CodeThresholdExceedsQuestion,
			Message: fmt.Sprintf("passThreshold (%d) exceeds the question count (%d) — the quiz can never be passed.", threshold, len(rawQuestions)),
		})
	}

	seenIDs := map[string]bool{}
	for _, rawQuestion := range rawQuestions {
		question, isObj := rawQuestion.(map[string]interface{})
		if !isObj {
			issues = append(issues, Issue{
				Code:    CodeMalformedQuestion,
				Message: "A questions entry is not an object.",
			})
			continue
		}

		questionID, hasID := question["id"].(string)
		if !hasID {
			issues = append(issues, Issue{
				Code:    CodeMalformedQuestion,
				Message: "A question is missing a string id.",
			})
			continue
		}

		// CLI-additional: prompt must be a non-empty string; help, when
		// present and non-null, must be a string. Neither depends on the
		// options shape, so they are reported even when options are broken.
		if prompt, isStr := question["prompt"].(string); !isStr || prompt == "" {
			issues = append(issues, Issue{
				Code:       CodeMalformedPrompt,
				Message:    fmt.Sprintf("Question %q must have a non-empty string prompt.", questionID),
				QuestionID: questionID,
			})
		}
		if help, present := question["help"]; present && help != nil {
			if _, isStr := help.(string); !isStr {
				issues = append(issues, Issue{
					Code:       CodeMalformedHelp,
					Message:    fmt.Sprintf("Question %q has a help field that is not a string.", questionID),
					QuestionID: questionID,
				})
			}
		}

		options, optionsOK := optionRecords(question["options"])
		if !optionsOK {
			issues = append(issues, Issue{
				Code:       CodeMalformedQuestion,
				Message:    fmt.Sprintf("Question %q has malformed options — expected an array of { value, label } string pairs.", questionID),
				QuestionID: questionID,
			})
			continue
		}

		if seenIDs[questionID] {
			issues = append(issues, Issue{
				Code:       CodeDuplicateQuestionIDs,
				Message:    fmt.Sprintf("Question id %q is used more than once — answers persist keyed by id.", questionID),
				QuestionID: questionID,
			})
		}
		seenIDs[questionID] = true

		optionValues := make([]string, 0, len(options))
		distinct := map[string]bool{}
		for _, opt := range options {
			optionValues = append(optionValues, opt.value)
			distinct[opt.value] = true
		}
		if len(optionValues) < 2 {
			issues = append(issues, Issue{
				Code:       CodeTooFewOptions,
				Message:    fmt.Sprintf("Question %q has fewer than two options.", questionID),
				QuestionID: questionID,
			})
		}
		if len(distinct) != len(optionValues) {
			issues = append(issues, Issue{
				Code:       CodeDuplicateOptionValues,
				Message:    fmt.Sprintf("Question %q has duplicate option values — the correct answer would be ambiguous.", questionID),
				QuestionID: questionID,
			})
		}

		correctValue, isStr := question["correctValue"].(string)
		if !isStr || correctValue == "" {
			issues = append(issues, Issue{
				Code:       CodeMissingCorrectValue,
				Message:    fmt.Sprintf("Question %q has no correctValue designated.", questionID),
				QuestionID: questionID,
			})
		} else if !distinct[correctValue] {
			issues = append(issues, Issue{
				Code:       CodeDanglingCorrectValue,
				Message:    fmt.Sprintf("Question %q has correctValue %q matching none of its options — it can never be answered correctly.", questionID, correctValue),
				QuestionID: questionID,
			})
		}
	}

	return issues
}

// Summarize digests an envelope. It is exact on a validated quiz and tolerant
// on anything else: non-array questions count as zero, non-object questions
// and non-string ids are skipped, and a non-integral or missing threshold
// reports as zero. QuestionIDs is never nil so JSON output emits [].
func Summarize(env map[string]interface{}) Summary {
	s := Summary{QuestionIDs: []string{}}
	if env == nil {
		return s
	}
	if threshold, ok := integral(env["passThreshold"]); ok {
		s.PassThreshold = threshold
	}
	questions, _ := env["questions"].([]interface{})
	s.QuestionCount = len(questions)
	for _, raw := range questions {
		if q, ok := raw.(map[string]interface{}); ok {
			if id, ok := q["id"].(string); ok {
				s.QuestionIDs = append(s.QuestionIDs, id)
			}
		}
	}
	return s
}

type optionRecord struct {
	value string
	label string
}

// optionRecords mirrors Array.isArray(options) && options.every(isQuizOptionRecord).
func optionRecords(raw interface{}) ([]optionRecord, bool) {
	list, ok := raw.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]optionRecord, 0, len(list))
	for _, item := range list {
		rec, ok := item.(map[string]interface{})
		if !ok {
			return nil, false
		}
		value, vOK := rec["value"].(string)
		label, lOK := rec["label"].(string)
		if !vOK || !lOK {
			return nil, false
		}
		out = append(out, optionRecord{value: value, label: label})
	}
	return out, true
}

// integral reports whether v is a JSON number with no fractional part and
// returns it as an int. JSON numbers decode as float64; 1.5 is not integral.
func integral(v interface{}) (int, bool) {
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
		return 0, false
	}
	if f > math.MaxInt32 || f < math.MinInt32 {
		return 0, false
	}
	return int(f), true
}

// formatNumber renders a decoded JSON value for a message without the
// float64 artifacts ("99" not "99.000000", "1.5" as written).
func formatNumber(v interface{}) string {
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%v", f)
	}
	return fmt.Sprintf("%v", v)
}
