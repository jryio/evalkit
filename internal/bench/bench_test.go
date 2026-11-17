//go:build unix

package bench_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jryio/evalkit/internal/bench"
	"github.com/jryio/evalkit/internal/stats"
	"github.com/jryio/evalkit/internal/suite"
)

const suiteHead = `title: Fixture benchmark
view: runs
runs_from: runs/*/run.json
questions:
  - id: verdict
    kind: choice
    primary: true
    options:
      - { value: pass, label: Pass, tone: positive }
      - { value: fail, label: Fail, tone: negative }
columns:
  - { id: cost_usd, label: Cost, unit: $, better: lower }
  - { id: latency_s, label: Latency, unit: s, better: lower }
`

// passIfFixed is a score stage: pass when the workspace's a.txt says fixed.
const passIfFixed = `v=fail; grep -q fixed a.txt 2>/dev/null && v=pass; printf '{"proposed":{"verdict":{"value":"%s","why":"a.txt was checked."}}}' "$v"`

func write(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		tb.Fatal(err)
	}
}

func git(tb testing.TB, dir string, args ...string) {
	tb.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// fixture writes a benchmark whose workspace is a one-file git repository and
// returns the suite's directory. benchYAML is the bench block's body, indented
// two spaces; ids are the case ids.
func fixture(tb testing.TB, benchYAML string, ids ...string) string {
	tb.Helper()
	dir := tb.TempDir()
	repo := filepath.Join(dir, "repo")
	write(tb, filepath.Join(repo, "a.txt"), "broken\n")
	git(tb, repo, "init", "-q", "-b", "main")
	git(tb, repo, "add", "-A")
	git(tb, repo, "commit", "-q", "-m", "one")
	var cases strings.Builder
	for i, id := range ids {
		group := "train"
		if i%2 == 1 {
			group = "test"
		}
		fmt.Fprintf(&cases, `{"id":%q,"group":%q,"prompt":"fix %s","expect":"SECRET-ANSWER"}`+"\n", id, group, id)
	}
	write(tb, filepath.Join(dir, "cases.jsonl"), cases.String())
	write(tb, filepath.Join(dir, "suite.yaml"), suiteHead+"groups: [{ id: train, label: Train }, { id: test, label: Held-out }]\nbench:\n  cases: cases.jsonl\n  workspace: { sources: [{ path: repo }] }\n"+benchYAML)
	return dir
}

func load(tb testing.TB, dir string) *bench.Loaded {
	tb.Helper()
	l, err := bench.Load(filepath.Join(dir, "suite.yaml"))
	if err != nil {
		tb.Fatalf("bench.Load: %v", err)
	}
	return l
}

func run(tb testing.TB, dir string, opts bench.Options) bench.Summary {
	tb.Helper()
	opts.SlotsRoot = filepath.Join(tb.TempDir(), "slots")
	sum, err := bench.Run(tb.Context(), load(tb, dir), opts)
	if err != nil {
		tb.Fatalf("bench.Run: %v", err)
	}
	if sum.ByClass["harness"] > 0 {
		tb.Logf("run %s had harness failures: %v", opts.Run, errorLines(tb, dir, opts.Run))
	}
	return sum
}

func rows(tb testing.TB, dir, runID string) []map[string]any {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "runs", runID, "rows.jsonl"))
	if err != nil {
		tb.Fatal(err)
	}
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			tb.Fatalf("row %q: %v", line, err)
		}
		out = append(out, row)
	}
	return out
}

func errorLines(tb testing.TB, dir, runID string) []map[string]any {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "runs", runID, "errors.jsonl"))
	if err != nil {
		return nil
	}
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		var e map[string]any
		json.Unmarshal([]byte(line), &e)
		out = append(out, e)
	}
	return out
}

func verdict(row map[string]any) string {
	p, _ := row["proposed"].(map[string]any)
	v, _ := p["verdict"].(map[string]any)
	s, _ := v["value"].(string)
	return s
}

// report loads the suite the way the page does and scores its runs.
func report(tb testing.TB, dir string) stats.Report {
	tb.Helper()
	res, err := suite.Load(filepath.Join(dir, "suite.yaml"))
	if err != nil {
		tb.Fatal(err)
	}
	for _, p := range res.Problems {
		tb.Errorf("the suite with its runs has a problem: %s", p)
	}
	s, err := stats.Decode(res.Doc)
	if err != nil {
		tb.Fatal(err)
	}
	return stats.Summarize(s, nil, 1)
}

