package suite_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jryio/evalkit/internal/suite"
)

var revisionRe = regexp.MustCompile(`^[0-9a-f]{12}$`)

func load(tb testing.TB, path string) suite.Result {
	tb.Helper()
	res, err := suite.Load(path)
	if err != nil {
		tb.Fatalf("Load(%q): %v", path, err)
	}
	return res
}

func docItems(tb testing.TB, res suite.Result) []map[string]any {
	tb.Helper()
	b, err := json.Marshal(res.Doc)
	if err != nil {
		tb.Fatalf("marshal Doc: %v", err)
	}
	var doc struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		tb.Fatalf("unmarshal Doc: %v", err)
	}
	return doc.Items
}

func revisions(tb testing.TB, res suite.Result) map[string]string {
	tb.Helper()
	out := map[string]string{}
	for _, it := range docItems(tb, res) {
		out[it["id"].(string)], _ = it["revision"].(string)
	}
	return out
}

func writeSuite(tb testing.TB, content string) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "suite.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestLoad_ValidSuites(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
	}{
		{"kitchen sink", filepath.Join("..", "..", "examples", "kitchen-sink", "suite.yaml")},
		{"status updates demo", filepath.Join("..", "..", "demos", "01-status-updates", "suite.yaml")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := load(t, tc.path)
			for _, p := range res.Problems {
				if !p.Warning {
					t.Errorf("Load(%q) problem: %s", tc.path, p)
				}
			}
			if res.Fatal {
				t.Fatalf("Load(%q).Fatal = true", tc.path)
			}
			first := revisions(t, res)
			if len(first) == 0 {
				t.Fatalf("Load(%q) returned no items", tc.path)
			}
			for id, rev := range first {
				if !revisionRe.MatchString(rev) {
					t.Errorf("item %q revision = %q, want 12 hex digits", id, rev)
				}
			}
			again := revisions(t, load(t, tc.path))
			for id, rev := range first {
				if again[id] != rev {
					t.Errorf("item %q revision changed across loads: %q then %q", id, rev, again[id])
				}
			}
		})
	}
}

const revisionSuite = `title: Revisions
items:
  - id: a
    title: First
    show:
      - { kind: text, text: %q }
    context:
      - { kind: text, text: %q }
`

func TestLoad_Revision(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		before, after [2]string // show text, context text
		same          bool
	}{
		{"unchanged file", [2]string{"draft", "notes"}, [2]string{"draft", "notes"}, true},
		{"show changes", [2]string{"draft", "notes"}, [2]string{"draft v2", "notes"}, false},
		{"only context changes", [2]string{"draft", "notes"}, [2]string{"draft", "notes v2"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := revisions(t, load(t, writeSuite(t, fmt.Sprintf(revisionSuite, tc.before[0], tc.before[1]))))["a"]
			after := revisions(t, load(t, writeSuite(t, fmt.Sprintf(revisionSuite, tc.after[0], tc.after[1]))))["a"]
			if !revisionRe.MatchString(before) {
				t.Fatalf("revision = %q, want 12 hex digits", before)
			}
			if got := before == after; got != tc.same {
				t.Errorf("revision %q -> %q: same = %v, want %v", before, after, got, tc.same)
			}
		})
	}
}

