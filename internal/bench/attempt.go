package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/jryio/evalkit/internal/agent"
	"github.com/jryio/evalkit/internal/workspace"
)

// Row is one item line of a run, or a stage's fragment of one.
type Row map[string]any

// payload is what every stage reads on stdin, and what attempts/<key>.json keeps.
type payload struct {
	Case      Case              `json:"case"`
	Run       string            `json:"run"`
	Rep       int               `json:"rep"`
	SuiteDir  string            `json:"suite_dir"`
	Workspace string            `json:"workspace"`
	Cwd       string            `json:"cwd"`
	Out       string            `json:"out"`
	Sources   map[string]string `json:"sources,omitempty"`
	Prompt    string            `json:"prompt,omitempty"`
	Agent     *agent.Result     `json:"agent,omitempty"`
	Diff      map[string]string `json:"diff,omitempty"`
	Row       Row               `json:"row,omitempty"`
}

type attemptResult struct {
	row      Row
	cost     float64
	seconds  float64
	err      error
	class    string
	judgeErr error
}

func failed(res attemptResult, err error) attemptResult {
	res.err, res.class = err, agent.Harness
	if f, ok := errors.AsType[*agent.Failure](err); ok {
		res.class = f.Class
	}
	return res
}

// once makes one try of an attempt in a slot of its own.
func (r *runner) once(ctx context.Context, j job) (res attemptResult) {
	start := time.Now()
	defer func() { res.seconds = time.Since(start).Seconds() }()

	slot, err := r.pool.Acquire(ctx, j.c.pins())
	if err != nil {
		return failed(res, err)
	}
	defer r.pool.Release(slot)

	out := filepath.Join(r.runDir, "attempts", attemptKey(j.c.ID(), j.rep))
	if err := os.RemoveAll(out); err != nil {
		return failed(res, err)
	}
	for _, d := range []string{"tmp", "home"} {
		if err := os.MkdirAll(filepath.Join(out, d), 0o755); err != nil {
			return failed(res, err)
		}
	}
	if err := os.WriteFile(filepath.Join(out, "gitconfig"), []byte(attemptGitConfig), 0o644); err != nil {
		return failed(res, err)
	}
	for _, o := range r.cfg.Overlay {
		if err := copyPath(filepath.Join(r.l.Dir, o.From), filepath.Join(slot.Dir, o.To)); err != nil {
			return failed(res, fmt.Errorf("overlay %s: %w", o.From, err))
		}
	}
	p := payload{Case: j.c.withoutAnswers(), Run: r.opts.Run, Rep: j.rep, SuiteDir: r.l.Dir,
		Workspace: slot.Dir, Cwd: r.cwd(slot), Out: out, Sources: map[string]string{}}
	for _, name := range slot.Sources() {
		p.Sources[name] = slot.Path(name)
	}
	env := r.env(p)
	limit := time.Duration(r.cfg.Timeout * float64(time.Second))

	if r.cfg.Prepare != "" {
		if _, err := r.stage(ctx, "prepare", r.cfg.Prepare, p, env, limit); err != nil {
			return failed(res, err)
		}
	}

	if p.Prompt, err = renderPrompt(r.cfg.Agent, p.Case); err != nil {
		return failed(res, err)
	}
	req := agent.Request{Spec: r.cfg.Agent.Spec, Prompt: p.Prompt, System: r.system, Dir: p.Cwd, Env: env}
	if req.Spec.Adapter == agent.Command {
		req.Stdin, _ = json.Marshal(p)
	}
	if r.cfg.Sandbox {
		if req.Wrap, err = sandboxWrap(out, []string{slot.Dir, out}, []string{r.l.Dir, r.slots, r.opts.RunsDir}); err != nil {
			return failed(res, err)
		}
	}
	agentCtx, cancel := context.WithTimeout(ctx, limit)
	ar, err := agent.Run(agentCtx, req)
	cancel()
	res.cost = ar.CostUSD
	if err != nil {
		return failed(res, err)
	}
	// Only claude reports the skills it loaded; a stand-in agent has none to report.
	if r.cfg.Agent.Adapter == agent.Claude {
		for _, skill := range r.cfg.Agent.RequireSkills {
			if !slices.Contains(ar.Skills, skill) {
				return failed(res, fmt.Errorf("the agent did not load the skill %q, so this attempt did not test it", skill))
			}
		}
	}
	p.Agent = &ar
	var overlaid []string
	for _, o := range r.cfg.Overlay {
		overlaid = append(overlaid, o.To)
	}
	if p.Diff, err = slot.Diff(ctx, overlaid...); err != nil {
		return failed(res, err)
	}

	p.Case = j.c // the graders see the answers
	p.Row = baseRow(j.c, j.rep)
	stages := append([]string{r.cfg.Collect}, r.cfg.Score...)
	for i, command := range stages {
		if command == "" {
			continue
		}
		name := "collect"
		if i > 0 {
			name = fmt.Sprintf("score[%d]", i-1)
		}
		frag, err := r.stage(ctx, name, command, p, env, limit)
		if err != nil {
			return failed(res, err)
		}
		if err := merge(p.Row, frag); err != nil {
			return failed(res, fmt.Errorf("%s: %w", name, err))
		}
	}
	r.fillDefaults(p)
	if r.cfg.Judge != nil {
		cost, err := r.judge(ctx, p, env, limit)
		res.cost += cost
		if err != nil {
			res.judgeErr = err
			p.Row["flag"] = judgeFailed + err.Error()
		}
		if cost > 0 {
			r.setValue(p.Row, "judge_cost_usd", round(cost, 4))
		}
	}
	savePayload(p)
	res.row = p.Row
	return res
}