func TestRun_EndToEndAndResume(t *testing.T) {
	t.Parallel()
	// The agent fixes a.txt for every case but c.
	dir := fixture(t, `  concurrency: 3
  agent: { adapter: command, command: 'if [ "$EVALKIT_CASE" != c ]; then echo fixed > a.txt; fi; echo done' }
  score: ['`+strings.ReplaceAll(passIfFixed, "'", "''")+`']
`, "a", "b", "c", "d")

	sum := run(t, dir, bench.Options{Run: "base", Baseline: true, Dims: map[string]any{"round": 0}})
	if sum.Done != 4 || sum.Failed != 0 || sum.Planned != 4 {
		t.Fatalf("summary = %+v, want 4 rows", sum)
	}
	got := map[string]string{}
	for _, row := range rows(t, dir, "base") {
		got[row["id"].(string)] = verdict(row)
		show, _ := row["show"].([]any)
		if len(show) == 0 {
			t.Errorf("row %v has nothing to show", row["id"])
		}
		values, _ := row["values"].(map[string]any)
		if _, ok := values["latency_s"]; !ok {
			t.Errorf("row %v lacks the declared latency_s column", row["id"])
		}
	}
	want := map[string]string{"a": "pass", "b": "pass", "c": "fail", "d": "pass"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("verdicts = %v, want %v", got, want)
	}

	rep := report(t, dir)
	if len(rep.Runs) != 1 || rep.Runs[0].Run != "base" || !rep.Runs[0].Baseline || rep.Runs[0].Score == nil || rep.Runs[0].Score.Value != 0.75 {
		t.Errorf("report = %+v, want run base scoring 0.75", rep.Runs)
	}

	again := run(t, dir, bench.Options{Run: "base"})
	if again.Planned != 0 || again.Resumed != 4 || again.Done != 0 {
		t.Errorf("second run = %+v, want everything resumed", again)
	}
	if n := len(rows(t, dir, "base")); n != 4 {
		t.Errorf("%d rows after a resume, want 4", n)
	}
}

func TestRun_RepsAndASecondRunPair(t *testing.T) {
	t.Parallel()
	dir := fixture(t, `  reps: 2
  agent: { adapter: command, command: 'if [ "$EVALKIT_RUN" = v2 ]; then echo fixed > a.txt; fi; echo done' }
  score: ['`+strings.ReplaceAll(passIfFixed, "'", "''")+`']
`, "a", "b")
	run(t, dir, bench.Options{Run: "v1", Baseline: true})
	run(t, dir, bench.Options{Run: "v2"})
	rep := report(t, dir)
	if len(rep.Runs) != 2 {
		t.Fatalf("%d runs, want 2", len(rep.Runs))
	}
	v := rep.Runs[1].Versus
	if rep.Runs[1].Run != "v2" || v == nil || v.Run != "v1" || v.Paired != 4 || v.Up != 4 {
		t.Errorf("v2 versus = %+v, want all 4 attempts paired with v1 and improved", v)
	}
}

func TestRun_AttemptsAreIsolated(t *testing.T) {
	// Not parallel: it sets a variable the attempts must not inherit.
	t.Setenv("EVALKIT_TEST_SECRET", "hunter2")
	// Every agent reports what it can see, then leaves litter everywhere it can.
	agent := `seen=clean
[ -e ../litter ] && seen=slot-litter
[ -e litter ] && seen=repo-litter
[ "$(cat a.txt)" != broken ] && seen=dirty-repo
[ -n "$EVALKIT_TEST_SECRET" ] && seen=leaked-env
[ "$(git config --global user.name)" != evalkit ] && seen=the-users-git-config
grep -q SECRET-ANSWER && seen=saw-the-answer
case "$TMPDIR" in "$EVALKIT_OUT"/*) ;; *) seen=shared-tmp ;; esac
case "$HOME" in "$EVALKIT_OUT"/*) ;; *) seen=shared-home ;; esac
echo "$EVALKIT_CASE" > litter; echo x > ../litter; echo changed > a.txt; echo x > "$TMPDIR/t"
sleep 0.2
[ "$(cat litter)" != "$EVALKIT_CASE" ] && seen=another-attempt-wrote-here
echo "$seen"`
	write(t, filepath.Join(t.TempDir(), "x"), "")
	dir := fixture(t, "  concurrency: 4\n  reps: 2\n  home: private\n  agent: { adapter: command, command: agent.sh }\n", "a", "b", "c", "d", "e", "f")
	write(t, filepath.Join(dir, "bin", "agent.sh"), "#!/bin/sh\n"+agent+"\n")

	sum := run(t, dir, bench.Options{Run: "iso"})
	if sum.Done != 12 {
		t.Fatalf("summary = %+v, want 12 rows; errors: %v", sum, errorLines(t, dir, "iso"))
	}
	for _, row := range rows(t, dir, "iso") {
		show := row["show"].([]any)
		answer := strings.TrimSpace(show[0].(map[string]any)["text"].(string))
		if answer != "clean" {
			t.Errorf("attempt %v rep %v saw %q, want a clean, private workspace", row["id"], row["rep"], answer)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "repo", "a.txt")); string(b) != "broken\n" {
		t.Errorf("the source repository changed: a.txt = %q", b)
	}
}