func TestLoad_InvalidSuites(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		file     string
		prefix   string   // the message starts with this
		contains []string // the message contains each of these
		line     int
		path     string
		fatal    bool
		warning  bool
		item     string   // Problem.Item
		kept     []string // titles left in Doc["items"]; unused when fatal
		askOf    string   // item whose remaining ask ids are checked
		ask      []string
	}{
		{
			name: "yaml syntax error", file: "syntax.yaml",
			prefix: "YAML syntax:", line: 4, fatal: true,
		},
		{
			name: "unknown top-level key", file: "unknown-top-key.yaml",
			contains: []string{"unknown key", "titel"}, line: 2, path: "titel", fatal: true,
		},
		{
			name: "unknown artifact kind", file: "unknown-kind.yaml",
			contains: []string{`unknown kind "chartt"`}, line: 6, path: "items[0].show[0].kind",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "duplicate item id", file: "duplicate-item.yaml",
			contains: []string{`duplicate item id "a"`}, line: 7, path: "items[1].id",
			item: "a", kept: []string{"First", "Third"},
		},
		{
			name: "item question key reserved by the kit", file: "reserved-key.yaml",
			contains: []string{`key "j" is already used by the kit`}, line: 6, path: "items[0].ask[0].key",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "proposed value not an option", file: "proposed-not-option.yaml",
			contains: []string{"is not an option"}, line: 13, path: "items[0].proposed.risk.value",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "suite question with unknown question", file: "unknown-with.yaml",
			contains: []string{`unknown question "verdict"`}, line: 6, path: "questions[0].with", fatal: true,
		},
		{
			name: "item group not defined", file: "unknown-group.yaml",
			contains: []string{`unknown group "lvie"`}, line: 6, path: "items[0].group",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "item question each names a missing artifact", file: "unknown-artifact.yaml",
			contains: []string{`unknown artifact "evidence"`}, line: 8, path: "items[0].ask[0].each",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "two primary questions", file: "two-primary.yaml",
			contains: []string{"only one question can be primary"}, line: 3, path: "questions", fatal: true,
		},
		{
			name: "action question without states", file: "action-no-states.yaml",
			contains: []string{"states"}, line: 3, path: "questions[0]", fatal: true,
		},
		{
			name: "with names a text question", file: "with-text.yaml",
			contains: []string{`cannot render with "notes"`, "text question"}, line: 7, path: "items[0].ask[1].with",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "with names a per-row question", file: "with-per-row-target.yaml",
			contains: []string{`cannot render with "keep"`, "asked per row"}, line: 9, path: "items[0].ask[1].with",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "per-row question renders with another", file: "with-from-per-row.yaml",
			contains: []string{"a per-row question cannot render with another question"}, line: 9, path: "items[0].ask[1].with",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "row without id under an each question", file: "row-no-id.yaml",
			contains: []string{`row needs an id: question "keep" asks about each row`}, line: 11, path: "items[0].show[0].rows[1]",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "row without id under a suite from question", file: "row-no-id-suite.yaml",
			contains: []string{`row needs an id: question "best" asks about each row`}, line: 12, path: "items[0].context[0].rows[0]",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "proposed action", file: "proposed-action.yaml",
			contains: []string{"an action cannot be proposed; the human approves it"}, line: 12, path: "items[0].proposed.post",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "proposed multi value not a list", file: "proposed-multi-not-list.yaml",
			contains: []string{`"flags" takes several options`, "must be a list"}, line: 13, path: "items[0].proposed.flags.value",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "proposed multi element not an option", file: "proposed-multi-not-option.yaml",
			contains: []string{`proposed value z is not an option of "flags"`}, line: 13, path: "items[0].proposed.flags.value[1]",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "proposed scale above the default max", file: "proposed-scale-range.yaml",
			contains: []string{`proposed value 7 for "clarity" is above the maximum 5`}, line: 10, path: "items[0].proposed.clarity.value",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "proposed scale not a number", file: "proposed-scale-not-number.yaml",
			contains: []string{`proposed value high for "clarity" must be a number`}, line: 12, path: "items[0].proposed.clarity.value",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "proposed number below min", file: "proposed-number-range.yaml",
			contains: []string{`proposed value -1 for "score" is below the minimum 0`}, line: 12, path: "items[0].proposed.score.value",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "per-row proposal names an unknown row", file: "proposed-unknown-row.yaml",
			contains: []string{`unknown row "r9" in artifact "rows"`}, line: 10, path: "items[0].proposed.keep.value.r9",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "duplicate ask label", file: "duplicate-label.yaml",
			contains: []string{"duplicate question label"}, line: 7, path: "items[0].ask[1].label", warning: true,
			item: "a", kept: []string{"First", "Second"}, askOf: "a", ask: []string{"why"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := load(t, filepath.Join("testdata", "bad", tc.file))
			if len(res.Problems) != 1 {
				t.Fatalf("Load(%s) problems = %v, want exactly one", tc.file, res.Problems)
			}
			p := res.Problems[0]
			if !strings.HasPrefix(p.Message, tc.prefix) {
				t.Errorf("message = %q, want prefix %q", p.Message, tc.prefix)
			}
			for _, s := range tc.contains {
				if !strings.Contains(p.Message, s) {
					t.Errorf("message = %q, want it to contain %q", p.Message, s)
				}
			}
			if p.File != tc.file {
				t.Errorf("File = %q, want %q", p.File, tc.file)
			}
			if p.Line != tc.line {
				t.Errorf("Line = %d, want %d (%s)", p.Line, tc.line, p)
			}
			if p.Path != tc.path {
				t.Errorf("Path = %q, want %q", p.Path, tc.path)
			}
			if p.Warning != tc.warning {
				t.Errorf("Warning = %v, want %v", p.Warning, tc.warning)
			}
			if p.Item != tc.item {
				t.Errorf("Item = %q, want %q", p.Item, tc.item)
			}
			if res.Fatal != tc.fatal {
				t.Fatalf("Fatal = %v, want %v", res.Fatal, tc.fatal)
			}
			if tc.fatal {
				if res.Doc != nil {
					t.Errorf("Doc = %v, want nil when fatal", res.Doc)
				}
				return
			}
			var titles []string
			for _, it := range docItems(t, res) {
				titles = append(titles, it["title"].(string))
				if it["id"] == tc.askOf {
					var ids []string
					for _, q := range it["ask"].([]any) {
						ids = append(ids, q.(map[string]any)["id"].(string))
					}
					if !slices.Equal(ids, tc.ask) {
						t.Errorf("item %q ask ids = %v, want %v", tc.askOf, ids, tc.ask)
					}
				}
			}
			if !slices.Equal(titles, tc.kept) {
				t.Errorf("Doc item titles = %v, want %v", titles, tc.kept)
			}
		})
	}
}

func TestProblem_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   suite.Problem
		want string
	}{
		{
			name: "item error with location",
			in:   suite.Problem{File: "suite.yaml", Line: 12, Col: 5, Path: "items[0].show[0].kind", Item: "x", Message: `unknown kind "chartt"`},
			want: `suite.yaml:12:5: error: items[0].show[0].kind (item x): unknown kind "chartt"`,
		},
		{
			name: "warning",
			in:   suite.Problem{File: "suite.yaml", Line: 3, Col: 7, Path: "items[1].ask[1].label", Item: "y", Message: "duplicate question label; dropped", Warning: true},
			want: "suite.yaml:3:7: warning: items[1].ask[1].label (item y): duplicate question label; dropped",
		},
		{
			name: "no line and no path",
			in:   suite.Problem{File: "suite.yaml", Message: "the file is empty"},
			want: "suite.yaml: error: the file is empty",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.in.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoad_KeepsKeyOrder(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "suite.yaml")
	src := "title: Order\nitems:\n  - id: a\n    title: A\n    show:\n      - kind: json\n        data: {zeta: 1, alpha: 2, mid: {b: 1, a: 2}}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res := load(t, path)
	b, err := json.Marshal(res.Doc)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"data":{"zeta":1,"alpha":2,"mid":{"b":1,"a":2}}`; !strings.Contains(string(b), want) {
		t.Errorf("Doc JSON = %s, want it to contain %s", b, want)
	}
}
