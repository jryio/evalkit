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
	return writeDir(tb, map[string]string{"suite.yaml": content})
}

// writeDir writes files (slash path -> content) to a temp dir and returns the
// path of suite.yaml in it.
func writeDir(tb testing.TB, files map[string]string) string {
	tb.Helper()
	dir := tb.TempDir()
	for name, content := range files {
		writeFile(tb, filepath.Join(dir, filepath.FromSlash(name)), content)
	}
	return filepath.Join(dir, "suite.yaml")
}

func writeFile(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatalf("write %s: %v", path, err)
	}
}

// jsonl is one JSONL item line.
func jsonl(id, title string) string {
	return fmt.Sprintf(`{"id":%q,"title":%q}`+"\n", id, title)
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
			name: "compare with one pane", file: "compare-one-pane.yaml",
			contains: []string{"minItems"}, line: 8, path: "items[0].show[0].panes",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "compare pane with an unknown key", file: "compare-unknown-pane-key.yaml",
			contains: []string{"unknown key 'colour'"}, line: 9, path: "items[0].show[0].panes[1].colour",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "compare pane error reported once, not on its siblings", file: "compare-bad-highlight.yaml",
			contains: []string{"want array"}, line: 11, path: "items[0].show[0].panes[1].highlight",
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
			name: "no items and no items_from", file: "no-items.yaml",
			contains: []string{"a suite needs items or items_from"}, line: 1, fatal: true,
		},
		{
			name: "duplicate column id", file: "duplicate-column.yaml",
			contains: []string{`duplicate column id "score"`}, line: 4, path: "columns[1].id", fatal: true,
		},
		{
			name: "column id same as a question", file: "column-question-id.yaml",
			contains: []string{`column "verdict" has the same id as a question`}, line: 5, path: "columns[0].id", fatal: true,
		},
		{
			name: "column min not below max", file: "column-min-max.yaml",
			contains: []string{"min must be below max"}, line: 4, path: "columns[0].min", fatal: true,
		},
		{
			name: "value for an unknown column", file: "unknown-value.yaml",
			contains: []string{`unknown column "latency" (want one of "score", "cost")`}, line: 10, path: "items[0].values.latency",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "values without columns", file: "values-no-columns.yaml",
			contains: []string{"values set but the suite defines no columns"}, line: 5, path: "items[0].values",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "per-row proposal with a baseline", file: "proposed-baseline-per-row.yaml",
			contains: []string{"baseline is not supported for per-row questions"}, line: 10, path: "items[0].proposed.keep.baseline",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "proposal baseline not an option", file: "proposed-baseline-not-option.yaml",
			contains: []string{`proposed value medium is not an option of "risk"`}, line: 15, path: "items[0].proposed.risk.baseline",
			item: "a", kept: []string{"Second"},
		},
		{
			name: "number question min not below max", file: "question-min-max.yaml",
			contains: []string{"min must be below max"}, line: 6, path: "questions[0].min", fatal: true,
		},
		{
			name: "item scale question max at the default min", file: "ask-scale-min-max.yaml",
			contains: []string{"min must be below max"}, line: 6, path: "items[0].ask[0].min",
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

// canonicalJSON keeps key order and turns the surrogate pair into one rune.
func TestLoad_JSONLKeepsKeyOrder(t *testing.T) {
	t.Parallel()
	path := writeDir(t, map[string]string{
		"suite.yaml": "title: Order\nitems_from: runs.jsonl\n",
		"runs.jsonl": `{"id":"j","title":"J \ud83d\udc40","show":[{"kind":"json","data":{"zeta":1,"alpha":2,"mid":{"b":[3,1],"a":null,"ok":true}}}]}` + "\n",
	})
	res := load(t, path)
	if len(res.Problems) != 0 {
		t.Fatalf("problems = %v, want none", res.Problems)
	}
	b, err := json.Marshal(res.Doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"title":"J ` + "\U0001F440" + `"`,
		`"data":{"zeta":1,"alpha":2,"mid":{"b":[3,1],"a":null,"ok":true}}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("Doc JSON = %s, want it to contain %s", b, want)
		}
	}
}

