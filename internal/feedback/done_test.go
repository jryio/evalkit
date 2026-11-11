package feedback_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jryio/evalkit/internal/feedback"
)

func recordDone(tb testing.TB, d *feedback.Dir, item, action string, rec feedback.Done) {
	tb.Helper()
	if err := d.RecordDone(item, action, rec); err != nil {
		tb.Fatalf("RecordDone(%q, %q): %v", item, action, err)
	}
}

func TestRecordDone(t *testing.T) {
	t.Parallel()
	d := openDir(t)
	all, err := d.DoneAll()
	if err != nil || len(all) != 0 {
		t.Fatalf("DoneAll() on an empty dir = %v, %v, want empty, nil", all, err)
	}
	recordDone(t, d, "a", "post", feedback.Done{URL: "https://x/1"})
	recordDone(t, d, "a", "merge", feedback.Done{At: "2026-11-09T00:00:00Z", Note: "merged"})
	recordDone(t, d, "a", "post", feedback.Done{At: "2026-11-09T01:00:00Z", URL: "https://x/2"})
	all, err = d.DoneAll()
	if err != nil {
		t.Fatalf("DoneAll: %v", err)
	}
	want := map[string]map[string]feedback.Done{"a": {
		"post":  {At: "2026-11-09T01:00:00Z", URL: "https://x/2"},
		"merge": {At: "2026-11-09T00:00:00Z", Note: "merged"},
	}}
	if !reflect.DeepEqual(all, want) {
		t.Errorf("DoneAll() = %v, want %v", all, want)
	}
}

func TestRecordDone_RunItem(t *testing.T) {
	t.Parallel()
	d := openDir(t)
	recordDone(t, d, "v9/auth", "post", feedback.Done{At: "2026-11-09T00:00:00Z"})
	all, err := d.DoneAll()
	if err != nil {
		t.Fatalf("DoneAll: %v", err)
	}
	want := map[string]map[string]feedback.Done{"v9/auth": {"post": {At: "2026-11-09T00:00:00Z"}}}
	if !reflect.DeepEqual(all, want) {
		t.Errorf("DoneAll() = %v, want %v", all, want)
	}
}

func TestRecordDone_OneFilePerAction(t *testing.T) {
	t.Parallel()
	d := openDir(t)
	recordDone(t, d, "item-1", "post.v2", feedback.Done{URL: "https://x/1"})
	path := filepath.Join(d.Path(), "done", "item-1", "post.v2.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var got feedback.Done
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if got.URL != "https://x/1" {
		t.Errorf("%s url = %q, want https://x/1", path, got.URL)
	}
}

func TestRecordDone_DefaultsAt(t *testing.T) {
	t.Parallel()
	d := openDir(t)
	recordDone(t, d, "a", "post", feedback.Done{})
	all, err := d.DoneAll()
	if err != nil {
		t.Fatalf("DoneAll: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, all["a"]["post"].At); err != nil {
		t.Errorf("At = %q, want RFC 3339: %v", all["a"]["post"].At, err)
	}
}

func TestRecordDone_RejectsBadIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, item, action string
	}{
		{"empty item", "", "post"},
		{"empty action", "a", ""},
		{"parent item", "..", "post"},
		{"double dot inside action", "a", "x..y"},
		{"two slashes in item", "a/b/c", "post"},
		{"empty run", "/b", "post"},
		{"empty run item", "a/", "post"},
		{"traversal in run item", "a/..", "post"},
		{"traversal in action", "a", "../../escape"},
		{"leading dot", ".hidden", "post"},
		{"leading dash", "a", "-post"},
		{"space", "a b", "post"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := openDir(t)
			if err := d.RecordDone(tc.item, tc.action, feedback.Done{}); err == nil {
				t.Errorf("RecordDone(%q, %q) = nil, want an error", tc.item, tc.action)
			}
			if _, err := os.Stat(filepath.Join(d.Path(), "done")); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("RecordDone(%q, %q) created done/ (stat error %v), want no write", tc.item, tc.action, err)
			}
			entries, err := os.ReadDir(filepath.Dir(d.Path()))
			if err != nil {
				t.Fatalf("ReadDir: %v", err)
			}
			if len(entries) != 1 {
				t.Errorf("RecordDone(%q, %q) wrote beside the feedback dir: %v", tc.item, tc.action, entries)
			}
		})
	}
}

func TestRecordDone_Parallel(t *testing.T) {
	t.Parallel()
	d := openDir(t)
	const n = 20
	name := func(i int) (item, action string) {
		return fmt.Sprintf("item%d", i%4), fmt.Sprintf("action%d", i)
	}
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			item, action := name(i)
			errs[i] = d.RecordDone(item, action, feedback.Done{Note: strconv.Itoa(i)})
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatalf("RecordDone: %v", err)
	}
	all, err := d.DoneAll()
	if err != nil {
		t.Fatalf("DoneAll: %v", err)
	}
	for i := range n {
		item, action := name(i)
		if got := all[item][action].Note; got != strconv.Itoa(i) {
			t.Errorf("DoneAll()[%s][%s].Note = %q, want %q", item, action, got, strconv.Itoa(i))
		}
	}
	total := 0
	for _, actions := range all {
		total += len(actions)
	}
	if total != n {
		t.Errorf("DoneAll() holds %d results, want %d", total, n)
	}
}

func TestDoneVersion(t *testing.T) {
	t.Parallel()
	d := openDir(t)
	if v := d.DoneVersion(); v != "" {
		t.Errorf("DoneVersion() before any record = %q, want empty", v)
	}
	recordDone(t, d, "a", "post", feedback.Done{})
	added := d.DoneVersion()
	if added == "" {
		t.Fatal("DoneVersion() after RecordDone is empty")
	}
	recordDone(t, d, "a", "merge", feedback.Done{})
	second := d.DoneVersion()
	if second == added {
		t.Errorf("DoneVersion() = %q after adding a result, want a change", second)
	}
	// A rewrite changes only the mtime; some filesystems tick every few milliseconds.
	time.Sleep(20 * time.Millisecond)
	recordDone(t, d, "a", "post", feedback.Done{Note: "again"})
	if rewritten := d.DoneVersion(); rewritten == second {
		t.Errorf("DoneVersion() = %q after rewriting a result, want a change", rewritten)
	}
}
