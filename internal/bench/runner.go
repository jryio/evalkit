package bench

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jryio/evalkit/internal/agent"
	"github.com/jryio/evalkit/internal/workspace"
)

// Options says which run to make.
type Options struct {
	Run         string         // the run's id
	Label, Note string         //
	Baseline    bool           // the run every other run is compared with
	Dims        map[string]any // what this run varies, by dimension id
	Meta        map[string]any // extra provenance for the manifest
	Only        []string       // run only these case ids
	Limit       int            // run only the first n cases
	Reps        int            // overrides bench.reps
	Concurrency int            // overrides bench.concurrency
	SlotsRoot   string         // where workspaces live; default: the user cache
	RunsDir     string         // where runs are written; default: runs/ beside the suite
	Agent       *AgentStage    // stands in for bench.agent
	NoJudge     bool           // skip the judge stage
	Log         io.Writer      // progress lines
}

// Summary is what a run did.
type Summary struct {
	Run             string         `json:"run"`
	Dir             string         `json:"dir"`
	Planned         int            `json:"planned"` // attempts this invocation set out to make
	Done            int            `json:"done"`    // rows written
	Resumed         int            `json:"resumed"` // attempts an earlier invocation already finished
	Failed          int            `json:"failed"`  // attempts with no row
	ByClass         map[string]int `json:"by_class,omitempty"`
	JudgeErrors     int            `json:"judge_errors,omitempty"`
	NotRun          int            `json:"not_run,omitempty"` // attempts the budget stopped
	CostUSD         float64        `json:"cost_usd"`
	Seconds         float64        `json:"seconds"`
	StoppedAtBudget bool           `json:"stopped_at_budget,omitempty"`
}

type job struct {
	c   Case
	rep int
}

type runner struct {
	l      *Loaded
	cfg    Config
	opts   Options
	runDir string
	pool   *workspace.Pool
	slots  string
	system string // the agent's system prompt file, read once
	log    io.Writer

	mu  sync.Mutex
	sum Summary
}

// SlotsRoot is where a suite's workspaces live by default: outside the suite,
// so an agent that walks up from its directory never finds the cases.
func SlotsRoot(suiteDir string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(suiteDir))
	return filepath.Join(cache, "evalkit", filepath.Base(suiteDir)+"-"+hex.EncodeToString(sum[:4]), "slots"), nil
}