// savePayload keeps the attempt beside its run, so it can be graded again
// without running the agent, and drops the attempt's scratch directories.
func savePayload(p payload) {
	if b, err := json.MarshalIndent(p, "", " "); err == nil {
		os.WriteFile(p.Out+".json", b, 0o644)
	}
	for _, d := range []string{"tmp", "home", "judge", "sandbox.sb", "gitconfig"} {
		os.RemoveAll(filepath.Join(p.Out, d))
	}
	os.Remove(p.Out) // gone unless a stage left files there
}

// cwd is the agent's directory: bench.cwd, else the only source, else the slot.
func (r *runner) cwd(slot *workspace.Slot) string {
	if r.cfg.Cwd != "" {
		return filepath.Join(slot.Dir, r.cfg.Cwd)
	}
	if names := slot.Sources(); len(names) == 1 {
		return slot.Path(names[0])
	}
	return slot.Dir
}

// passed are the variables every attempt inherits; passedPrefixes cover the
// agent CLIs' own settings and credentials. Everything else stays out unless
// bench.env names it.
var (
	passed         = []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "LANG", "LC_ALL", "LC_CTYPE", "TERM", "TZ", "SSL_CERT_FILE", "SSL_CERT_DIR"}
	passedPrefixes = []string{"ANTHROPIC_", "CLAUDE_", "OPENAI_", "CODEX_", "PI_", "XDG_"}
)

// env is the environment of an attempt's agent and stages.
func (r *runner) env(p payload) []string {
	all := slices.Contains(r.cfg.Env, "*")
	vars := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if all || slices.Contains(passed, k) || slices.Contains(r.cfg.Env, k) || slices.ContainsFunc(passedPrefixes, func(pre string) bool { return strings.HasPrefix(k, pre) }) {
			vars[k] = v
		}
	}
	vars["PATH"] = filepath.Join(r.l.Dir, "bin") + string(os.PathListSeparator) + vars["PATH"]
	vars["TMPDIR"] = filepath.Join(p.Out, "tmp")
	// The agent's git reads this file and not the user's config: no signing
	// prompt, hooks or credential helper, and the same identity on every machine.
	vars["GIT_CONFIG_GLOBAL"] = filepath.Join(p.Out, "gitconfig")
	vars["GIT_CONFIG_SYSTEM"] = os.DevNull
	vars["GIT_TERMINAL_PROMPT"] = "0"
	if r.cfg.Home == "private" {
		vars["HOME"] = filepath.Join(p.Out, "home")
	}
	vars["EVALKIT_SUITE_DIR"] = p.SuiteDir
	vars["EVALKIT_WORKSPACE"] = p.Workspace
	vars["EVALKIT_CWD"] = p.Cwd
	vars["EVALKIT_OUT"] = p.Out
	vars["EVALKIT_RUN"] = p.Run
	vars["EVALKIT_CASE"] = p.Case.ID()
	vars["EVALKIT_REP"] = fmt.Sprint(p.Rep)
	out := make([]string, 0, len(vars))
	for k, v := range vars {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}

// stage runs one shell stage with the payload on stdin and reads the row
// fragment it prints, if any.
func (r *runner) stage(ctx context.Context, name, command string, p payload, env []string, limit time.Duration) (Row, error) {
	stdin, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	dir := p.Cwd
	if _, err := os.Stat(dir); err != nil {
		dir = p.Out
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	out, err := agent.Exec(ctx, command, stdin, dir, env)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil, nil
	}
	var frag Row
	if err := json.Unmarshal([]byte(trimmed), &frag); err != nil {
		return nil, fmt.Errorf("%s printed something that is not one JSON object: %.200s", name, trimmed)
	}
	return frag, nil
}

