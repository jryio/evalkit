package bench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jryio/evalkit/internal/agent"
	"github.com/jryio/evalkit/internal/stats"
)

// ClimbOptions says how to hill-climb.
type ClimbOptions struct {
	Prefix    string // runs are named <prefix>0 (the baseline), <prefix>1, …; default "round"
	Rounds    int    // overrides bench.climb.rounds
	SlotsRoot string
	Log       io.Writer
}

// Round is one round of a climb.
type Round struct {
	Run     string          `json:"run"`
	Change  string          `json:"change,omitempty"` // the proposer's own summary
	Train   *stats.Interval `json:"train"`
	Test    *stats.Interval `json:"test,omitempty"`
	Kept    bool            `json:"kept"`
	Reason  string          `json:"reason"`
	CostUSD float64         `json:"cost_usd"`
}

// climbState is climb.json beside the suite: where a climb stands.
type climbState struct {
	Best   string  `json:"best"`
	Rounds []Round `json:"rounds"`
}

const proposerPrompt = `You are improving something a benchmark measures.

The files you may change are under ./targets, at the same relative paths as in the project. history.md lists what earlier rounds changed and how they scored. failures.json holds the training cases the current best version still gets wrong: for each, the case, what the agent answered, and why the grader marked it down. passing.json lists the training cases that pass now.

Make one focused change under ./targets that should fix a class of these failures without breaking the cases that pass. Do not write the answer to a specific case into the files: cases you cannot see decide the final score. Change nothing outside ./targets.

When you are done, reply with one sentence that says what you changed and why.`