func TestLoad_TableSuite(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "table.yaml")
	res := load(t, path)
	if res.Fatal || len(res.Problems) != 0 {
		t.Fatalf("Load(%q) fatal = %v, problems = %v, want a clean load", path, res.Fatal, res.Problems)
	}
	if got := len(docItems(t, res)); got != 2 {
		t.Errorf("Load(%q) items = %d, want 2", path, got)
	}
}

func TestLoad_ItemsFrom(t *testing.T) {
	t.Parallel()
	const inline = "title: Runs\nitems:\n  - { id: a, title: A }\n"
	tests := []struct {
		name     string
		dir      string            // the suite's directory under the temp dir
		files    map[string]string // suite.yaml and the files beside it, relative to the temp dir
		items    []string          // id=title of each Doc item, in order
		problems []string          // prefix of each Problem.String(), in order
		fatal    bool
	}{
		{
			name: "string form appends after inline items",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": jsonl("b", "B") + jsonl("c", "C"),
			},
			items: []string{"a=A", "b=B", "c=C"},
		},
		{
			name: "list form keeps entry order",
			files: map[string]string{
				"suite.yaml": inline + "items_from: [z.jsonl, y.jsonl]\n",
				"z.jsonl":    jsonl("b", "B"),
				"y.jsonl":    jsonl("c", "C"),
			},
			items: []string{"a=A", "b=B", "c=C"},
		},
		{
			name: "glob sorts its matches",
			files: map[string]string{
				"suite.yaml":   inline + "items_from: runs/*.jsonl\n",
				"runs/2.jsonl": jsonl("c", "C"),
				"runs/1.jsonl": jsonl("b", "B"),
			},
			items: []string{"a=A", "b=B", "c=C"},
		},
		{
			name: "a file named twice loads once",
			files: map[string]string{
				"suite.yaml":   inline + "items_from: [runs/1.jsonl, \"runs/*.jsonl\"]\n",
				"runs/1.jsonl": jsonl("b", "B"),
				"runs/2.jsonl": jsonl("c", "C"),
			},
			items: []string{"a=A", "b=B", "c=C"},
		},
		{
			name: "items_from without items",
			files: map[string]string{
				"suite.yaml": "title: Runs\nitems_from: runs.jsonl\n",
				"runs.jsonl": jsonl("b", "B"),
			},
			items: []string{"b=B"},
		},
		{
			name: "explicit empty items",
			files: map[string]string{
				"suite.yaml": "title: Runs\nitems: []\n",
			},
		},
		{
			name: "a later line replaces an earlier one in place",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": jsonl("b", "B1") + jsonl("c", "C") + jsonl("b", "B2"),
			},
			items: []string{"a=A", "b=B2", "c=C"},
		},
		{
			name: "a later file replaces an earlier one's line",
			files: map[string]string{
				"suite.yaml": inline + "items_from: [x.jsonl, y.jsonl]\n",
				"x.jsonl":    jsonl("b", "B1") + jsonl("c", "C"),
				"y.jsonl":    jsonl("b", "B2"),
			},
			items: []string{"a=A", "b=B2", "c=C"},
		},
		{
			name: "a line does not replace an inline item",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": jsonl("a", "A2"),
			},
			items:    []string{"a=A"},
			problems: []string{`runs.jsonl:1:1: error: items[1].id (item a): duplicate item id "a" (first at items[0])`},
		},
		{
			name: "unfinished last line and blank lines are skipped",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": jsonl("b", "B") + "\n  \n" + jsonl("c", "C") + `{"id":"d","ti`,
			},
			items: []string{"a=A", "b=B", "c=C"},
		},
		{
			name: "malformed line",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": jsonl("b", "B") + "{not json\n" + jsonl("c", "C"),
			},
			items:    []string{"a=A", "b=B", "c=C"},
			problems: []string{"runs.jsonl:2:1: error: JSON syntax: "},
		},
		{
			name: "escaped surrogate pair",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": `{"id":"e1","title":"watching \ud83d\udc40"}` + "\n",
			},
			items: []string{"a=A", "e1=watching \U0001F440"},
		},
		{
			name: "malformed line reports the JSON error",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": jsonl("b", "B") + `{"id": "x" "title": "X"}` + "\n",
			},
			items:    []string{"a=A", "b=B"},
			problems: []string{`runs.jsonl:2:1: error: JSON syntax: invalid character '"' after object key:value pair`},
		},
		{
			name: "line not an object",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": "[1, 2]\n" + jsonl("b", "B"),
			},
			items:    []string{"a=A", "b=B"},
			problems: []string{"runs.jsonl:1:1: error: each line must be a JSON object"},
		},
		{
			name: "semantic error in a line hides only that item",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": jsonl("b", "B") + `{"id":"c","title":"C","show":[{"kind":"chartt"}]}` + "\n" + jsonl("d", "D"),
			},
			items:    []string{"a=A", "b=B", "d=D"},
			problems: []string{`runs.jsonl:2:1: error: items[2].show[0].kind (item c): unknown kind "chartt"`},
		},
		{
			name: "schema error in a line hides only that item",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": jsonl("b", "B") + `{"id":"c"}` + "\n",
			},
			items:    []string{"a=A", "b=B"},
			problems: []string{"runs.jsonl:2:1: error: items[2] (item c): "},
		},
		{
			name: "missing file is a warning",
			files: map[string]string{
				"suite.yaml": inline + "items_from: missing.jsonl\n",
			},
			items:    []string{"a=A"},
			problems: []string{`suite.yaml:4:13: warning: items_from: "missing.jsonl" matches no file yet`},
		},
		{
			name: "list entry matching nothing is a warning",
			files: map[string]string{
				"suite.yaml": inline + "items_from: [runs.jsonl, \"later/*.jsonl\"]\n",
				"runs.jsonl": jsonl("b", "B"),
			},
			items:    []string{"a=A", "b=B"},
			problems: []string{`suite.yaml:4:26: warning: items_from[1]: "later/*.jsonl" matches no file yet`},
		},
		{
			name: "a directory is not a file",
			files: map[string]string{
				"suite.yaml":   inline + "items_from: runs\n",
				"runs/1.jsonl": jsonl("b", "B"),
			},
			items:    []string{"a=A"},
			problems: []string{`suite.yaml:4:13: warning: items_from: "runs" matches no file yet`},
		},
		{
			name: "bad glob pattern",
			files: map[string]string{
				"suite.yaml":   inline + "items_from: \"runs/[.jsonl\"\n",
				"runs/1.jsonl": jsonl("b", "B"),
			},
			problems: []string{`suite.yaml:4:13: error: items_from: bad glob pattern "runs/[.jsonl"`},
			fatal:    true,
		},
		{
			name: "plain path in a directory with glob characters",
			dir:  "b [v2]",
			files: map[string]string{
				"b [v2]/suite.yaml":   "title: Runs\nitems_from: runs/1.jsonl\n",
				"b [v2]/runs/1.jsonl": jsonl("b", "B"),
			},
			items: []string{"b=B"},
		},
		{
			name: "glob in a directory with glob characters",
			dir:  "b [v2]",
			files: map[string]string{
				"b [v2]/suite.yaml":   "title: Runs\nitems_from: runs/*.jsonl\n",
				"b [v2]/runs/1.jsonl": jsonl("b", "B"),
				"b [v2]/runs/2.jsonl": jsonl("c", "C"),
			},
			items: []string{"b=B", "c=C"},
		},
		{
			name: "two objects on one line",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": `{"id":"b","title":"B"} {"id":"c","title":"C"}` + "\n",
			},
			items:    []string{"a=A"},
			problems: []string{"runs.jsonl:1:1: error: JSON syntax: unexpected data after the JSON value"},
		},
		{
			name: "data after the object",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": `{"id":"x","title":"x"} junk` + "\n",
			},
			items:    []string{"a=A"},
			problems: []string{"runs.jsonl:1:1: error: JSON syntax: unexpected data after the JSON value"},
		},
		{
			name: "YAML flow mapping",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": "{id: yamlish, title: t}\n",
			},
			items:    []string{"a=A"},
			problems: []string{"runs.jsonl:1:1: error: JSON syntax: invalid character 'i'"},
		},
		{
			name: "trailing comma",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": jsonl("b", "B") + `{"id": "x",}` + "\n",
			},
			items:    []string{"a=A", "b=B"},
			problems: []string{"runs.jsonl:2:1: error: JSON syntax: invalid character '}' looking for beginning of object key string"},
		},
		{
			name: "control characters YAML does not allow raw",
			files: map[string]string{
				"suite.yaml": inline + "items_from: runs.jsonl\n",
				"runs.jsonl": `{"id":"d1","title":"del\u007f"}` + "\n" +
					"{\"id\":\"d2\",\"title\":\"del\x7f\"}\n" +
					"{\"id\":\"n1\",\"title\":\"nel\xc2\x85\"}\n",
			},
			items: []string{"a=A", "d1=del\x7f", "d2=del\x7f", "n1=nel\xc2\x85"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := filepath.Dir(writeDir(t, tc.files))
			res := load(t, filepath.Join(root, tc.dir, "suite.yaml"))
			if res.Fatal != tc.fatal {
				t.Fatalf("Fatal = %v, want %v; problems = %v", res.Fatal, tc.fatal, res.Problems)
			}
			if len(res.Problems) != len(tc.problems) {
				t.Fatalf("problems = %v, want %d", res.Problems, len(tc.problems))
			}
			for i, p := range res.Problems {
				if !strings.HasPrefix(p.String(), tc.problems[i]) {
					t.Errorf("problem %d = %q, want prefix %q", i, p, tc.problems[i])
				}
			}
			var items []string
			for _, it := range docItems(t, res) {
				items = append(items, fmt.Sprintf("%v=%v", it["id"], it["title"]))
			}
			if !slices.Equal(items, tc.items) {
				t.Errorf("Doc items = %v, want %v", items, tc.items)
			}
		})
	}
}

// docRuns returns the ids of the Doc's runs, in order.
func docRuns(tb testing.TB, res suite.Result) []string {
	tb.Helper()
	b, err := json.Marshal(res.Doc)
	if err != nil {
		tb.Fatalf("marshal Doc: %v", err)
	}
	var doc struct {
		Runs []struct {
			ID string `json:"id"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		tb.Fatalf("unmarshal Doc: %v", err)
	}
	var ids []string
	for _, r := range doc.Runs {
		ids = append(ids, r.ID)
	}
	return ids
}

