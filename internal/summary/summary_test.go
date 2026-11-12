package summary_test

import (
	"strings"
	"testing"

	"github.com/jryio/evalkit/internal/summary"
)

type obj = map[string]any

func snapshot(seq int, items obj, actions ...any) obj {
	return obj{
		"suite":    "Kitchen sink",
		"sequence": seq,
		"sent_at":  "2026-11-09T12:00:00Z",
		"progress": obj{"reviewed": 1, "total": 3},
		"actions":  actions,
		"items":    items,
	}
}

func item(title string, answers obj) obj {
	return obj{"title": title, "revision": "r1", "answers": answers, "edits": obj{}}
}

func TestWrite(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		cur, prev obj
		want      []string // each must appear in the output
		notWant   []string // none may appear
	}{
		{
			name: "first send lists every answer",
			cur: snapshot(1, obj{
				"a": obj{"title": "Alpha", "revision": "r1", "answers": obj{"verdict": "good", "notes": "looks fine"}, "edits": obj{"draft": "new text"}},
				"b": item("Beta", obj{"verdict": "bad"}),
			}),
			want: []string{
				"Kitchen sink · sent #1 at 2026-11-09T12:00:00Z · 1 of 3 reviewed\nFile: feedback/sent-01.json\n",
				"\nFeedback:\n",
				"\nAlpha (a)\n  notes: looks fine\n  verdict: good\n  edited draft: new text\n",
				"\nBeta (b)\n  verdict: bad\n",
			},
			notWant: []string{"Changed since", "Unchanged", "Approved actions"},
		},
		{
			name: "flagged progress counts flagged items",
			cur: obj{
				"suite": "Kitchen sink", "sequence": 1, "sent_at": "2026-11-09T12:00:00Z",
				"progress": obj{"reviewed": 1, "total": 2, "flagged": true},
				"items":    obj{"a": item("Alpha", obj{"verdict": "good"})},
			},
			want:    []string{"Kitchen sink · sent #1 at 2026-11-09T12:00:00Z · 1 of 2 flagged reviewed\n"},
			notWant: []string{"of 2 reviewed"},
		},
		{
			name:    "second send prints only changed keys",
			cur:     snapshot(2, obj{"a": item("Alpha", obj{"verdict": "bad", "notes": "looks fine"})}),
			prev:    snapshot(1, obj{"a": item("Alpha", obj{"verdict": "good", "notes": "looks fine"})}),
			want:    []string{"\nChanged since #1:\n", "\nAlpha (a)\n  verdict: bad\n"},
			notWant: []string{"notes:", "Feedback:"},
		},
		{
			name:    "removed answer prints cleared",
			cur:     snapshot(2, obj{"a": item("Alpha", obj{"verdict": "good"})}),
			prev:    snapshot(1, obj{"a": item("Alpha", obj{"verdict": "good", "notes": "x"})}),
			want:    []string{"\nAlpha (a)\n  notes: cleared\n"},
			notWant: []string{"verdict:"},
		},
		{
			name: "changed proposal prints the proposed value",
			cur: snapshot(1, obj{"a": obj{
				"title": "Alpha", "revision": "r1",
				"answers": obj{"risk": "high"},
				"changed": obj{"risk": obj{"from": "medium", "to": "high"}},
			}}),
			want: []string{"  risk: high (proposed medium)\n"},
		},
		{
			name: "multi-line text is indented 4 spaces",
			cur:  snapshot(1, obj{"a": item("Alpha", obj{"notes": "first line\nsecond line\n"})}),
			want: []string{"  notes: \n    first line\n    second line\n"},
		},
		{
			name: "row maps",
			cur:  snapshot(1, obj{"a": item("Alpha", obj{"keep": obj{"o6": true, "o4": true}, "best": obj{"f1": "x"}})}),
			want: []string{"  keep: o4, o6\n", "  best: f1=x\n"},
		},
		{
			name: "revision change",
			cur:  snapshot(2, obj{"a": obj{"title": "Alpha", "revision": "r2", "answers": obj{}}}),
			prev: snapshot(1, obj{"a": item("Alpha", obj{})}),
			want: []string{"  revision: r2 (was r1; earlier answers moved to history)\n"},
		},
		{
			name: "approved actions",
			cur:  snapshot(1, obj{"a": item("Alpha", obj{"post": true})}, obj{"item": "a", "action": "post", "title": "Alpha"}),
			want: []string{"\nApproved actions. Perform each, then record it with `evalkit done`:\n  post · Alpha (a)\n"},
		},
		{
			name: "actions already reported are listed apart",
			cur: snapshot(2, obj{"a": item("Alpha", obj{"post": true, "merge": true})},
				obj{"item": "a", "action": "post", "title": "Alpha"},
				obj{"item": "a", "action": "merge", "title": "Alpha"}),
			prev: snapshot(1, obj{"a": item("Alpha", obj{"post": true})},
				obj{"item": "a", "action": "post", "title": "Alpha"}),
			want: []string{
				"\nApproved actions. Perform each, then record it with `evalkit done`:\n  merge · Alpha (a)\n",
				"\nStill approved from earlier sends (already reported; record them with `evalkit done` once performed):\n  post · Alpha (a)\n",
			},
			notWant: []string{"`evalkit done`:\n  post"},
		},
		{
			name:    "only already reported actions omit the approved header",
			cur:     snapshot(2, obj{"a": item("Alpha", obj{"post": true})}, obj{"item": "a", "action": "post", "title": "Alpha"}),
			prev:    snapshot(1, obj{"a": item("Alpha", obj{"post": true})}, obj{"item": "a", "action": "post", "title": "Alpha"}),
			want:    []string{"\nStill approved from earlier sends (already reported; record them with `evalkit done` once performed):\n  post · Alpha (a)\n"},
			notWant: []string{"Approved actions."},
		},
		{
			name:    "same action on another item is new",
			cur:     snapshot(2, obj{}, obj{"item": "b", "action": "post", "title": "Beta"}),
			prev:    snapshot(1, obj{}, obj{"item": "a", "action": "post", "title": "Alpha"}),
			want:    []string{"\nApproved actions. Perform each, then record it with `evalkit done`:\n  post · Beta (b)\n"},
			notWant: []string{"Still approved"},
		},
		{
			name:    "item missing from the new send prints all input cleared",
			cur:     snapshot(2, obj{"a": item("Alpha", obj{"verdict": "good"})}),
			prev:    snapshot(1, obj{"a": item("Alpha", obj{"verdict": "good"}), "b": item("Beta", obj{"verdict": "bad"})}),
			want:    []string{"\nBeta (b)\n  all input cleared\n", "\nUnchanged: 1 items\n"},
			notWant: []string{"verdict:"},
		},
		{
			name: "unchanged items are counted",
			cur: snapshot(2, obj{
				"a": item("Alpha", obj{"verdict": "good"}),
				"b": item("Beta", obj{"verdict": "good"}),
				"c": item("Gamma", obj{"verdict": "bad"}),
			}),
			prev: snapshot(1, obj{
				"a": item("Alpha", obj{"verdict": "good"}),
				"b": item("Beta", obj{"verdict": "good"}),
				"c": item("Gamma", obj{"verdict": "good"}),
			}),
			want:    []string{"\nGamma (c)\n  verdict: bad\n", "\nUnchanged: 2 items\n"},
			notWant: []string{"Alpha", "Beta"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			summary.Write(&b, tc.cur, tc.prev, "feedback/sent-01.json")
			out := b.String()
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output lacks %q:\n%s", w, out)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(out, w) {
					t.Errorf("output contains %q:\n%s", w, out)
				}
			}
		})
	}
}
