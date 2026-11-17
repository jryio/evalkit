package bench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jryio/evalkit/internal/agent"
	"github.com/jryio/evalkit/internal/suite"
)

const judgeInstructions = `

Grade the answer below against the instructions above. For each question give a value and, in "why", one plain sentence a newcomer could follow. Judge only what is in front of you; do not run tools.`

// judgeQuestions are the suite questions the judge answers.
func (r *runner) judgeQuestions() []suite.Question {
	var out []suite.Question
	for _, q := range r.l.Suite.Questions {
		if q.Each != "" || !slices.Contains([]string{"choice", "number", "scale", "toggle"}, q.Kind) {
			continue
		}
		if len(r.cfg.Judge.Questions) > 0 && !slices.Contains(r.cfg.Judge.Questions, q.ID) {
			continue
		}
		out = append(out, q)
	}
	return out
}

// judgeSchema is the JSON Schema of the judge's answer: {question: {value, why}}.
func judgeSchema(questions []suite.Question) ([]byte, error) {
	props := map[string]any{}
	var required []string
	for _, q := range questions {
		var value map[string]any
		switch q.Kind {
		case "choice":
			var enum []any
			for _, o := range q.Options {
				enum = append(enum, o.Value)
			}
			value = map[string]any{"enum": enum}
		case "toggle":
			value = map[string]any{"type": "boolean"}
		default:
			value = map[string]any{"type": "number"}
			if q.Min != nil {
				value["minimum"] = *q.Min
			}
			if q.Max != nil {
				value["maximum"] = *q.Max
			}
		}
		if q.Label != "" {
			value["description"] = q.Label
		}
		props[q.ID] = map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"value", "why"},
			"properties": map[string]any{"value": value, "why": map[string]any{"type": "string"}},
		}
		required = append(required, q.ID)
	}
	return json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": props})
}

// judge asks the judge model for a proposal per question and puts them on the
// row. It returns what the judge cost.
func (r *runner) judge(ctx context.Context, p payload, env []string, limit time.Duration) (float64, error) {
	j := r.cfg.Judge
	questions := r.judgeQuestions()
	if len(questions) == 0 {
		return 0, errors.New("the suite has no choice, number, scale or toggle question for the judge to answer")
	}
	rubric, err := os.ReadFile(filepath.Join(r.l.Dir, j.Rubric))
	if err != nil {
		return 0, fmt.Errorf("bench.judge.rubric: %w", err)
	}
	schema, err := judgeSchema(questions)
	if err != nil {
		return 0, err
	}
	// The judge works in an empty directory with no tools: it sees only the prompt.
	dir := filepath.Join(p.Out, "judge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	spec := j.Spec
	spec.Tools = "none"
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	res, err := agent.Run(ctx, agent.Request{Spec: spec, Prompt: judgePrompt(p), System: string(rubric) + judgeInstructions, Schema: schema, Dir: dir, Env: env})
	if err != nil {
		return res.CostUSD, err
	}
	var answers map[string]struct {
		Value any    `json:"value"`
		Why   string `json:"why"`
	}
	if err := json.Unmarshal(res.Structured, &answers); err != nil {
		return res.CostUSD, fmt.Errorf("the judge's answer is not {question: {value, why}}: %w", err)
	}
	proposed := Row{}
	for _, q := range questions {
		a, ok := answers[q.ID]
		if !ok {
			return res.CostUSD, fmt.Errorf("the judge gave no answer for %q", q.ID)
		}
		proposed[q.ID] = map[string]any{"value": a.Value, "why": a.Why}
	}
	return res.CostUSD, merge(p.Row, Row{"proposed": map[string]any(proposed)})
}

const maxJudgeText = 60_000

// judgePrompt is the attempt as the judge sees it: the case with its expected
// answer, what the agent said and changed, and what the scorers measured.
func judgePrompt(p payload) string {
	var b strings.Builder
	kase, _ := json.MarshalIndent(p.Case, "", " ")
	fmt.Fprintf(&b, "<case>\n%s\n</case>\n\n<answer>\n%s\n</answer>\n", kase, clip(p.Agent.Output, maxJudgeText))
	for _, name := range sortedKeys(p.Diff) {
		if d := p.Diff[name]; strings.TrimSpace(d) != "" {
			fmt.Fprintf(&b, "\n<changes source=%q>\n%s\n</changes>\n", name, clip(d, maxJudgeText))
		}
	}
	if values, ok := p.Row["values"].(map[string]any); ok && len(values) > 0 {
		measured, _ := json.Marshal(values)
		fmt.Fprintf(&b, "\n<measured>\n%s\n</measured>\n", measured)
	}
	return b.String()
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "\n… (cut)"
	}
	return s
}
