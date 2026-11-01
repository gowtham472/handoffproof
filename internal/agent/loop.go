package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Redact masks secret-like strings (hp-untrusted-input, hp-secrets-and-safety).
// It classifies by meaning: any credential-shaped value is masked, whatever
// key name or spelling carries it (quoted JSON keys, long field names, short
// values). Unrecognized credential shapes fail closed via the generic
// key=value rule rather than passing through.
func Redact(s string) string {
	s = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`).ReplaceAllString(s, "[REDACTED:private-key]")
	s = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`).ReplaceAllString(s, "[REDACTED:aws-key]")
	s = regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}\b|\bgithub_pat_[A-Za-z0-9_]{20,}\b`).ReplaceAllString(s, "[REDACTED:github-token]")
	s = regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`).ReplaceAllString(s, "[REDACTED:slack-token]")
	s = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`).ReplaceAllString(s, "[REDACTED:api-key]")
	s = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{4,}\b`).ReplaceAllString(s, "[REDACTED:jwt]")
	s = regexp.MustCompile(`(?i)(authorization\s*:\s*(?:bearer\s+)?)[^\s"',;]+`).ReplaceAllString(s, "${1}[REDACTED:token]")
	s = regexp.MustCompile(`Bearer\s+[A-Za-z0-9._\-]{8,}`).ReplaceAllString(s, "Bearer [REDACTED:token]")
	s = regexp.MustCompile(`(?i)((?:api[_-]?key|apikey|secret|password|passwd|pwd|token|auth|credential|private[_-]?key)["']?\s*[:=]\s*["']?)[^\s"',;]{4,}`).ReplaceAllString(s, "${1}[REDACTED:secret]")
	s = regexp.MustCompile(`([a-z][a-z0-9+.-]*://[^:\s/]+:)[^@\s]+@`).ReplaceAllString(s, "${1}[REDACTED:connection-string]@")
	return s
}

// Runner executes model phases with tool use and schema validation.
type Runner struct {
	Cfg    Config
	Spec   *Spec
	Exec   *Executor
	budget budget
}

type budget struct {
	perPhase map[string]int
	total    int
}

// NewRunner builds a runner. The executor may be nil for text-only phases.
func NewRunner(cfg Config, spec *Spec, exec *Executor) *Runner {
	return &Runner{Cfg: cfg, Spec: spec, Exec: exec, budget: budget{perPhase: map[string]int{}}}
}

// UsageTotal accumulates token usage across a run.
type UsageTotal struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Calls        int
}

// PhaseResult is one validated phase output.
type PhaseResult struct {
	Phase string
	JSON  json.RawMessage
	Usage UsageTotal
}

var phaseCalls = 40  // max_tool_calls_per_phase
var totalCalls = 200 // max_total_tool_calls_per_run

// maxPhaseTurns caps model turns (responses) in one phase. Tool turns and
// schema retries are counted separately: a schema failure never consumes
// tool-turn budget, and a tool turn never consumes a schema retry.
var maxPhaseTurns = 24

// maxSchemaRetries caps consecutive schema-validation failures per phase.
var maxSchemaRetries = 2

// RunPhase runs one phase: model turns with tools until it returns JSON
// validating against the phase schema. Schema failures are retried with
// the validation error (up to maxSchemaRetries in a row), then the run
// fails closed. Turns are capped separately at maxPhaseTurns.
func (r *Runner) RunPhase(ctx context.Context, phase, task string) (*PhaseResult, error) {
	schema, ok := r.Spec.Schemas[phase]
	if !ok {
		return nil, fmt.Errorf("unknown phase %q", phase)
	}
	input := []any{userText(task)}
	var prevID string
	var usage UsageTotal
	violations := ""
	turns := 0
	schemaRetries := 0
	for {
		turns++
		if turns > maxPhaseTurns {
			return nil, fmt.Errorf("phase %s: exceeded turn limit (%d)", phase, maxPhaseTurns)
		}
		req := responseRequest{
			Model:           r.Cfg.ModelID,
			Instructions:    r.Spec.SystemPrompt,
			Input:           input,
			Tools:           r.Spec.ResponsesTools(),
			Text:            Format(phase, schema),
			MaxOutputTokens: r.Spec.MaxOutputTokens,
			PreviousID:      prevID,
		}
		if violations != "" {
			// Schema feedback replaces the task input on retry.
			req.Input = []any{userText("Your previous output failed schema validation:\n" + violations + "\nReturn corrected JSON only.")}
			req.PreviousID = prevID
		}
		resp, err := r.Cfg.CreateResponse(ctx, req)
		if err != nil {
			return nil, err
		}
		usage.InputTokens += resp.Usage.InputTokens
		usage.OutputTokens += resp.Usage.OutputTokens
		usage.TotalTokens += resp.Usage.TotalTokens
		usage.Calls++
		prevID = resp.ID
		calls := resp.Calls()
		if len(calls) > 0 {
			if r.Exec == nil {
				return nil, fmt.Errorf("model requested tools but no executor is configured")
			}
			var outs []any
			for _, call := range calls {
				if err := r.spend(phase); err != nil {
					return nil, err
				}
				result, err := r.Exec.Execute(ctx, call.Name, call.Arguments)
				if err != nil {
					result = `{"error":` + jsonString(err.Error()) + `}`
				}
				outs = append(outs, map[string]any{
					"type":    "function_call_output",
					"call_id": call.CallID,
					"output":  result,
				})
			}
			input = outs
			violations = ""
			schemaRetries = 0 // a tool turn breaks any schema-failure streak
			continue
		}
		text := strings.TrimSpace(resp.Text())
		if text == "" {
			return nil, fmt.Errorf("phase %s: empty model output", phase)
		}
		var v any
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			violations = "output is not valid JSON: " + err.Error()
		} else if probs := ValidateSchema(schema, v); len(probs) > 0 {
			violations = strings.Join(probs, "; ")
		} else {
			return &PhaseResult{Phase: phase, JSON: json.RawMessage(text), Usage: usage}, nil
		}
		schemaRetries++
		if schemaRetries > maxSchemaRetries {
			return nil, fmt.Errorf("phase %s: schema validation failed after retries: %s", phase, violations)
		}
	}
}

func (r *Runner) spend(phase string) error {
	r.budget.perPhase[phase]++
	r.budget.total++
	if r.budget.perPhase[phase] > phaseCalls {
		return fmt.Errorf("tool budget exceeded for phase %s (%d)", phase, phaseCalls)
	}
	if r.budget.total > totalCalls {
		return fmt.Errorf("tool budget exceeded for run (%d)", totalCalls)
	}
	return nil
}

func userText(text string) map[string]any {
	return map[string]any{
		"role":    "user",
		"content": []map[string]any{{"type": "input_text", "text": text}},
	}
}

func jsonString(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}
