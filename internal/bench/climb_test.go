//go:build unix

package bench_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jryio/evalkit/internal/bench"
)

// climbFixture is a benchmark where a case passes once the number in
// variant/level.txt (the climb target, overlaid into the workspace) reaches
// the case's level. propose is the proposer's shell command.
func climbFixture(tb testing.TB, startLevel, propose string) string {
	tb.Helper()
	dir := fixture(tb, `  overlay: [{ from: variant/level.txt, to: repo/LEVEL }]
  agent: { adapter: command, command: agent.sh }
  score: [score.sh]
  climb:
    targets: [variant/level.txt]
    train: [train]
    test: [test]
    target: 1
    propose: { adapter: command, command: propose.sh }
`, "unused")
	write(tb, filepath.Join(dir, "cases.jsonl"), `{"id":"t1","group":"train","prompt":"p","level":1}
{"id":"t2","group":"train","prompt":"p","level":2}
{"id":"t3","group":"train","prompt":"p","level":3}
{"id":"h1","group":"test","prompt":"p","level":2}
{"id":"h2","group":"test","prompt":"p","level":9}
`)
	write(tb, filepath.Join(dir, "variant", "level.txt"), startLevel+"\n")
	write(tb, filepath.Join(dir, "bin", "agent.sh"), `#!/bin/sh
need=$(sed -n 's/.*"level":\([0-9]*\).*/\1/p' | head -1)
[ "$(cat LEVEL)" -ge "$need" ] && echo fixed > a.txt
echo done
`)
	write(tb, filepath.Join(dir, "bin", "score.sh"), "#!/bin/sh\n"+passIfFixed+"\n")
	write(tb, filepath.Join(dir, "bin", "propose.sh"), "#!/bin/sh\n"+propose+"\n")
	return dir
}

func climb(tb testing.TB, dir string, rounds int) []bench.Round {
	tb.Helper()
	got, err := bench.Climb(tb.Context(), filepath.Join(dir, "suite.yaml"), bench.ClimbOptions{Rounds: rounds, SlotsRoot: filepath.Join(tb.TempDir(), "slots")})
	if err != nil {
		tb.Fatalf("Climb: %v", err)
	}
	return got
}

func level(tb testing.TB, dir string) string {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "variant", "level.txt"))
	if err != nil {
		tb.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestClimb_KeepsRoundsThatRaiseTheTrainScore(t *testing.T) {
	t.Parallel()
	dir := climbFixture(t, "0", `n=$(cat targets/variant/level.txt)
cp failures.json "$EVALKIT_SUITE_DIR/seen-$n.json"
echo $((n+1)) > targets/variant/level.txt
echo "Raised the level to $((n+1))."`)

	rounds := climb(t, dir, 5)
	// Train: 0/3, 1/3, 2/3, 3/3, then the target of 1 stops the climb.
	if len(rounds) != 4 {
		t.Fatalf("%d rounds, want the baseline and 3 rounds before the target stops it: %+v", len(rounds), rounds)
	}
	wantTest := []float64{0, 0, 0.5, 0.5}
	for i, r := range rounds {
		if !r.Kept {
			t.Errorf("round %d was not kept: %s", i, r.Reason)
		}
		if got, want := r.Train.Value, float64(i)/3; got < want-1e-9 || got > want+1e-9 {
			t.Errorf("round %d train = %v, want %v", i, got, want)
		}
		if r.Test == nil || r.Test.Value != wantTest[i] {
			t.Errorf("round %d held-out = %+v, want %v", i, r.Test, wantTest[i])
		}
	}
	if rounds[1].Change != "Raised the level to 1." {
		t.Errorf("round 1 change = %q, want the proposer's sentence", rounds[1].Change)
	}
	if got := level(t, dir); got != "3" {
		t.Errorf("level.txt = %s after the climb, want the best round's 3", got)
	}
	// The proposer saw the failing train cases and never a held-out one.
	seen, err := os.ReadFile(filepath.Join(dir, "seen-0.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(seen), `"t1"`) || strings.Contains(string(seen), `"h1"`) || strings.Contains(string(seen), `"h2"`) {
		t.Errorf("the proposer's failures should hold train cases only:\n%s", seen)
	}
	if patch, _ := os.ReadFile(filepath.Join(dir, "runs", "round1", "change.patch")); !strings.Contains(string(patch), "+1") {
		t.Errorf("round1/change.patch should hold the edit, got:\n%s", patch)
	}
	rep := report(t, dir)
	if len(rep.Runs) != 4 || !rep.Runs[0].Baseline {
		t.Errorf("the suite lists %d runs, want 4 with round0 the baseline", len(rep.Runs))
	}
}

func TestClimb_RevertsARoundThatDoesNotHelp(t *testing.T) {
	t.Parallel()
	dir := climbFixture(t, "2", `echo 0 > targets/variant/level.txt; echo "Lowered the level."`)
	rounds := climb(t, dir, 1)
	if len(rounds) != 2 || rounds[1].Kept {
		t.Fatalf("rounds = %+v, want a baseline and one reverted round", rounds)
	}
	if !strings.Contains(rounds[1].Reason, "reverted") {
		t.Errorf("reason = %q, want it to say the round was reverted", rounds[1].Reason)
	}
	if got := level(t, dir); got != "2" {
		t.Errorf("level.txt = %s, want the baseline's 2 back", got)
	}

	// A later climb continues from the same state and proposes on top of the baseline again.
	more := climb(t, dir, 1)
	if len(more) != 3 || more[2].Run != "round2" {
		t.Errorf("continued climb = %+v, want a third round named round2", more)
	}
}

func TestClimb_StopsWhenTheProposerChangesNothing(t *testing.T) {
	t.Parallel()
	dir := climbFixture(t, "1", `echo "Nothing to do."`)
	if rounds := climb(t, dir, 3); len(rounds) != 1 {
		t.Errorf("%d rounds, want only the baseline", len(rounds))
	}
}
