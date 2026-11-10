package feedback_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jryio/evalkit/internal/feedback"
)

const waitTimeout = 2 * time.Second

func openDir(tb testing.TB) *feedback.Dir {
	tb.Helper()
	d, err := feedback.Open(filepath.Join(tb.TempDir(), "feedback"))
	if err != nil {
		tb.Fatalf("Open: %v", err)
	}
	return d
}

func submit(tb testing.TB, d *feedback.Dir) feedback.Snapshot {
	tb.Helper()
	snap, err := d.Submit(map[string]any{"items": map[string]any{}})
	if err != nil {
		tb.Fatalf("Submit: %v", err)
	}
	return snap
}

func state(tb testing.TB, d *feedback.Dir) map[string]any {
	tb.Helper()
	b, err := d.State()
	if err != nil {
		tb.Fatalf("State: %v", err)
	}
	return decodeJSON(tb, b)
}

func decodeJSON(tb testing.TB, b []byte) map[string]any {
	tb.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		tb.Fatalf("decode %q: %v", b, err)
	}
	return m
}

func TestState_EmptyWhenNothingSaved(t *testing.T) {
	t.Parallel()
	b, err := openDir(t).State()
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if string(b) != "{}" {
		t.Errorf("State() = %q, want {}", b)
	}
}

func TestSaveState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		want    string // the stored state when no error is wanted
		wantErr bool
	}{
		{name: "object without rev", in: `{"items":{"a":{"revision":"r1"}}}`, want: `{"items":{"a":{"revision":"r1"}},"rev":1}`},
		{name: "object with rev 0", in: `{"rev":0,"n":1.5}`, want: `{"n":1.5,"rev":1}`},
		{name: "array", in: `[1,2]`, wantErr: true},
		{name: "string", in: `"x"`, wantErr: true},
		{name: "number", in: `1`, wantErr: true},
		{name: "not json", in: `{`, wantErr: true},
		{name: "null", in: `null`, wantErr: true},
		{name: "negative rev", in: `{"rev":-1}`, wantErr: true},
		{name: "fractional rev", in: `{"rev":0.5}`, wantErr: true},
		{name: "string rev", in: `{"rev":"0"}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := openDir(t)
			rev, err := d.SaveState([]byte(tc.in))
			if (err != nil) != tc.wantErr {
				t.Fatalf("SaveState(%s) error = %v, want error %v", tc.in, err, tc.wantErr)
			}
			if tc.wantErr {
				if errors.Is(err, feedback.ErrConflict) {
					t.Errorf("SaveState(%s) error = %v, want a validation error", tc.in, err)
				}
				return
			}
			if rev != 1 {
				t.Errorf("SaveState(%s) rev = %d, want 1", tc.in, rev)
			}
			if got, want := state(t, d), decodeJSON(t, []byte(tc.want)); !reflect.DeepEqual(got, want) {
				t.Errorf("State() = %v, want %v", got, want)
			}
		})
	}
}

func TestSaveState_Revisions(t *testing.T) {
	t.Parallel()
	d := openDir(t)
	for want := 1; want <= 3; want++ {
		rev, err := d.SaveState(fmt.Appendf(nil, `{"rev":%d,"n":%d}`, want-1, want))
		if err != nil || rev != want {
			t.Fatalf("SaveState(rev=%d) = %d, %v, want %d, nil", want-1, rev, err, want)
		}
	}
	for _, stale := range []string{`{"n":"stale"}`, `{"rev":1,"n":"stale"}`, `{"rev":9,"n":"stale"}`} {
		rev, err := d.SaveState([]byte(stale))
		if !errors.Is(err, feedback.ErrConflict) || rev != 3 {
			t.Errorf("SaveState(%s) = %d, %v, want 3, ErrConflict", stale, rev, err)
		}
	}
	if got, want := state(t, d), decodeJSON(t, []byte(`{"rev":3,"n":3}`)); !reflect.DeepEqual(got, want) {
		t.Errorf("State() after conflicts = %v, want %v", got, want)
	}
}

func TestSubmit_NumbersSnapshots(t *testing.T) {
	t.Parallel()
	d := openDir(t)
	if got := d.Latest(); got != 0 {
		t.Errorf("Latest() before any send = %d, want 0", got)
	}
	for want := 1; want <= 3; want++ {
		snap := submit(t, d)
		if snap.Sequence != want {
			t.Errorf("Submit #%d sequence = %d", want, snap.Sequence)
		}
		if wantPath := filepath.Join(d.Path(), fmt.Sprintf("sent-%02d.json", want)); snap.Path != wantPath {
			t.Errorf("Submit #%d path = %q, want %q", want, snap.Path, wantPath)
		}
		b, err := os.ReadFile(snap.Path)
		if err != nil {
			t.Fatalf("read snapshot: %v", err)
		}
		got := decodeJSON(t, b)
		if got["sequence"] != float64(want) {
			t.Errorf("%s sequence = %v, want %d", snap.Path, got["sequence"], want)
		}
		sentAt, _ := got["sent_at"].(string)
		if _, err := time.Parse(time.RFC3339, sentAt); err != nil {
			t.Errorf("%s sent_at = %q, want RFC 3339: %v", snap.Path, sentAt, err)
		}
	}
	if got := d.Latest(); got != 3 {
		t.Errorf("Latest() = %d, want 3", got)
	}
}

func TestPoll_SeesOtherDir(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "feedback")
	reader, err := feedback.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	writer, err := feedback.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	type result struct {
		seq int
		err error
	}
	got := make(chan result, 1)
	go func() {
		seq, err := reader.Poll(t.Context(), 0, 10*time.Millisecond)
		got <- result{seq, err}
	}()
	submit(t, writer)
	select {
	case r := <-got:
		if r.err != nil || r.seq != 1 {
			t.Errorf("Poll = %d, %v, want 1, nil", r.seq, r.err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Poll did not see the other Dir's snapshot")
	}
}

func TestSeen_RoundTrip(t *testing.T) {
	t.Parallel()
	d := openDir(t)
	if got := d.Seen(); got != 0 {
		t.Errorf("Seen() before SetSeen = %d, want 0", got)
	}
	if err := d.SetSeen(4); err != nil {
		t.Fatalf("SetSeen: %v", err)
	}
	if got := d.Seen(); got != 4 {
		t.Errorf("Seen() = %d, want 4", got)
	}
}