// Climb runs the baseline if there is none, then up to Rounds rounds: a
// proposer edits the targets, the benchmark runs, and the change is kept only
// when the train score rises. The targets end as the best round left them.
func Climb(ctx context.Context, path string, opts ClimbOptions) ([]Round, error) {
	l, err := Load(path)
	if err != nil {
		return nil, err
	}
	c := l.Config.Climb
	if c == nil || len(c.Targets) == 0 {
		return nil, errors.New("the suite has no bench.climb with targets")
	}
	if opts.Prefix == "" {
		opts.Prefix = "round"
	}
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	rounds := c.Rounds
	if opts.Rounds > 0 {
		rounds = opts.Rounds
	}
	if rounds <= 0 {
		rounds = 3
	}
	statePath := filepath.Join(l.Dir, "climb.json")
	var st climbState
	if b, err := os.ReadFile(statePath); err == nil {
		if err := json.Unmarshal(b, &st); err != nil {
			return nil, fmt.Errorf("climb.json: %w", err)
		}
	}
	save := func() error {
		b, err := json.MarshalIndent(st, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(statePath, append(b, '\n'), 0o644)
	}
	runOne := func(id string, round int, note string, baseline bool) (Round, error) {
		if err := snapshot(l.Dir, c.Targets, variantDir(l.Dir, id)); err != nil {
			return Round{}, err
		}
		sum, err := Run(ctx, l, Options{Run: id, Baseline: baseline, Note: note, Dims: map[string]any{"round": round},
			Meta: map[string]any{"parent": st.Best}, SlotsRoot: opts.SlotsRoot, Log: opts.Log})
		if err != nil {
			return Round{}, err
		}
		r := Round{Run: id, Change: note, CostUSD: sum.CostUSD}
		if r.Train, err = groupScore(path, id, c.Train); err != nil {
			return r, err
		}
		if len(c.Test) > 0 {
			r.Test, _ = groupScore(path, id, c.Test)
		}
		return r, nil
	}

	if len(st.Rounds) == 0 {
		fmt.Fprintln(opts.Log, "baseline:")
		r, err := runOne(opts.Prefix+"0", 0, "", true)
		if err != nil {
			return st.Rounds, err
		}
		r.Kept, r.Reason = true, "baseline"
		st.Best, st.Rounds = r.Run, []Round{r}
		if err := save(); err != nil {
			return st.Rounds, err
		}
	}

	for range rounds {
		best := st.round(st.Best)
		if reached(c.Target, best.Train) {
			fmt.Fprintf(opts.Log, "stopping: the train score %.3f reached the target\n", best.Train.Value)
			break
		}
		n := len(st.Rounds)
		id := fmt.Sprintf("%s%d", opts.Prefix, n)
		fmt.Fprintf(opts.Log, "round %d: proposing a change on top of %s\n", n, st.Best)
		change, patch, cost, err := propose(ctx, l, st)
		if err != nil {
			return st.Rounds, fmt.Errorf("round %d: propose: %w", n, err)
		}
		if patch == "" {
			fmt.Fprintln(opts.Log, "stopping: the proposer changed nothing")
			break
		}
		if l, err = Load(path); err != nil { // the targets may be files the suite reads
			return st.Rounds, err
		}
		r, err := runOne(id, n, change, false)
		r.CostUSD += cost
		os.WriteFile(filepath.Join(l.Dir, "runs", id, "change.patch"), []byte(patch), 0o644)
		if err != nil {
			restore(l.Dir, c.Targets, variantDir(l.Dir, st.Best))
			return st.Rounds, err
		}
		switch {
		case r.Train == nil:
			r.Reason = "no train case was scored"
		case r.Train.Value > best.Train.Value:
			r.Kept, r.Reason = true, fmt.Sprintf("train rose from %.3f to %.3f", best.Train.Value, r.Train.Value)
		default:
			r.Reason = fmt.Sprintf("train %.3f did not beat %.3f; reverted", r.Train.Value, best.Train.Value)
		}
		st.Rounds = append(st.Rounds, r)
		if r.Kept {
			st.Best = r.Run
		} else if err := restore(l.Dir, c.Targets, variantDir(l.Dir, st.Best)); err != nil {
			return st.Rounds, err
		}
		if err := save(); err != nil {
			return st.Rounds, err
		}
		fmt.Fprintf(opts.Log, "round %d: %s\n", n, r.Reason)
	}
	return st.Rounds, restore(l.Dir, c.Targets, variantDir(l.Dir, st.Best))
}

func (st climbState) round(id string) Round {
	i := slices.IndexFunc(st.Rounds, func(r Round) bool { return r.Run == id })
	return st.Rounds[i]
}

func reached(target float64, train *stats.Interval) bool {
	return target > 0 && train != nil && train.Value >= target
}

func variantDir(suiteDir, run string) string { return filepath.Join(suiteDir, "runs", run, "variant") }

// snapshot copies the target files into dir, keeping their relative paths.
func snapshot(suiteDir string, targets []string, dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for _, t := range targets {
		if err := copyPath(filepath.Join(suiteDir, t), filepath.Join(dir, t)); err != nil {
			return fmt.Errorf("climb target %s: %w", t, err)
		}
	}
	return nil
}

// restore puts a snapshot's target files back in the suite.
func restore(suiteDir string, targets []string, dir string) error {
	for _, t := range targets {
		if err := copyPath(filepath.Join(dir, t), filepath.Join(suiteDir, t)); err != nil {
			return err
		}
	}
	return nil
}

// groupScore is one run's score over the given groups (nil: every item).
func groupScore(path, run string, groups []string) (*stats.Interval, error) {
	l, err := Load(path)
	if err != nil {
		return nil, err
	}
	rep := stats.Summarize(l.Stats, groups, 1)
	if rep.Metric == nil {
		return nil, errors.New("nothing scores the runs: mark a question or a column primary")
	}
	i := slices.IndexFunc(rep.Runs, func(r stats.RunSummary) bool { return r.Run == run })
	if i < 0 {
		return nil, fmt.Errorf("run %s is not in the suite: it needs runs_from: runs/*/run.json", run)
	}
	return rep.Runs[i].Score, nil
}

// propose runs the proposer in a scratch directory that holds the targets, the
// best run's failed train cases and the history, then copies the edited
// targets back. patch is the edit as a unified diff; empty means no change.
func propose(ctx context.Context, l *Loaded, st climbState) (change, patch string, cost float64, err error) {
	c := l.Config.Climb
	dir, err := os.MkdirTemp("", "evalkit-propose-*")
	if err != nil {
		return "", "", 0, err
	}
	defer os.RemoveAll(dir)
	targets := filepath.Join(dir, "targets")
	if err := snapshot(l.Dir, c.Targets, targets); err != nil {
		return "", "", 0, err
	}
	failures, passing, err := trainCases(l, st.Best)
	if err != nil {
		return "", "", 0, err
	}
	files := map[string]any{"failures.json": failures, "passing.json": passing}
	for name, v := range files {
		b, err := json.MarshalIndent(v, "", " ")
		if err != nil {
			return "", "", 0, err
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return "", "", 0, err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "history.md"), []byte(history(st)), 0o644); err != nil {
		return "", "", 0, err
	}

	prompt := proposerPrompt
	if c.Propose.Prompt != "" {
		prompt += "\n\n" + c.Propose.Prompt
	}
	var system string
	if c.Propose.System != "" {
		b, err := os.ReadFile(filepath.Join(l.Dir, c.Propose.System))
		if err != nil {
			return "", "", 0, fmt.Errorf("bench.climb.propose.system: %w", err)
		}
		system = string(b)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(l.Config.Timeout*float64(time.Second)))
	defer cancel()
	// The proposer is the user's own improver, so it keeps their environment; bin/ is on PATH as for stages.
	env := append(os.Environ(), "EVALKIT_SUITE_DIR="+l.Dir, "PATH="+filepath.Join(l.Dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	res, err := agent.Run(ctx, agent.Request{Spec: c.Propose.Spec, Prompt: prompt, System: system, Dir: dir, Env: env})
	if err != nil {
		return "", "", res.CostUSD, err
	}
	// diff exits 1 when the trees differ.
	diff, _ := exec.CommandContext(ctx, "diff", "-ruN", variantDir(l.Dir, st.Best), targets).Output()
	if len(strings.TrimSpace(string(diff))) == 0 {
		return "", "", res.CostUSD, nil
	}
	if err := restore(l.Dir, c.Targets, targets); err != nil {
		return "", "", res.CostUSD, err
	}
	return lastLine(res.Output), string(diff), res.CostUSD, nil
}

// trainCase is one training attempt as the proposer sees it.
type trainCase struct {
	Case     Case           `json:"case"`
	Rep      int            `json:"rep,omitempty"`
	Answer   string         `json:"answer"`
	Graded   map[string]any `json:"graded,omitempty"`
	Measured map[string]any `json:"measured,omitempty"`
	Flag     any            `json:"flag,omitempty"`
}

// trainCases reads the best run's stored attempts and splits its train cases
// into those that fall short of a full score and the ids of those that do not.
// Held-out cases never leave this function.
func trainCases(l *Loaded, run string) (failures []trainCase, passing []string, err error) {
	c := l.Config.Climb
	files, err := filepath.Glob(filepath.Join(l.Dir, "runs", run, "attempts", "*.json"))
	if err != nil {
		return nil, nil, err
	}
	failures, passing = []trainCase{}, []string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, err
		}
		var p payload
		if err := json.Unmarshal(b, &p); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", f, err)
		}
		group, _ := p.Row["group"].(string)
		if len(c.Train) > 0 && !slices.Contains(c.Train, group) {
			continue
		}
		var it stats.Item
		if rb, err := json.Marshal(p.Row); err == nil {
			json.Unmarshal(rb, &it)
		}
		if score, ok := stats.ItemScore(l.Stats, it); ok && score >= 1 {
			passing = append(passing, attemptKey(p.Case.ID(), p.Rep))
			continue
		}
		tc := trainCase{Case: p.Case, Rep: p.Rep, Flag: p.Row["flag"]}
		if p.Agent != nil {
			tc.Answer = clip(p.Agent.Output, 8000)
		}
		tc.Graded, _ = p.Row["proposed"].(map[string]any)
		tc.Measured, _ = p.Row["values"].(map[string]any)
		failures = append(failures, tc)
	}
	return failures, passing, nil
}

func history(st climbState) string {
	var b strings.Builder
	b.WriteString("| run | kept | train score | what changed |\n|---|---|---|---|\n")
	for _, r := range st.Rounds {
		score := "–"
		if r.Train != nil {
			score = fmt.Sprintf("%.3f", r.Train.Value)
		}
		kept := "no (reverted)"
		if r.Kept {
			kept = "yes"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", r.Run, kept, score, strings.ReplaceAll(r.Change, "|", "/"))
	}
	fmt.Fprintf(&b, "\nThe files under ./targets are the version from %s, the best so far.\n", st.Best)
	return b.String()
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return clip(strings.TrimSpace(lines[len(lines)-1]), 200)
}
