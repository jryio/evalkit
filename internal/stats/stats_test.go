package stats

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// The expected numbers in this file come from the page's own code (web/kit/core/stats.js).
func TestWilson(t *testing.T) {
	t.Parallel()
	iv, ok := Wilson(8, 10)
	if !ok || iv.Value != 0.8 || math.Abs(iv.Lo-0.4902) > 5e-5 || math.Abs(iv.Hi-0.9433) > 5e-5 {
		t.Errorf("Wilson(8, 10) = %+v, want 0.8 in [0.4902, 0.9433]", iv)
	}
	if iv, _ := Wilson(0, 10); !near(iv.Lo, 0) {
		t.Errorf("Wilson(0, 10).Lo = %v, want 0", iv.Lo)
	}
	if iv, _ := Wilson(10, 10); !near(iv.Hi, 1) {
		t.Errorf("Wilson(10, 10).Hi = %v, want 1", iv.Hi)
	}
	if _, ok := Wilson(0, 0); ok {
		t.Error("Wilson(0, 0) reported an interval")
	}
}

func TestRNG_MatchesThePage(t *testing.T) {
	t.Parallel()
	next := rng(7)
	for i, want := range []float64{0.011704753153026104, 0.06195825757458806, 0.97690763277933} {
		if got := next(); !near(got, want) {
			t.Errorf("draw %d of rng(7) = %v, want %v", i, got, want)
		}
	}
}

func TestBootstrapMean_MatchesThePage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		xs       []float64
		clusters []string
		seed     uint32
		want     Interval
	}{
		{"independent values", []float64{3, 5, 4, 8, 6, 2, 7, 5, 4, 6}, nil, 3, Interval{5, 3.9, 6.1}},
		{"reps of a case move together", []float64{1, 1, 0, 0, 1, 0}, []string{"a", "a", "b", "b", "c", "c"}, 1, Interval{0.5, 0, 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := BootstrapMean(tc.xs, tc.clusters, tc.seed)
			if !ok || !near(got.Value, tc.want.Value) || !near(got.Lo, tc.want.Lo) || !near(got.Hi, tc.want.Hi) {
				t.Errorf("BootstrapMean = %+v, want %+v", got, tc.want)
			}
		})
	}
	if _, ok := BootstrapMean(nil, nil, 1); ok {
		t.Error("BootstrapMean of nothing reported an interval")
	}
}

func TestMcNemar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wins, losses int
		want         float64
	}{{7, 1, 0.0703125}, {3, 3, 1}, {0, 0, 1}}
	for _, tc := range tests {
		if got := McNemar(tc.wins, tc.losses); !near(got, tc.want) {
			t.Errorf("McNemar(%d, %d) = %v, want %v", tc.wins, tc.losses, got, tc.want)
		}
	}
}

func readJSON(tb testing.TB, name string, v any) {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		tb.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		tb.Fatalf("%s: %v", name, err)
	}
}

// The page summarized the hill-climb demo (six runs, 12 cases, 2 reps) into
// hill-climb.want.json; Summarize must give the same numbers.
func TestSummarize_MatchesThePage(t *testing.T) {
	t.Parallel()
	var s Suite
	readJSON(t, "hill-climb.suite.json", &s)
	type wantRow struct {
		Run    string    `json:"run"`
		Items  int       `json:"items"`
		N      int       `json:"n"`
		Score  *Interval `json:"score"`
		Spend  *float64  `json:"spend"`
		Versus *struct {
			Run    string   `json:"run"`
			Paired int      `json:"paired"`
			N      int      `json:"n"`
			Delta  Interval `json:"delta"`
			Up     int      `json:"up"`
			Same   int      `json:"same"`
			Down   int      `json:"down"`
			P      *float64 `json:"p"`
		} `json:"versus"`
	}
	var want struct {
		Metric string    `json:"metric"`
		All    []wantRow `json:"all"`
		Test   []wantRow `json:"test"`
	}
	readJSON(t, "hill-climb.want.json", &want)

	tests := []struct {
		name   string
		groups []string
		want   []wantRow
	}{{"every item", nil, want.All}, {"held-out group only", []string{"test"}, want.Test}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rep := Summarize(s, tc.groups, 1)
			if rep.Metric == nil || rep.Metric.ID != want.Metric {
				t.Fatalf("metric = %+v, want %q", rep.Metric, want.Metric)
			}
			if len(rep.Runs) != len(tc.want) {
				t.Fatalf("%d runs, want %d", len(rep.Runs), len(tc.want))
			}
			for i, w := range tc.want {
				g := rep.Runs[i]
				if g.Run != w.Run || g.Items != w.Items || g.N != w.N {
					t.Errorf("run %d = %s items %d n %d, want %s items %d n %d", i, g.Run, g.Items, g.N, w.Run, w.Items, w.N)
				}
				if g.Score == nil || !near(g.Score.Value, w.Score.Value) || !near(g.Score.Lo, w.Score.Lo) || !near(g.Score.Hi, w.Score.Hi) {
					t.Errorf("%s score = %+v, want %+v", w.Run, g.Score, w.Score)
				}
				if (g.Spend == nil) != (w.Spend == nil) || (g.Spend != nil && !near(*g.Spend, *w.Spend)) {
					t.Errorf("%s spend = %v, want %v", w.Run, g.Spend, w.Spend)
				}
				if (g.Versus == nil) != (w.Versus == nil) {
					t.Errorf("%s versus = %+v, want %+v", w.Run, g.Versus, w.Versus)
					continue
				}
				if w.Versus == nil {
					continue
				}
				v := g.Versus
				if v.Run != w.Versus.Run || v.Paired != w.Versus.Paired || v.N != w.Versus.N || v.Up != w.Versus.Up || v.Same != w.Versus.Same || v.Down != w.Versus.Down {
					t.Errorf("%s versus = %+v, want %+v", w.Run, v, w.Versus)
				}
				if !near(v.Delta.Value, w.Versus.Delta.Value) || !near(v.Delta.Lo, w.Versus.Delta.Lo) || !near(v.Delta.Hi, w.Versus.Delta.Hi) {
					t.Errorf("%s delta = %+v, want %+v", w.Run, v.Delta, w.Versus.Delta)
				}
				if v.P == nil || !near(*v.P, *w.Versus.P) {
					t.Errorf("%s p = %v, want %v", w.Run, v.P, *w.Versus.P)
				}
				if len(v.Improved) != v.Up || len(v.Regressed) != v.Down {
					t.Errorf("%s lists %d improved and %d regressed, want %d and %d", w.Run, len(v.Improved), len(v.Regressed), v.Up, v.Down)
				}
			}
		})
	}
}