// Run makes or resumes one run of the benchmark.
func Run(ctx context.Context, l *Loaded, opts Options) (Summary, error) {
	if !idRe.MatchString(opts.Run) {
		return Summary{}, fmt.Errorf("run id %q: use letters, digits, '_', '.' and '-'", opts.Run)
	}
	cfg := l.Config
	if opts.Reps > 0 {
		cfg.Reps = opts.Reps
	}
	if opts.Concurrency > 0 {
		cfg.Concurrency = opts.Concurrency
	}
	if opts.Agent != nil {
		cfg.Agent = *opts.Agent
	}
	if opts.NoJudge {
		cfg.Judge = nil
	}
	if opts.RunsDir == "" {
		opts.RunsDir = filepath.Join(l.Dir, "runs")
	}
	if opts.SlotsRoot == "" {
		root, err := SlotsRoot(l.Dir)
		if err != nil {
			return Summary{}, err
		}
		opts.SlotsRoot = root
	}
	r := &runner{l: l, cfg: cfg, opts: opts, runDir: filepath.Join(opts.RunsDir, opts.Run), slots: opts.SlotsRoot, log: opts.Log}
	if r.log == nil {
		r.log = io.Discard
	}
	r.sum = Summary{Run: opts.Run, Dir: r.runDir, ByClass: map[string]int{}}
	if err := os.MkdirAll(filepath.Join(r.runDir, "attempts"), 0o755); err != nil {
		return r.sum, err
	}
	if cfg.Agent.System != "" {
		b, err := os.ReadFile(filepath.Join(l.Dir, cfg.Agent.System))
		if err != nil {
			return r.sum, fmt.Errorf("bench.agent.system: %w", err)
		}
		r.system = string(b)
	}

	done, err := doneAttempts(r.rowsPath())
	if err != nil {
		return r.sum, err
	}
	var todo []job
	for i, c := range l.Cases {
		if len(opts.Only) > 0 && !slices.Contains(opts.Only, c.ID()) {
			continue
		}
		if opts.Limit > 0 && i >= opts.Limit {
			break
		}
		for rep := range cfg.Reps {
			if done[attemptKey(c.ID(), rep)] {
				r.sum.Resumed++
				continue
			}
			todo = append(todo, job{c, rep})
		}
	}
	r.sum.Planned = len(todo)

	if r.pool, err = workspace.NewPool(ctx, cfg.Workspace, l.Dir, r.slots, cfg.Concurrency); err != nil {
		return r.sum, err
	}
	started := time.Now()
	man := r.manifest(ctx, started)
	if err := r.writeManifest(man); err != nil {
		return r.sum, err
	}
	fmt.Fprintf(r.log, "run %s: %d attempts to make (%d already done), %d at a time\n", opts.Run, len(todo), r.sum.Resumed, cfg.Concurrency)

	jobs := make(chan job)
	var wg sync.WaitGroup
	for range cfg.Concurrency {
		wg.Go(func() {
			for j := range jobs {
				// Checked here, not when the job was queued: by now the attempts before it have been billed.
				if !r.overBudget() {
					r.attempt(ctx, j)
				}
			}
		})
	}
feed:
	for _, j := range todo {
		select {
		case jobs <- j:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	r.sum.Seconds = time.Since(started).Seconds()
	r.sum.NotRun = r.sum.Planned - r.sum.Done - r.sum.Failed
	r.sum.StoppedAtBudget = r.sum.NotRun > 0 && r.overBudget()
	man.Meta["finished_at"] = time.Now().UTC().Format(time.RFC3339)
	man.Meta["cost_usd"] = round(r.sum.CostUSD, 4)
	man.Meta["seconds"] = round(r.sum.Seconds, 1)
	man.Meta["rows"] = r.sum.Done + r.sum.Resumed
	man.Meta["failed"] = r.sum.Failed
	if err := r.writeManifest(man); err != nil {
		return r.sum, err
	}
	return r.sum, ctx.Err()
}

func (r *runner) rowsPath() string   { return filepath.Join(r.runDir, "rows.jsonl") }
func (r *runner) errorsPath() string { return filepath.Join(r.runDir, "errors.jsonl") }

func (r *runner) overBudget() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg.BudgetUSD > 0 && r.sum.CostUSD >= r.cfg.BudgetUSD
}

func attemptKey(id string, rep int) string { return fmt.Sprintf("%s.r%d", id, rep) }

// doneAttempts is the (case, rep) keys that already have a row.
func doneAttempts(rowsPath string) (map[string]bool, error) {
	done := map[string]bool{}
	b, err := os.ReadFile(rowsPath)
	if errors.Is(err, os.ErrNotExist) {
		return done, nil
	}
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(nil, 64<<20)
	for sc.Scan() {
		var row struct {
			ID  string `json:"id"`
			Rep int    `json:"rep"`
		}
		if json.Unmarshal(sc.Bytes(), &row) == nil && row.ID != "" {
			done[attemptKey(row.ID, row.Rep)] = true
		}
	}
	return done, sc.Err()
}

// manifest is the run as the suite's runs_from reads it.
type manifest struct {
	ID        string         `json:"id"`
	Label     string         `json:"label,omitempty"`
	Note      string         `json:"note,omitempty"`
	Baseline  bool           `json:"baseline,omitempty"`
	StartedAt string         `json:"started_at"`
	ItemsFrom string         `json:"items_from"`
	Dims      map[string]any `json:"dims,omitempty"`
	Meta      map[string]any `json:"meta"`
}

func (r *runner) manifestPath() string { return filepath.Join(r.runDir, "run.json") }

func (r *runner) manifest(ctx context.Context, started time.Time) manifest {
	// A resumed run keeps the time it first started, so it keeps its place among the runs.
	startedAt := started.UTC().Format(time.RFC3339)
	if b, err := os.ReadFile(r.manifestPath()); err == nil {
		var old manifest
		if json.Unmarshal(b, &old) == nil && old.StartedAt != "" {
			startedAt = old.StartedAt
		}
	}
	rows, err := filepath.Rel(r.l.Dir, r.rowsPath())
	if err != nil {
		rows = r.rowsPath()
	}
	a := r.cfg.Agent
	meta := map[string]any{
		"agent":       a.Adapter,
		"cases":       len(r.l.Cases),
		"reps":        r.cfg.Reps,
		"concurrency": r.cfg.Concurrency,
		"harness":     r.harnessHash(),
		"variant":     r.variantHash(),
	}
	if a.Model != "" {
		meta["model"] = a.Model
	}
	if a.Effort != "" {
		meta["effort"] = a.Effort
	}
	if v := agent.Version(ctx, a.Adapter); v != "" {
		meta["agent_version"] = v
	}
	if out, err := exec.CommandContext(ctx, "git", "-C", r.l.Dir, "rev-parse", "--short", "HEAD").Output(); err == nil {
		meta["commit"] = strings.TrimSpace(string(out))
	}
	for k, v := range r.opts.Meta {
		meta[k] = v
	}
	return manifest{ID: r.opts.Run, Label: r.opts.Label, Note: r.opts.Note, Baseline: r.opts.Baseline,
		StartedAt: startedAt, ItemsFrom: filepath.ToSlash(rows), Dims: r.opts.Dims, Meta: meta}
}

func (r *runner) writeManifest(m manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.manifestPath(), append(b, '\n'), 0o644)
}