// renderPrompt fills the agent's prompt template from the case. Without a
// template the prompt is the case's prompt field; a command agent, which reads
// the whole case on stdin, may have neither.
func renderPrompt(a AgentStage, c Case) (string, error) {
	tmpl := a.Prompt
	if tmpl == "" {
		if _, has := c["prompt"]; !has && a.Adapter == agent.Command {
			return "", nil
		}
		tmpl = "{{.prompt}}"
	}
	t, err := template.New("prompt").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("bench.agent.prompt: %w", err)
	}
	var b bytes.Buffer
	if err := t.Execute(&b, map[string]any(c)); err != nil {
		return "", fmt.Errorf("case %s: the prompt template needs a field the case lacks: %w", c.ID(), err)
	}
	return b.String(), nil
}

func baseRow(c Case, rep int) Row {
	row := Row{"id": c.ID(), "title": c.ID()}
	if rep > 0 {
		row["rep"] = rep
	}
	for _, k := range []string{"title", "short", "subtitle", "group"} {
		if s, ok := c[k].(string); ok && s != "" {
			row[k] = s
		}
	}
	return row
}

// fragmentKeys are the item keys a stage may set.
var fragmentKeys = []string{"title", "short", "subtitle", "group", "badges", "meta", "show", "context", "values", "proposed", "flag"}

// merge adds a stage's fragment to the row: show and context append, values
// and proposed merge by key, everything else replaces.
func merge(row, frag Row) error {
	for k, v := range frag {
		if !slices.Contains(fragmentKeys, k) {
			return fmt.Errorf("a stage may not set %q; it may set %s", k, strings.Join(fragmentKeys, ", "))
		}
		switch k {
		case "show", "context":
			add, ok := v.([]any)
			if !ok {
				return fmt.Errorf("%s must be a list of artifacts", k)
			}
			have, _ := row[k].([]any)
			row[k] = append(have, add...)
		case "values", "proposed":
			add, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("%s must be an object", k)
			}
			have, _ := row[k].(map[string]any)
			if have == nil {
				have = map[string]any{}
			}
			for kk, vv := range add {
				have[kk] = vv
			}
			row[k] = have
		default:
			row[k] = v
		}
	}
	return nil
}

// attemptGitConfig is the whole global git config of an agent under test.
const attemptGitConfig = "[user]\n\tname = evalkit\n\temail = evalkit@localhost\n[gc]\n\tauto = 0\n[maintenance]\n\tauto = false\n"

const judgeFailed = "The judge failed, so this row has no verdict: "

const maxToolOutput = 4000

// fillDefaults gives the row what no stage gave it: the answer and the diffs
// to review, the transcript and prompt as context, and the measured columns
// the suite declares.
func (r *runner) fillDefaults(p payload) {
	row, a := p.Row, p.Agent
	if _, ok := row["show"]; !ok {
		var show []any
		if strings.TrimSpace(a.Output) != "" {
			show = append(show, map[string]any{"kind": "markdown", "label": "Answer", "text": a.Output})
		}
		for _, name := range sortedKeys(p.Diff) {
			if d := p.Diff[name]; strings.TrimSpace(d) != "" {
				show = append(show, map[string]any{"kind": "diff", "label": "Changes in " + name, "diff": d})
			}
		}
		if len(show) == 0 {
			show = append(show, map[string]any{"kind": "text", "label": "Answer", "text": "(the agent produced no answer and changed nothing)"})
		}
		row["show"] = show
	}
	context, _ := row["context"].([]any)
	if len(a.Transcript) > 0 {
		turns := slices.Clone(a.Transcript)
		for i, t := range turns {
			if len(t.Output) > maxToolOutput {
				turns[i].Output = t.Output[:maxToolOutput] + "\n… (cut)"
			}
		}
		context = append(context, map[string]any{"kind": "transcript", "label": "Agent transcript", "rows": turns})
	}
	if p.Prompt != "" {
		context = append(context, map[string]any{"kind": "text", "label": "Prompt", "text": p.Prompt})
	}
	if len(context) > 0 {
		row["context"] = context
	}
	if a.CostUSD > 0 { // codex reports tokens, not dollars: an unknown cost is not a cost of zero
		r.setValue(row, "cost_usd", round(a.CostUSD, 4))
	}
	r.setValue(row, "latency_s", round(a.Seconds, 1))
	r.setValue(row, "turns", a.Turns)
	r.setValue(row, "tokens_in", a.Tokens.In+a.Tokens.CacheRead+a.Tokens.CacheWrite)
	r.setValue(row, "tokens_out", a.Tokens.Out)
}

// setValue sets a measured value when the suite declares the column and no
// stage set it.
func (r *runner) setValue(row Row, column string, v any) {
	if !r.l.Columns[column] {
		return
	}
	values, _ := row["values"].(map[string]any)
	if values == nil {
		values = map[string]any{}
		row["values"] = values
	}
	if _, set := values[column]; !set {
		values[column] = v
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// copyPath copies a file or directory over whatever is at dst.
func copyPath(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if fi.IsDir() {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		return os.CopyFS(dst, os.DirFS(src))
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, fi.Mode().Perm())
}