func TestMetricOf(t *testing.T) {
	t.Parallel()
	choice := Question{ID: "verdict", Kind: "choice", Primary: true}
	lo, hi := 0.0, 10.0
	tests := []struct {
		name       string
		suite      Suite
		wantID     string
		wantKind   string
		wantBetter string
		wantNone   bool
	}{
		{"primary choice question is a rate", Suite{Questions: []Question{choice}}, "verdict", "rate", "higher", false},
		{"primary number question is a mean", Suite{Questions: []Question{{ID: "q", Kind: "number", Primary: true}}}, "q", "mean", "higher", false},
		{"a primary column beats the primary question", Suite{Questions: []Question{choice}, Columns: []Column{{ID: "ms", Primary: true, Better: "lower"}}}, "ms", "mean", "lower", false},
		{"else the first bounded column", Suite{Columns: []Column{{ID: "cost", Better: "lower"}, {ID: "score", Min: &lo, Max: &hi}}}, "score", "mean", "higher", false},
		{"else the first directed column", Suite{Columns: []Column{{ID: "note"}, {ID: "cost", Better: "lower"}}}, "cost", "mean", "lower", false},
		{"nothing scores", Suite{Questions: []Question{{ID: "t", Kind: "text", Primary: true}}}, "", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, ok := MetricOf(tc.suite)
			if ok == tc.wantNone {
				t.Fatalf("MetricOf found = %v, want %v", ok, !tc.wantNone)
			}
			if ok && (m.ID != tc.wantID || m.Kind != tc.wantKind || m.Better != tc.wantBetter) {
				t.Errorf("MetricOf = %s %s %s, want %s %s %s", m.ID, m.Kind, m.Better, tc.wantID, tc.wantKind, tc.wantBetter)
			}
		})
	}
}

func TestSummarize_LowerIsBetterColumn(t *testing.T) {
	t.Parallel()
	item := func(run, id string, ms float64) Item {
		return Item{ID: run + "/" + id, Run: run, Pair: id, Values: map[string]any{"ms": ms}}
	}
	s := Suite{
		Columns: []Column{{ID: "ms", Primary: true, Better: "lower", Noise: 5}},
		Runs:    []Run{{ID: "a", Baseline: true}, {ID: "b"}},
		Items: []Item{
			item("a", "x", 100), item("a", "y", 100), item("a", "z", 100),
			item("b", "x", 60), item("b", "y", 103), item("b", "z", 140),
		},
	}
	v := Summarize(s, nil, 1).Runs[1].Versus
	if v == nil || v.Up != 1 || v.Same != 1 || v.Down != 1 {
		t.Fatalf("versus = %+v, want one faster, one within noise, one slower", v)
	}
	if v.Improved[0] != "x" || v.Regressed[0] != "z" {
		t.Errorf("improved %v regressed %v, want [x] and [z]", v.Improved, v.Regressed)
	}
	if v.P != nil {
		t.Errorf("p = %v for a mean, want none", *v.P)
	}
}
