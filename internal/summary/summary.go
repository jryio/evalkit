// Package summary prints what changed between two snapshots, so an agent reads
// only the new feedback instead of the whole file.
package summary

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// Write prints snapshot cur, diffed against prev (nil for the first send).
func Write(w io.Writer, cur, prev map[string]any, path string) {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	progress, _ := cur["progress"].(map[string]any)
	p("%v · sent #%v at %v · %v of %v reviewed\n", cur["suite"], cur["sequence"], cur["sent_at"], progress["reviewed"], progress["total"])
	p("File: %s\n", path)

	writeActions(w, cur, prev)

	curItems, _ := cur["items"].(map[string]any)
	prevItems, _ := prev["items"].(map[string]any)
	if prev != nil {
		p("\nChanged since #%v:\n", prev["sequence"])
	} else {
		p("\nFeedback:\n")
	}
	unchanged := 0
	for _, id := range slices.Sorted(maps.Keys(union(curItems, prevItems))) {
		old, _ := prevItems[id].(map[string]any)
		if _, ok := curItems[id]; !ok {
			p("\n%v (%s)\n  all input cleared\n", old["title"], id)
			continue
		}
		c, _ := curItems[id].(map[string]any)
		lines := itemDiff(c, old)
		if len(lines) == 0 {
			unchanged++
			continue
		}
		p("\n%v (%s)\n", c["title"], id)
		for _, l := range lines {
			p("%s\n", l)
		}
	}
	if unchanged > 0 {
		p("\nUnchanged: %d items\n", unchanged)
	}
}

// writeActions lists approved actions, separating those prev already reported so
// the agent does not perform them twice.
func writeActions(w io.Writer, cur, prev map[string]any) {
	reported := map[[2]string]bool{}
	for _, a := range actions(prev) {
		reported[actionKey(a)] = true
	}
	var fresh, earlier []map[string]any
	for _, a := range actions(cur) {
		if reported[actionKey(a)] {
			earlier = append(earlier, a)
		} else {
			fresh = append(fresh, a)
		}
	}
	listActions(w, "Approved actions. Perform each, then record it with `evalkit done`:", fresh)
	listActions(w, "Still approved from earlier sends (already reported; record them with `evalkit done` once performed):", earlier)
}

func actions(snap map[string]any) []map[string]any {
	list, _ := snap["actions"].([]any)
	out := make([]map[string]any, len(list))
	for i, a := range list {
		out[i], _ = a.(map[string]any)
	}
	return out
}

func actionKey(a map[string]any) [2]string {
	return [2]string{fmt.Sprint(a["item"]), fmt.Sprint(a["action"])}
}

func listActions(w io.Writer, header string, list []map[string]any) {
	if len(list) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n", header)
	for _, a := range list {
		fmt.Fprintf(w, "  %v · %v (%v)\n", a["action"], a["title"], a["item"])
	}
}

func itemDiff(cur, prev map[string]any) []string {
	var lines []string
	if prev != nil && cur["revision"] != prev["revision"] {
		lines = append(lines, fmt.Sprintf("  revision: %v (was %v; earlier answers moved to history)", cur["revision"], prev["revision"]))
	}
	for _, field := range []string{"answers", "edits"} {
		c, _ := cur[field].(map[string]any)
		o, _ := prev[field].(map[string]any)
		changed, _ := cur["changed"].(map[string]any)
		for _, k := range slices.Sorted(maps.Keys(union(c, o))) {
			v, had := c[k]
			if reflect.DeepEqual(v, o[k]) {
				continue
			}
			label := k
			if field == "edits" {
				label = "edited " + k
			}
			if !had {
				lines = append(lines, fmt.Sprintf("  %s: cleared", label))
				continue
			}
			text := format(v)
			if ch, ok := changed[k].(map[string]any); ok {
				if from, scalar := ch["from"]; scalar {
					text += fmt.Sprintf(" (proposed %s)", format(from))
				} else {
					text += " (overrides proposals for " + strings.Join(slices.Sorted(maps.Keys(ch)), ", ") + ")"
				}
			}
			lines = append(lines, "  "+label+": "+text)
		}
	}
	return lines
}

func union(a, b map[string]any) map[string]any {
	out := maps.Clone(a)
	if out == nil {
		out = map[string]any{}
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// format prints scalars plainly, lists comma-separated, row maps as key=value
// and multi-line text as an indented block.
func format(v any) string {
	switch x := v.(type) {
	case string:
		if strings.Contains(x, "\n") {
			return "\n    " + strings.ReplaceAll(strings.TrimRight(x, "\n"), "\n", "\n    ")
		}
		return x
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = format(e)
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		var parts []string
		for _, k := range slices.Sorted(maps.Keys(x)) {
			if x[k] == true {
				parts = append(parts, k)
			} else {
				parts = append(parts, k+"="+format(x[k]))
			}
		}
		return strings.Join(parts, ", ")
	case nil:
		return "—"
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