func TestLoad_Runs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		file     string   // under testdata/runs
		items    []string // "run id pair title" of each Doc item, in order
		runs     []string // ids of the Doc's runs, in order
		problems []string // prefix of each Problem.String(), in order
		fatal    bool
	}{
		{
			name: "two runs with partial overlap and a rep",
			file: "overlap.yaml",
			items: []string{
				"v8 v8/auth auth Auth v8",
				"v8 v8/billing billing Billing v8",
				"v9 v9/auth auth Auth v9",
				"v9 v9/auth.r1 auth.r1 Auth v9 rep 1",
				"v9 v9/search search Search v9",
			},
			runs: []string{"v8", "v9"},
		},
		{
			name: "run ids written as unquoted dates",
			file: "date-ids.yaml",
			items: []string{
				"2026-10-05 2026-10-05/auth auth Auth v8",
				"2026-10-05 2026-10-05/billing billing Billing v8",
				"2026-10-12 2026-10-12/auth auth Auth v9",
				"2026-10-12 2026-10-12/auth.r1 auth.r1 Auth v9 rep 1",
				"2026-10-12 2026-10-12/search search Search v9",
			},
			runs: []string{"2026-10-05", "2026-10-12"},
		},
		{
			name: "a line names another run",
			file: "conflict.yaml",
			items: []string{
				"v8 v8/auth auth Auth v8",
				"v8 v8/billing billing Billing v8",
				"v9 v9/billing billing Billing v9",
				"v9 v9/search search Search v9",
			},
			runs:     []string{"v8", "v9"},
			problems: []string{`conflict.jsonl:1:1: error: line says run "v8" but runs[1] "v9" reads it`},
		},
		{
			name: "a later line replaces the same run, id and rep in place",
			file: "replace.yaml",
			items: []string{
				"v8 v8/auth auth Auth v8",
				"v8 v8/billing billing Billing v8",
				"v9 v9/auth auth Auth 3",
				"v9 v9/billing billing Billing",
				"v9 v9/auth.r1 auth.r1 Auth rep 1",
			},
			runs: []string{"v8", "v9"},
		},
		{
			name: "duplicate run, id and rep",
			file: "duplicate.yaml",
			items: []string{
				"v9 v9/auth auth Auth inline",
				"v9 v9/billing.r1 billing.r1 Billing",
				"v9 v9/auth.r1 auth.r1 Auth v9 rep 1",
				"v9 v9/search search Search v9",
			},
			runs: []string{"v9"},
			problems: []string{
				`duplicate.yaml:7:11: error: items[2].id (item v9/billing.r1): duplicate item "billing" rep 1 in run "v9" (first at items[1])`,
				`v9.jsonl:1:1: error: items[3].id (item v9/auth): duplicate item "auth" in run "v9" (first at items[0])`,
			},
		},
		{
			name: "runs not declared under runs",
			file: "undeclared.yaml",
			items: []string{
				"v10 v10/auth auth Auth v10",
				"v9 v9/auth auth Auth v9",
				"v10 v10/billing billing Billing v10",
				"v8 v8/auth auth Auth v8",
				"v8 v8/billing billing Billing v8",
			},
			runs: []string{"v8", "v10", "v9"},
			problems: []string{
				`mixed.jsonl:1:1: warning: items[0].run (item v10/auth): run "v10" is not declared under runs`,
				`mixed.jsonl:2:1: warning: items[1].run (item v9/auth): run "v9" is not declared under runs`,
			},
		},
		{
			name: "runs from the lines alone",
			file: "implied.yaml",
			items: []string{
				"v10 v10/auth auth Auth v10",
				"v9 v9/auth auth Auth v9",
				"v10 v10/billing billing Billing v10",
			},
			runs: []string{"v10", "v9"},
		},
		{
			name:     "two baselines",
			file:     "two-baselines.yaml",
			problems: []string{`two-baselines.yaml:4:25: error: runs[1].baseline: only one run can be the baseline; runs[0] "v8" already is`},
			fatal:    true,
		},
		{
			name: "an item without a run in a suite with runs",
			file: "no-run.yaml",
			items: []string{
				"v8 v8/auth auth Auth v8",
				"v8 v8/billing billing Billing v8",
			},
			runs:     []string{"v8"},
			problems: []string{`no-run.yaml:5:5: error: items[0] (item stray): item has no run: in a suite with runs, every item belongs to one`},
		},
		{
			name:     "two primary columns",
			file:     "two-primary.yaml",
			problems: []string{`two-primary.yaml:4:26: error: columns[1].primary: only one column can be primary; "audit" already is`},
			fatal:    true,
		},
		{
			name: "duplicate run and dimension ids and order values",
			file: "bad-runs.yaml",
			problems: []string{
				`bad-runs.yaml:4:11: error: dimensions[1].id: duplicate dimension id "model"`,
				`bad-runs.yaml:5:38: error: dimensions[2].order[2]: duplicate value "low" in order`,
				`bad-runs.yaml:8:11: error: runs[1].id: duplicate run id "v8"`,
			},
			fatal: true,
		},
		{
			name: "run dims and values",
			file: "dims-values.yaml",
			problems: []string{
				`dims-values.yaml:7:65: warning: runs[0].dims.temp: unknown dimension "temp" (want one of "model")`,
				`dims-values.yaml:7:106: error: runs[0].values.latency: unknown column "latency" (want one of "cost_usd")`,
			},
			fatal: true,
		},
		{
			name: "inline baselines are ignored",
			file: "baseline-ignored.yaml",
			items: []string{
				"v9 v9/auth auth Auth",
				"v9 v9/billing billing Billing",
				"v9 v9/search search Search",
			},
			runs:     []string{"v9"},
			problems: []string{`baseline-ignored.yaml:10:87: warning: items[1].values.cost_usd.baseline (item v9/billing): baseline is ignored when the suite has runs: the baseline run supplies it`},
		},
		{
			name:     "view runs without runs",
			file:     "view-runs.yaml",
			items:    []string{"<nil> auth <nil> Auth"},
			problems: []string{`view-runs.yaml:2:7: warning: view: view: runs needs runs; the table shows instead`},
		},
		{
			name: "a run's file matches nothing yet",
			file: "run-file-missing.yaml",
			items: []string{
				"v8 v8/auth auth Auth v8",
				"v8 v8/billing billing Billing v8",
				"v9 v9/auth auth Auth v9",
				"v9 v9/auth.r1 auth.r1 Auth v9 rep 1",
				"v9 v9/search search Search v9",
			},
			runs:     []string{"v8", "v9"},
			problems: []string{`run-file-missing.yaml:4:38: warning: runs[1].items_from[1]: "later.jsonl" matches no file yet`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := load(t, filepath.Join("testdata", "runs", tc.file))
			if res.Fatal != tc.fatal {
				t.Fatalf("Fatal = %v, want %v; problems = %v", res.Fatal, tc.fatal, res.Problems)
			}
			if len(res.Problems) != len(tc.problems) {
				t.Fatalf("problems = %v, want %d", res.Problems, len(tc.problems))
			}
			for i, p := range res.Problems {
				if !strings.HasPrefix(p.String(), tc.problems[i]) {
					t.Errorf("problem %d = %q, want prefix %q", i, p, tc.problems[i])
				}
			}
			var items []string
			for _, it := range docItems(t, res) {
				items = append(items, fmt.Sprintf("%v %v %v %v", it["run"], it["id"], it["pair"], it["title"]))
			}
			if !slices.Equal(items, tc.items) {
				t.Errorf("Doc items = %q, want %q", items, tc.items)
			}
			if got := docRuns(t, res); !slices.Equal(got, tc.runs) {
				t.Errorf("Doc runs = %v, want %v", got, tc.runs)
			}
		})
	}
}