// harnessHash changes when what grades a run changes: the cases, the stage
// commands and scripts, the rubric. Scores under two harnesses do not compare.
func (r *runner) harnessHash() string {
	h := sha256.New()
	c := r.cfg
	fmt.Fprint(h, c.Prepare, "\x00", c.Collect, "\x00", strings.Join(c.Score, "\x00"))
	files := []string{c.Cases}
	if c.Judge != nil {
		files = append(files, c.Judge.Rubric)
		fmt.Fprint(h, c.Judge.Adapter, c.Judge.Model, c.Judge.Effort)
	}
	bin, _ := filepath.Glob(filepath.Join(r.l.Dir, "bin", "*"))
	for _, f := range bin {
		if rel, err := filepath.Rel(r.l.Dir, f); err == nil {
			files = append(files, rel)
		}
	}
	hashFiles(h, r.l.Dir, files)
	return hex.EncodeToString(h.Sum(nil)[:5])
}

// variantHash changes when the thing under test changes: the overlay, the
// system prompt, the prompt template, the climb targets.
func (r *runner) variantHash() string {
	h := sha256.New()
	c := r.cfg
	fmt.Fprint(h, c.Agent.Prompt)
	files := []string{c.Agent.System}
	for _, o := range c.Overlay {
		files = append(files, o.From)
	}
	if c.Climb != nil {
		files = append(files, c.Climb.Targets...)
	}
	hashFiles(h, r.l.Dir, files)
	return hex.EncodeToString(h.Sum(nil)[:5])
}

// hashFiles writes each named file, or every file under a named directory, to h.
func hashFiles(h io.Writer, dir string, names []string) {
	for _, name := range names {
		if name == "" {
			continue
		}
		filepath.WalkDir(filepath.Join(dir, name), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if b, err := os.ReadFile(path); err == nil {
				fmt.Fprint(h, path, "\x00")
				h.Write(b)
			}
			return nil
		})
	}
}

// attempt runs one (case, rep), retrying harness failures, and records a row or an error.
func (r *runner) attempt(ctx context.Context, j job) {
	key := attemptKey(j.c.ID(), j.rep)
	var res attemptResult
	for try := 0; ; try++ {
		res = r.once(ctx, j)
		r.mu.Lock()
		r.sum.CostUSD += res.cost
		r.mu.Unlock()
		if res.err == nil || ctx.Err() != nil {
			break
		}
		r.recordError(j, try, res)
		if try >= r.cfg.Retries || (res.class != agent.Harness && res.class != agent.Timeout) {
			break
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx.Err() != nil && res.err != nil {
		return // cancelled: neither a row nor an error; a resume makes it again
	}
	if res.err != nil {
		r.sum.Failed++
		r.sum.ByClass[res.class]++
		fmt.Fprintf(r.log, "  %-28s FAILED (%s): %v\n", key, res.class, res.err)
		return
	}
	if res.judgeErr != nil {
		r.sum.JudgeErrors++
	}
	if err := appendLine(r.rowsPath(), res.row); err != nil {
		r.sum.Failed++
		r.sum.ByClass[agent.Harness]++
		fmt.Fprintf(r.log, "  %-28s FAILED (harness): %v\n", key, err)
		return
	}
	r.sum.Done++
	fmt.Fprintf(r.log, "  %-28s ok  %5.1fs  $%.4f  (%d of %d)\n", key, res.seconds, res.cost, r.sum.Done+r.sum.Failed, r.sum.Planned)
}

func (r *runner) recordError(j job, try int, res attemptResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	appendLine(r.errorsPath(), map[string]any{
		"id": j.c.ID(), "rep": j.rep, "try": try, "class": res.class, "error": res.err.Error(),
		"cost_usd": round(res.cost, 4), "seconds": round(res.seconds, 1), "at": time.Now().UTC().Format(time.RFC3339),
	})
}

func appendLine(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func round(v float64, places int) float64 {
	p := 1.0
	for range places {
		p *= 10
	}
	return float64(int64(v*p+0.5)) / p
}
