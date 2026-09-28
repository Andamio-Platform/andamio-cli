// Package prompts recognizes, validates and summarizes prompts envelopes — a
// written assignment asked as a few short questions, each answered in its own
// box. Two shapes ride the platform's opaque JSON fields, the same way the
// quiz does:
//
//   - a definition (`type: "prompts"`) in an assignment's `content_json`;
//   - evidence (`type: "prompts-evidence"`) in a commitment's `evidence`,
//     self-contained: each answer carries its prompt's label and question.
//
// The Andamio app is the authority. fcb-fan-engagement-app is the only app
// that renders prompts today; Recognize mirrors isPromptsContentEnvelope and
// isPromptsEvidenceEnvelope, and Validate mirrors validatePromptsDefinition,
// keeping its control flow, issue codes and wording. The upstream revision is
// recorded in testdata/prompts/SOURCE.md; a rule change is re-mirrored by hand
// and pinned by the fixtures under testdata/prompts. When andamio-app-v2
// ships prompts, Validate becomes the union of both apps' rules, as quiz does.
//
// Recognition and validity are separate, as in the app: a definition with an
// unsupported version is still prompts-shaped, just not valid. Evidence has no
// validator upstream; its recognition guard is field-strict, so recognizing it
// is the validity check.
//
// The package has no dependency on cmd or on any other internal package.
package prompts

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Kind is the recognized shape of a JSON value.
type Kind int

const (
	// NotObject is any JSON value that is not an object.
	NotObject Kind = iota
	// Definition is a prompts-shaped definition: `type: "prompts"`, numeric
	// `version`, array `prompts`. Validity is a separate question.
	Definition
	// Evidence is prompts evidence: `type: "prompts-evidence"`, numeric
	// `version`, and an `answers` array whose every element carries string
	// promptId, label, question and answer.
	Evidence
	// Other is an object that is neither: a Tiptap doc, a quiz, quiz
	// evidence, or a malformed prompts envelope.
	Other
)

