package suite_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/jryio/evalkit/internal/suite"
)

func TestLoad_RunsFrom(t *testing.T) {
	t.Parallel()
	const suiteYAML = "title: Bench\nruns_from: runs/*/run.json\n"
	manifest := func(id, started string) string {
		return `{"id":"` + id + `","started_at":"` + started + `","items_from":"runs/` + id + `/rows.jsonl","dims":{"round":1},"meta":{"commit":"abc"}}`
	}
	tests := []struct {
		name     string
		files    map[string]string
		wantRuns []string
		wantIDs  []string
		wantWarn bool
	}{
		{
			name: "manifests become runs in started order",
			files: map[string]string{
				"suite.yaml":        suiteYAML,
				"runs/b/run.json":   manifest("b", "2026-11-01T00:00:00Z"),
				"runs/b/rows.jsonl": jsonl("x", "X"),
				"runs/a/run.json":   manifest("a", "2026-11-02T00:00:00Z"),
				"runs/a/rows.jsonl": jsonl("x", "X"),
			},
			wantRuns: []string{"b", "a"},
			wantIDs:  []string{"b/x", "a/x"},
		},
		{
			name: "declared runs come first",
			files: map[string]string{
				"suite.yaml":        "title: Bench\nruns: [{ id: base, items_from: base.jsonl }]\nruns_from: runs/*/run.json\n",
				"base.jsonl":        jsonl("x", "X"),
				"runs/a/run.json":   manifest("a", "2026-11-02T00:00:00Z"),
				"runs/a/rows.jsonl": jsonl("x", "X"),
			},
			wantRuns: []string{"base", "a"},
			wantIDs:  []string{"base/x", "a/x"},
		},
		{
			name:     "no manifest yet is a warning",
			files:    map[string]string{"suite.yaml": suiteYAML},
			wantWarn: true,
		},
		{
			name: "a manifest that is not an object is skipped",
			files: map[string]string{
				"suite.yaml":        suiteYAML,
				"runs/a/run.json":   "[1]",
				"runs/b/run.json":   manifest("b", "2026-11-01T00:00:00Z"),
				"runs/b/rows.jsonl": jsonl("x", "X"),
			},
			wantRuns: []string{"b"},
			wantIDs:  []string{"b/x"},
			wantWarn: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := load(t, writeDir(t, tc.files))
			if res.Fatal {
				t.Fatalf("Load is fatal: %v", res.Problems)
			}
			if got := docRuns(t, res); !slices.Equal(got, tc.wantRuns) {
				t.Errorf("runs = %v, want %v", got, tc.wantRuns)
			}
			var ids []string
			for _, it := range docItems(t, res) {
				ids = append(ids, it["id"].(string))
			}
			if !slices.Equal(ids, tc.wantIDs) {
				t.Errorf("item ids = %v, want %v", ids, tc.wantIDs)
			}
			warned := slices.ContainsFunc(res.Problems, func(p suite.Problem) bool { return p.Warning })
			if warned != tc.wantWarn {
				t.Errorf("warned = %v, want %v: %v", warned, tc.wantWarn, res.Problems)
			}
		})
	}
}

func TestVersion_ChangesWithRunManifest(t *testing.T) {
	t.Parallel()
	path := writeDir(t, map[string]string{
		"suite.yaml":      "title: Bench\nruns_from: runs/*/run.json\n",
		"runs/a/run.json": `{"id":"a"}`,
	})
	before := version(t, path)
	writeFile(t, filepath.Join(filepath.Dir(path), "runs", "a", "run.json"), `{"id":"a","note":"done"}`)
	if after := version(t, path); after == before {
		t.Errorf("version = %q after a run manifest changed, want a change", after)
	}
}