func TestRun_FailuresStayOutOfTheScore(t *testing.T) {
	t.Parallel()
	agent := `case "$EVALKIT_CASE" in
crash) echo boom >&2; exit 2 ;;
hang) sleep 30 ;;
esac
echo fixed > a.txt; echo done`
	dir := fixture(t, "  timeout: 1\n  retries: 1\n  agent: { adapter: command, command: agent.sh }\n  score: [score.sh]\n", "ok", "crash", "hang", "badstage")
	write(t, filepath.Join(dir, "bin", "agent.sh"), "#!/bin/sh\n"+agent+"\n")
	write(t, filepath.Join(dir, "bin", "score.sh"), "#!/bin/sh\nif [ \"$EVALKIT_CASE\" = badstage ]; then echo not json; exit 0; fi\n"+passIfFixed+"\n")

	l := load(t, dir)
	sum, err := bench.Run(t.Context(), l, bench.Options{Run: "f", SlotsRoot: filepath.Join(t.TempDir(), "slots")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Done != 1 || sum.Failed != 3 {
		t.Fatalf("summary = %+v, want 1 row and 3 failures", sum)
	}
	wantClass := map[string]int{"agent_error": 1, "timeout": 1, "harness": 1}
	if fmt.Sprint(sum.ByClass) != fmt.Sprint(wantClass) {
		t.Errorf("classes = %v, want %v", sum.ByClass, wantClass)
	}
	if got := rows(t, dir, "f"); len(got) != 1 || got[0]["id"] != "ok" {
		t.Errorf("rows = %v, want only the attempt that finished", got)
	}
	tries := map[string]int{}
	for _, e := range errorLines(t, dir, "f") {
		tries[e["id"].(string)]++
	}
	// A crash is the agent's own failure and is not retried; a timeout and a broken stage are.
	if want := map[string]int{"crash": 1, "hang": 2, "badstage": 2}; fmt.Sprint(tries) != fmt.Sprint(want) {
		t.Errorf("tries per failed case = %v, want %v", tries, want)
	}
	if rep := report(t, dir); rep.Runs[0].N != 1 || rep.Runs[0].Score.Value != 1 {
		t.Errorf("score = %+v over %d rows, want 1.0 over the one finished attempt", rep.Runs[0].Score, rep.Runs[0].N)
	}

	// A later run of the same id retries only what has no row.
	again, _ := bench.Run(t.Context(), l, bench.Options{Run: "f", SlotsRoot: filepath.Join(t.TempDir(), "slots")})
	if again.Resumed != 1 || again.Planned != 3 {
		t.Errorf("resume = %+v, want 1 resumed and 3 retried", again)
	}
}

func TestRun_BudgetStopsNewAttempts(t *testing.T) {
	t.Parallel()
	dir := fixture(t, `  concurrency: 1
  budget_usd: 2
  agent: { adapter: command, command: 'echo ''{"output":"done","cost_usd":1}''' }
`, "a", "b", "c", "d", "e")
	sum := run(t, dir, bench.Options{Run: "b"})
	if sum.Done != 2 || sum.NotRun != 3 || !sum.StoppedAtBudget || sum.CostUSD != 2 {
		t.Errorf("summary = %+v, want 2 rows, 3 not run, $2 spent", sum)
	}
	if got := rows(t, dir, "b")[0]["values"].(map[string]any)["cost_usd"]; got != 1.0 {
		t.Errorf("cost_usd = %v, want the agent's reported 1", got)
	}
}

func TestRun_OverlayReachesTheAgentAndStaysOutOfTheDiff(t *testing.T) {
	t.Parallel()
	dir := fixture(t, `  overlay: [{ from: variant/hint.md, to: repo/HINT.md }]
  agent: { adapter: command, command: 'cat HINT.md > a.txt; echo done', system: variant/hint.md }
  score: ['`+strings.ReplaceAll(passIfFixed, "'", "''")+`']
`, "a")
	write(t, filepath.Join(dir, "variant", "hint.md"), "fixed\n")
	run(t, dir, bench.Options{Run: "o"})
	row := rows(t, dir, "o")[0]
	if verdict(row) != "pass" {
		t.Fatalf("verdict = %q, want pass: the agent did not get the overlay", verdict(row))
	}
	var diff string
	for _, a := range row["show"].([]any) {
		if m := a.(map[string]any); m["kind"] == "diff" {
			diff = m["diff"].(string)
		}
	}
	if !strings.Contains(diff, "+fixed") || strings.Contains(diff, "HINT.md") {
		t.Errorf("diff should show the agent's edit and not the overlay file:\n%s", diff)
	}
}

func TestRun_JudgeProposesAndRegrades(t *testing.T) {
	t.Parallel()
	dir := fixture(t, "  agent: { adapter: command, command: 'echo an answer' }\n  judge: { adapter: command, command: judge.sh, rubric: rubric.md }\n", "good", "judgefail")
	write(t, filepath.Join(dir, "rubric.md"), "Pass when the answer is an answer.\n")
	judge := func(v string) string {
		return "#!/bin/sh\nif [ \"$EVALKIT_CASE\" = judgefail ] && [ ! -e \"$EVALKIT_SUITE_DIR/fixed\" ]; then echo I cannot; exit 0; fi\necho '{\"verdict\":{\"value\":\"" + v + "\",\"why\":\"Because the rubric says so.\"}}'\n"
	}
	write(t, filepath.Join(dir, "bin", "judge.sh"), judge("fail"))

	sum := run(t, dir, bench.Options{Run: "j"})
	if sum.Done != 2 || sum.JudgeErrors != 1 {
		t.Fatalf("summary = %+v, want 2 rows, one without a verdict", sum)
	}
	byID := map[string]map[string]any{}
	for _, row := range rows(t, dir, "j") {
		byID[row["id"].(string)] = row
	}
	if verdict(byID["good"]) != "fail" {
		t.Errorf("judge verdict = %q, want fail", verdict(byID["good"]))
	}
	if flag, _ := byID["judgefail"]["flag"].(string); !strings.Contains(flag, "judge failed") {
		t.Errorf("a row the judge could not grade should be flagged, got %q", flag)
	}

	// The rubric changed: grade the stored attempts again without the agent.
	write(t, filepath.Join(dir, "bin", "judge.sh"), judge("pass"))
	write(t, filepath.Join(dir, "fixed"), "")
	n, _, err := bench.Regrade(t.Context(), load(t, dir), bench.RegradeOptions{Run: "j", Judge: true})
	if err != nil || n != 2 {
		t.Fatalf("Regrade = %d rows, %v; want 2", n, err)
	}
	rep := report(t, dir)
	if rep.Runs[0].N != 2 || rep.Runs[0].Score.Value != 1 {
		t.Errorf("after regrading, score = %+v over %d, want 1.0 over 2", rep.Runs[0].Score, rep.Runs[0].N)
	}
}

func manifestDims(tb testing.TB, dir, runID string) map[string]any {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "runs", runID, "run.json"))
	if err != nil {
		tb.Fatal(err)
	}
	var m struct {
		Dims map[string]any `json:"dims"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		tb.Fatal(err)
	}
	return m.Dims
}

// A run fills the dimensions the suite declares from what it actually ran.
func TestRun_FillsDeclaredDimensions(t *testing.T) {
	// Not parallel: it sets an environment variable.
	dir := fixture(t, "  reps: 2\n  agent: { adapter: command, command: 'echo done', effort: low }\n", "a")
	suitePath := filepath.Join(dir, "suite.yaml")
	b, err := os.ReadFile(suitePath)
	if err != nil {
		t.Fatal(err)
	}
	write(t, suitePath, strings.Replace(string(b), "bench:", "dimensions: [{ id: agent }, { id: effort }, { id: model }, { id: isolate }, { id: reps }, { id: round }]\nbench:", 1))

	run(t, dir, bench.Options{Run: "auto"})
	// model is empty and round is not something a run knows: both stay unset.
	want := map[string]any{"agent": "command", "effort": "low", "isolate": "agent", "reps": 2.0}
	if got := manifestDims(t, dir, "auto"); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("dims = %v, want %v", got, want)
	}

	// A declared dimension named after a variable the suite passes through takes its value.
	t.Setenv("FLAVOUR", "vanilla")
	b, _ = os.ReadFile(suitePath)
	write(t, suitePath, strings.Replace(strings.Replace(string(b), "{ id: round }", "{ id: round }, { id: flavour }", 1), "  reps: 2\n", "  reps: 2\n  env: [FLAVOUR]\n", 1))
	run(t, dir, bench.Options{Run: "env"})
	if got := manifestDims(t, dir, "env")["flavour"]; got != "vanilla" {
		t.Errorf("dims[flavour] = %v, want the passed-through variable's value", got)
	}

	run(t, dir, bench.Options{Run: "given", Effort: "high", Dims: map[string]any{"agent": "mine", "round": 3}})
	want = map[string]any{"agent": "mine", "effort": "high", "flavour": "vanilla", "isolate": "agent", "reps": 2.0, "round": 3.0}
	if got := manifestDims(t, dir, "given"); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("dims = %v, want the caller's values kept and the rest filled: %v", got, want)
	}
	report(t, dir)
}

// require_skills is about the real agent: evalkit check's stand-ins, which are
// command agents, must still be able to run the suite.
func TestRun_RequiredSkillsDoNotApplyToAStandIn(t *testing.T) {
	t.Parallel()
	dir := fixture(t, "  agent: { adapter: command, command: 'echo done', require_skills: [changelog] }\n", "a")
	if sum := run(t, dir, bench.Options{Run: "s"}); sum.Done != 1 {
		t.Errorf("summary = %+v, want the command agent's attempt to count; errors: %v", sum, errorLines(t, dir, "s"))
	}
}

// A command agent reads the case on stdin, so its cases need no prompt field.
func TestRun_CommandAgentNeedsNoPrompt(t *testing.T) {
	t.Parallel()
	dir := fixture(t, "  agent: { adapter: command, command: 'grep -o size...[0-9]*' }\n", "unused")
	write(t, filepath.Join(dir, "cases.jsonl"), `{"id":"a","size":42}`+"\n")
	if sum := run(t, dir, bench.Options{Run: "n"}); sum.Done != 1 {
		t.Fatalf("summary = %+v, want 1 row; errors: %v", sum, errorLines(t, dir, "n"))
	}
	answer := rows(t, dir, "n")[0]["show"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(answer, "42") {
		t.Errorf("answer = %q, want the agent to have read the case from stdin", answer)
	}
}

func TestRun_CasePinsTheWorkspace(t *testing.T) {
	t.Parallel()
	dir := fixture(t, "  agent: { adapter: command, command: 'cat a.txt' }\n", "x")
	repo := filepath.Join(dir, "repo")
	write(t, filepath.Join(repo, "a.txt"), "second\n")
	git(t, repo, "commit", "-q", "-am", "two")
	write(t, filepath.Join(dir, "cases.jsonl"), `{"id":"old","prompt":"p","pin":{"repo":"HEAD~1"}}`+"\n"+`{"id":"new","prompt":"p"}`+"\n")
	run(t, dir, bench.Options{Run: "p", Concurrency: 1})
	got := map[string]string{}
	for _, row := range rows(t, dir, "p") {
		got[row["id"].(string)] = strings.TrimSpace(row["show"].([]any)[0].(map[string]any)["text"].(string))
	}
	if want := map[string]string{"old": "broken", "new": "second"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("a.txt per case = %v, want %v", got, want)
	}
}

func TestRun_SandboxConfinesTheAgent(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" {
		t.Skip("bench.sandbox is the macOS sandbox")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, ".evalkit-sandbox-test-"+filepath.Base(t.TempDir()))
	t.Cleanup(func() { os.Remove(outside) })
	agent := `r=""
echo x > inside.txt 2>/dev/null && r="$r wrote-workspace"
echo x > "$TMPDIR/scratch" 2>/dev/null && r="$r wrote-tmp"
echo x > "` + outside + `" 2>/dev/null && r="$r WROTE-IN-HOME"
echo x > "$EVALKIT_SUITE_DIR/planted.txt" 2>/dev/null && r="$r WROTE-IN-THE-SUITE"
echo x > "$EVALKIT_WORKSPACE/../planted.txt" 2>/dev/null && r="$r WROTE-BESIDE-THE-SLOTS"
cat "$EVALKIT_SUITE_DIR/cases.jsonl" >/dev/null 2>&1 && r="$r READ-THE-CASES"
ls "$EVALKIT_WORKSPACE/.." >/dev/null 2>&1 && r="$r LISTED-OTHER-SLOTS"
ls "$EVALKIT_OUT/.." >/dev/null 2>&1 && r="$r LISTED-OTHER-ATTEMPTS"
echo "$r"`
	// The stand-in agent lives outside the suite: a sandboxed agent cannot read the suite.
	script := filepath.Join(t.TempDir(), "agent.sh")
	write(t, script, "#!/bin/sh\n"+agent+"\n")
	dir := fixture(t, "  sandbox: true\n  agent: { adapter: command, command: "+script+" }\n", "a")
	sum := run(t, dir, bench.Options{Run: "s"})
	if sum.Done != 1 {
		t.Fatalf("summary = %+v; errors: %v", sum, errorLines(t, dir, "s"))
	}
	got := strings.Fields(rows(t, dir, "s")[0]["show"].([]any)[0].(map[string]any)["text"].(string))
	if !slices.Equal(got, []string{"wrote-workspace", "wrote-tmp"}) {
		t.Errorf("sandboxed agent could: %v, want only its workspace and its temp directory", got)
	}
}

func TestScore_OracleAndNull(t *testing.T) {
	t.Parallel()
	dir := fixture(t, "  agent: { adapter: command, command: 'echo done' }\n  score: ['"+strings.ReplaceAll(passIfFixed, "'", "''")+"']\n", "a", "b")
	l := load(t, dir)
	for _, tc := range []struct {
		name, command string
		want          float64
	}{{"oracle", "echo fixed > a.txt", 1}, {"null", "true", 0}} {
		stage := l.Config.Agent
		stage.Command = tc.command
		runs := t.TempDir()
		sum, err := bench.Run(t.Context(), l, bench.Options{Run: tc.name, RunsDir: runs, SlotsRoot: filepath.Join(t.TempDir(), "slots"), Agent: &stage})
		if err != nil || sum.Done != 2 {
			failures, _ := os.ReadFile(filepath.Join(sum.Dir, "errors.jsonl"))
			t.Fatalf("%s: summary = %+v, %v\n%s", tc.name, sum, err, failures)
		}
		got, err := bench.Score(l, filepath.Join(sum.Dir, "rows.jsonl"))
		if err != nil || got.Value != tc.want || got.N != 2 {
			t.Errorf("%s scored %+v, %v; want %v over 2", tc.name, got, err, tc.want)
		}
		if _, err := os.Stat(filepath.Join(dir, "runs", tc.name)); err == nil {
			t.Errorf("the %s probe left a run in the suite", tc.name)
		}
	}
}

func TestLoad_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		suite string
		cases string
		want  string
	}{
		{"no bench block", suiteHead + "items: []\n", "", "no bench block"},
		{"a case without an id", suiteHead + "bench: { cases: cases.jsonl, agent: { adapter: command, command: 'true' } }\n", `{"prompt":"p"}`, "needs an id"},
		{"a case id twice", suiteHead + "bench: { cases: cases.jsonl, agent: { adapter: command, command: 'true' } }\n", `{"id":"a"}` + "\n" + `{"id":"a"}`, "appears twice"},
		{"an unknown bench key", suiteHead + "bench: { cases: cases.jsonl, agent: { adapter: command }, speed: fast }\n", `{"id":"a"}`, "has errors"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			write(t, filepath.Join(dir, "suite.yaml"), tc.suite)
			write(t, filepath.Join(dir, "cases.jsonl"), tc.cases+"\n")
			_, err := bench.Load(filepath.Join(dir, "suite.yaml"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Load error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