func (k Kind) String() string {
	switch k {
	case NotObject:
		return "not an object"
	case Definition:
		return "prompts definition"
	case Evidence:
		return "prompts evidence"
	case Other:
		return "other"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

const (
	// ContentType is the definition's `type` value.
	ContentType = "prompts"
	// EvidenceType is the evidence's `type` value.
	EvidenceType = "prompts-evidence"
	// SupportedVersion is the only envelope version the app accepts.
	SupportedVersion = 1
)

// Issue codes. The first five are the app's PromptsDefinitionIssueCode;
// CodeNotPrompts is a guard for callers that hand Validate something
// Recognize would not call a definition.
const (
	CodeUnsupportedVersion = "unsupported-version"
	CodeEmptyPrompts       = "empty-prompts"
	CodeMalformedPrompt    = "malformed-prompt"
	CodeDuplicatePromptIDs = "duplicate-prompt-ids"
	CodeMalformedIntro     = "malformed-intro"

	CodeNotPrompts = "not-prompts"
)

// AllCodes lists every issue code Validate can emit.
var AllCodes = []string{
	CodeUnsupportedVersion,
	CodeEmptyPrompts,
	CodeMalformedPrompt,
	CodeDuplicatePromptIDs,
	CodeMalformedIntro,
	CodeNotPrompts,
}

// Issue is one violated rule. PromptID is set when the rule applies to a
// specific prompt.
type Issue struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	PromptID string `json:"prompt_id,omitempty"`
}

// String renders the issue as one line suitable for stderr.
func (i Issue) String() string {
	if i.PromptID != "" {
		return fmt.Sprintf("%s [prompt %q]: %s", i.Code, i.PromptID, i.Message)
	}
	return fmt.Sprintf("%s: %s", i.Code, i.Message)
}

// Summary is the scriptable digest of a definition.
type Summary struct {
	PromptCount int      `json:"prompt_count"`
	PromptIDs   []string `json:"prompt_ids"`
}

// Answer is one answered prompt from evidence, re-keyed to the CLI's
// snake_case JSON convention (the envelope itself uses camelCase promptId).
type Answer struct {
	PromptID string `json:"prompt_id"`
	Label    string `json:"label"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Recognize decodes raw JSON and classifies its shape. The error is non-nil
// only when raw is not valid JSON. env is the decoded object for every kind
// but NotObject.
func Recognize(raw []byte) (Kind, map[string]interface{}, error) {
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return NotObject, nil, fmt.Errorf("invalid JSON: %w", err)
	}
	env, ok := value.(map[string]interface{})
	if !ok {
		return NotObject, nil, nil
	}
	return Classify(env), env, nil
}

// Classify is Recognize for an already-decoded object. nil is NotObject.
func Classify(env map[string]interface{}) Kind {
	switch {
	case env == nil:
		return NotObject
	case isDefinition(env):
		return Definition
	case isEvidence(env):
		return Evidence
	}
	return Other
}

// isDefinition mirrors isPromptsContentEnvelope: version-agnostic on purpose.
func isDefinition(env map[string]interface{}) bool {
	if env["type"] != ContentType {
		return false
	}
	if _, ok := env["version"].(float64); !ok {
		return false
	}
	_, ok := env["prompts"].([]interface{})
	return ok
}

// isEvidence mirrors isPromptsEvidenceEnvelope: field-strict on every answer,
// with no length cap.
func isEvidence(env map[string]interface{}) bool {
	_, ok := evidenceAnswers(env)
	return ok
}

func evidenceAnswers(env map[string]interface{}) ([]Answer, bool) {
	if env == nil || env["type"] != EvidenceType {
		return nil, false
	}
	if _, ok := env["version"].(float64); !ok {
		return nil, false
	}
	list, ok := env["answers"].([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]Answer, 0, len(list))
	for _, item := range list {
		rec, ok := item.(map[string]interface{})
		if !ok {
			return nil, false
		}
		promptID, idOK := rec["promptId"].(string)
		label, labelOK := rec["label"].(string)
		question, questionOK := rec["question"].(string)
		answer, answerOK := rec["answer"].(string)
		if !idOK || !labelOK || !questionOK || !answerOK {
			return nil, false
		}
		out = append(out, Answer{PromptID: promptID, Label: label, Question: question, Answer: answer})
	}
	return out, true
}

// Answers returns the evidence's answers in stored order, and false when env
// is not prompts evidence. The slice is empty, never nil, for evidence with
// no answers.
func Answers(env map[string]interface{}) ([]Answer, bool) {
	return evidenceAnswers(env)
}

// Parse is Recognize followed by Validate when the value is a definition.
// issues is empty for a valid definition and always empty for other kinds.
func Parse(raw []byte) (env map[string]interface{}, kind Kind, issues []Issue, err error) {
	kind, env, err = Recognize(raw)
	if err != nil {
		return nil, kind, nil, err
	}
	if kind == Definition {
		issues = Validate(env)
	}
	return env, kind, issues, nil
}

// Validate mirrors the app's validatePromptsDefinition, in its check order:
// version, intro, empty prompts (which returns early), then each prompt. It
// collects every issue and never panics on malformed elements. env is
// expected to be a Definition; anything else yields a single not-prompts
// issue rather than a false pass.
func Validate(env map[string]interface{}) []Issue {
	issues := []Issue{}

	if env == nil || !isDefinition(env) {
		return append(issues, Issue{
			Code:    CodeNotPrompts,
			Message: `Not a prompts envelope — expected an object with type "prompts", a numeric version and a prompts array.`,
		})
	}

	if version := env["version"].(float64); version != SupportedVersion {
		issues = append(issues, Issue{
			Code:    CodeUnsupportedVersion,
			Message: fmt.Sprintf("Envelope version %v is not supported (expected %d).", version, SupportedVersion),
		})
	}

	if intro, present := env["intro"]; present && intro != nil {
		if _, isObj := intro.(map[string]interface{}); !isObj {
			issues = append(issues, Issue{
				Code:    CodeMalformedIntro,
				Message: "intro must be a rich-text document object when present.",
			})
		}
	}

	rawPrompts := env["prompts"].([]interface{})
	if len(rawPrompts) == 0 {
		return append(issues, Issue{
			Code:    CodeEmptyPrompts,
			Message: "The assignment has no prompts.",
		})
	}

	seenIDs := map[string]bool{}
	for _, rawPrompt := range rawPrompts {
		prompt, isObj := rawPrompt.(map[string]interface{})
		if !isObj {
			issues = append(issues, Issue{
				Code:    CodeMalformedPrompt,
				Message: "A prompts entry is not an object.",
			})
			continue
		}

		promptID, ok := nonEmptyString(prompt["id"])
		if !ok {
			issues = append(issues, Issue{
				Code:    CodeMalformedPrompt,
				Message: "A prompt is missing a non-empty string id.",
			})
			continue
		}

		if seenIDs[promptID] {
			issues = append(issues, Issue{
				Code:     CodeDuplicatePromptIDs,
				Message:  fmt.Sprintf("Prompt id %q is used more than once — answers are matched back to their box by id.", promptID),
				PromptID: promptID,
			})
		}
		seenIDs[promptID] = true

		if _, ok := nonEmptyString(prompt["label"]); !ok {
			issues = append(issues, Issue{
				Code:     CodeMalformedPrompt,
				Message:  fmt.Sprintf("Prompt %q has no label.", promptID),
				PromptID: promptID,
			})
		}
		if _, ok := nonEmptyString(prompt["question"]); !ok {
			issues = append(issues, Issue{
				Code:     CodeMalformedPrompt,
				Message:  fmt.Sprintf("Prompt %q has no question.", promptID),
				PromptID: promptID,
			})
		}
	}

	return issues
}

// Summarize digests a definition. It is exact on a validated definition and
// tolerant on anything else: non-array prompts count as zero, and non-object
// prompts and non-string ids are counted but not listed. PromptIDs is never
// nil so JSON output emits [].
func Summarize(env map[string]interface{}) Summary {
	s := Summary{PromptIDs: []string{}}
	if env == nil {
		return s
	}
	list, _ := env["prompts"].([]interface{})
	s.PromptCount = len(list)
	for _, raw := range list {
		if p, ok := raw.(map[string]interface{}); ok {
			if id, ok := p["id"].(string); ok {
				s.PromptIDs = append(s.PromptIDs, id)
			}
		}
	}
	return s
}

// nonEmptyString mirrors the app's isNonEmptyString: a string with
// non-whitespace content. The original value is returned untrimmed.
func nonEmptyString(v interface{}) (string, bool) {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", false
	}
	return s, true
}
