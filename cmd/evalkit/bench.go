package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jryio/evalkit/internal/bench"
	"github.com/jryio/evalkit/internal/workspace"
)

// dimFlag collects --dim key=value pairs.
type dimFlag map[string]any

func (d dimFlag) String() string { return fmt.Sprint(map[string]any(d)) }
func (d dimFlag) Set(s string) error {
	k, v, ok := strings.Cut(s, "=")
	if !ok || k == "" {
		return errors.New("want key=value")
	}
	if n, err := strconv.ParseFloat(v, 64); err == nil {
		d[k] = n
	} else {
		d[k] = v
	}
	return nil
}
func (d dimFlag) Get() any { return map[string]any(d) }

var benchCommands = []*command{
	{name: "run", args: "[SUITE]", summary: "run the suite's benchmark as one run", run: runRun,
		flags: func(fs *flag.FlagSet) {
			fs.String("run", "", "the run's id (default: r-<date>-<time>); an existing id is resumed")
			fs.String("label", "", "shown instead of the id")
			fs.String("note", "", "a one-line note on the run")
			fs.Bool("baseline", false, "mark this the run every other run is compared with")
			fs.Var(dimFlag{}, "dim", "a dimension of the run, key=value (repeatable)")
			fs.String("only", "", "run only these case ids (comma-separated)")
			fs.Int("limit", 0, "run only the first N cases")
			fs.Int("reps", 0, "attempts per case (default: bench.reps)")
			fs.Int("concurrency", 0, "attempts at once (default: bench.concurrency)")
			fs.String("slots", "", "where workspaces live (default: the user cache)")
		},
		help: `Runs every case of the suite's bench block through its agent, each attempt in
its own workspace slot, then through the collect, score and judge stages. Rows
go to runs/<run>/rows.jsonl as they finish, and runs/<run>/run.json registers
the run with the suite (runs_from: runs/*/run.json), so a served page fills in
live. Attempts that fail before producing an answer go to errors.jsonl, never
into the score. Running the same id again makes only what is missing.

  evalkit run --run baseline --baseline
  evalkit run --run v2 --dim round=2 --note "shorter prompt"
  evalkit run --run v2 --only auth,refund`},
	{name: "check", args: "[SUITE]", summary: "prove a benchmark is sound before paying for a run", run: runCheck,
		flags: func(fs *flag.FlagSet) {
			fs.String("slots", "", "where workspaces live (default: the user cache)")
		},
		help: `Makes one workspace slot and times it, estimates the disk every slot needs,
then runs the stand-in agents the suite names: bench.oracle must score every
case as a pass and bench.null as a fail. Nothing it runs is kept or billed.

  evalkit check`},
	{name: "grade", args: "[SUITE]", summary: "grade a finished run again without running the agent", run: runGrade,
		flags: func(fs *flag.FlagSet) {
			fs.String("run", "", "the run to grade again")
			fs.Bool("score", false, "run the score stages again")
			fs.Bool("judge", false, "run the judge again")
			fs.String("only", "", "only these case ids (comma-separated)")
		},
		help: `After a scorer or the judge's rubric changed, grades the stored attempts of a
run again. The agent does not run. Each regraded row replaces the earlier one.

  evalkit grade --run v2 --judge`},
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func loadBench(pos []string, name string) (*bench.Loaded, error) {
	path, pos, err := suiteArg(pos)
	if err != nil {
		return nil, err
	}
	if err := want(name, pos, 0); err != nil {
		return nil, err
	}
	return bench.Load(path)
}

func runRun(f *flag.FlagSet, pos []string) error {
	l, err := loadBench(pos, "run")
	if err != nil {
		return err
	}
	opts := bench.Options{
		Run: str(f, "run"), Label: str(f, "label"), Note: str(f, "note"), Baseline: boolean(f, "baseline"),
		Only: splitList(str(f, "only")), Limit: integer(f, "limit"), Reps: integer(f, "reps"),
		Concurrency: integer(f, "concurrency"), SlotsRoot: str(f, "slots"), Log: os.Stdout,
	}
	if dims := f.Lookup("dim").Value.(flag.Getter).Get().(map[string]any); len(dims) > 0 {
		opts.Dims = dims
	}
	if opts.Run == "" {
		opts.Run = "r-" + time.Now().Format("20060102-150405")
	}
	if !l.RunsFrom {
		fmt.Println("note: the suite has no runs_from, so the page will not list this run. Add: runs_from: runs/*/run.json")
	}
	ctx, stop := signalContext()
	defer stop()
	sum, err := bench.Run(ctx, l, opts)
	printSummaryLine(sum)
	if err != nil {
		return err
	}
	if rep, err := loadReport(l.Path, nil); err == nil && rep.Metric != nil {
		fmt.Println()
		writeReport(os.Stdout, rep)
	}
	if sum.Failed > 0 {
		return fmt.Errorf("%d attempt(s) have no row; see %s", sum.Failed, filepath.Join(sum.Dir, "errors.jsonl"))
	}
	return nil
}

func printSummaryLine(s bench.Summary) {
	fmt.Printf("run %s: %d rows written, %d failed", s.Run, s.Done, s.Failed)
	if s.Resumed > 0 {
		fmt.Printf(", %d already done", s.Resumed)
	}
	for class, n := range s.ByClass {
		fmt.Printf(" [%s: %d]", class, n)
	}
	if s.JudgeErrors > 0 {
		fmt.Printf(", %d without a judge verdict", s.JudgeErrors)
	}
	if s.StoppedAtBudget {
		fmt.Printf(", %d not run: the budget was spent", s.NotRun)
	}
	fmt.Printf(", $%.4f, %.0fs\n", s.CostUSD, s.Seconds)
}

func runGrade(f *flag.FlagSet, pos []string) error {
	l, err := loadBench(pos, "grade")
	if err != nil {
		return err
	}
	opts := bench.RegradeOptions{Run: str(f, "run"), Score: boolean(f, "score"), Judge: boolean(f, "judge"), Only: splitList(str(f, "only")), Log: os.Stdout}
	if opts.Run == "" {
		return errors.New("grade: name the run with --run")
	}
	if !opts.Score && !opts.Judge {
		return errors.New("grade: pass --score, --judge or both")
	}
	ctx, stop := signalContext()
	defer stop()
	rows, cost, err := bench.Regrade(ctx, l, opts)
	fmt.Printf("run %s: %d rows regraded, $%.4f\n", opts.Run, rows, cost)
	return err
}

func runCheck(f *flag.FlagSet, pos []string) error {
	l, err := loadBench(pos, "check")
	if err != nil {
		return err
	}
	ctx, stop := signalContext()
	defer stop()
	cfg := l.Config
	fmt.Printf("%d cases × %d rep(s), %d at a time, agent %s\n", len(l.Cases), cfg.Reps, cfg.Concurrency, cfg.Agent.Adapter)

	slots := str(f, "slots")
	if slots == "" {
		if slots, err = bench.SlotsRoot(l.Dir); err != nil {
			return err
		}
	}
	pool, err := workspace.NewPool(ctx, cfg.Workspace, l.Dir, slots, 1)
	if err != nil {
		return err
	}
	est, err := pool.Probe(ctx)
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	need := est.SlotBytes * int64(cfg.Concurrency)
	fmt.Printf("workspace: one slot takes %s to make and %s to reset; %d slots need about %s",
		est.Provision.Round(time.Millisecond), est.Reset.Round(time.Millisecond), cfg.Concurrency, bytesText(need))
	if est.FreeBytes >= 0 {
		fmt.Printf(" of %s free", bytesText(est.FreeBytes))
	}
	fmt.Println()
	if est.FreeBytes >= 0 && need > est.FreeBytes {
		return errors.New("the slots will not fit on the disk: lower bench.concurrency or narrow the workspace")
	}

	failed := false
	for _, probe := range []struct {
		name, command string
		wantPass      bool
	}{{"oracle", cfg.Oracle, true}, {"null", cfg.Null, false}} {
		if probe.command == "" {
			fmt.Printf("%s: not set (bench.%s), so the grader is unproven on a known %s\n", probe.name, probe.name, map[bool]string{true: "pass", false: "fail"}[probe.wantPass])
			continue
		}
		score, n, err := probeScore(ctx, l, slots, probe.name, probe.command)
		if err != nil {
			return fmt.Errorf("%s: %w", probe.name, err)
		}
		ok := (probe.wantPass && score >= 0.999) || (!probe.wantPass && score <= 0.001)
		verdict := "ok"
		if !ok {
			verdict, failed = "WRONG", true
		}
		fmt.Printf("%s: scored %.3f over %d cases, want %s: %s\n", probe.name, score, n, map[bool]string{true: "1", false: "0"}[probe.wantPass], verdict)
	}
	if failed {
		return errors.New("the grader does not tell a right answer from a wrong one; fix the score stages before running")
	}
	return nil
}

// probeScore runs a stand-in agent over every case once, in a throwaway runs
// directory, and returns the score the graders gave it.
func probeScore(ctx context.Context, l *bench.Loaded, slots, name, command string) (score float64, n int, err error) {
	tmp, err := os.MkdirTemp("", "evalkit-check-*")
	if err != nil {
		return 0, 0, err
	}
	defer os.RemoveAll(tmp)
	stage := l.Config.Agent
	stage.Adapter, stage.Command = "command", command
	sum, err := bench.Run(ctx, l, bench.Options{Run: name, RunsDir: tmp, SlotsRoot: slots, Reps: 1, Agent: &stage, NoJudge: true})
	if err != nil {
		return 0, 0, err
	}
	if sum.Failed > 0 {
		return 0, 0, fmt.Errorf("%d attempt(s) failed; the first error is in the run's errors file: %s", sum.Failed, firstError(filepath.Join(sum.Dir, "errors.jsonl")))
	}
	rep, err := bench.Score(l, filepath.Join(sum.Dir, "rows.jsonl"))
	if err != nil {
		return 0, 0, err
	}
	return rep.Value, rep.N, nil
}

func firstError(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return line
}

func bytesText(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