// A suite without runs loads byte for byte as it did before runs existed.
func TestLoad_NoRunsUnchanged(t *testing.T) {
	t.Parallel()
	got, err := json.MarshalIndent(load(t, filepath.Join("testdata", "table.yaml")), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "table.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got)+"\n" != string(want) {
		t.Errorf("Load(table.yaml) = %s\nwant testdata/table.golden.json", got)
	}
}

func TestVersion_ChangesWithRunJSONL(t *testing.T) {
	t.Parallel()
	path := writeDir(t, map[string]string{
		"suite.yaml": "title: Runs\nruns:\n  - { id: v8, items_from: v8.jsonl }\n",
		"v8.jsonl":   jsonl("a", "A"),
	})
	before := version(t, path)
	writeFile(t, filepath.Join(filepath.Dir(path), "v8.jsonl"), jsonl("a", "A")+jsonl("b", "B"))
	if after := version(t, path); after == before {
		t.Errorf("version = %q after appending a line to a run's file, want a change", after)
	}
}

func TestVersion_MatchesLoad(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		files map[string]string
	}{
		{"no items_from", map[string]string{"suite.yaml": "title: Runs\nitems: []\n"}},
		{"items_from", map[string]string{"suite.yaml": "title: Runs\nitems_from: runs/*.jsonl\n", "runs/1.jsonl": jsonl("a", "A")}},
		{"run items_from", map[string]string{"suite.yaml": "title: Runs\nruns: [{ id: v8, items_from: v8.jsonl }]\n", "v8.jsonl": jsonl("a", "A")}},
		{"yaml syntax error", map[string]string{"suite.yaml": "title: [\n"}},
		{"top level not a mapping", map[string]string{"suite.yaml": "- a\n"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			version(t, writeDir(t, tc.files))
		})
	}
}

func TestVersion_ChangesWithJSONL(t *testing.T) {
	t.Parallel()
	path := writeDir(t, map[string]string{
		"suite.yaml":   "title: Runs\nitems_from: runs/*.jsonl\n",
		"runs/1.jsonl": jsonl("a", "A"),
	})
	runs := filepath.Join(filepath.Dir(path), "runs")
	before := version(t, path)

	writeFile(t, filepath.Join(runs, "1.jsonl"), jsonl("a", "A")+jsonl("b", "B"))
	appended := version(t, path)
	if appended == before {
		t.Errorf("version = %q after appending a line, want a change", appended)
	}

	writeFile(t, filepath.Join(runs, "2.jsonl"), jsonl("c", "C"))
	if added := version(t, path); added == appended {
		t.Errorf("version = %q after adding a file, want a change", added)
	}
}

// version returns Version(path), checking that Load reports the same.
func version(tb testing.TB, path string) string {
	tb.Helper()
	v, err := suite.Version(path)
	if err != nil {
		tb.Fatalf("Version(%q): %v", path, err)
	}
	if got := load(tb, path).Version; got != v {
		tb.Errorf("Load(%q).Version = %q, Version() = %q, want equal", path, got, v)
	}
	return v
}
