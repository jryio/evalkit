package stats

import (
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Suite is the part of a loaded suite the score needs.
type Suite struct {
	Questions []Question `json:"questions"`
	Columns   []Column   `json:"columns"`
	Runs      []Run      `json:"runs"`
	Items     []Item     `json:"items"`
}

type Question struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Label   string   `json:"label"`
	Primary bool     `json:"primary"`
	Options []Option `json:"options"`
}

type Option struct {
	Value any    `json:"value"`
	Tone  string `json:"tone"`
}

type Column struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Unit    string   `json:"unit"`
	Better  string   `json:"better"`
	Noise   float64  `json:"noise"`
	Primary bool     `json:"primary"`
	Min     *float64 `json:"min"`
	Max     *float64 `json:"max"`
}

type Run struct {
	ID       string         `json:"id"`
	Label    string         `json:"label"`
	Baseline bool           `json:"baseline"`
	Values   map[string]any `json:"values"`
}

type Item struct {
	ID       string              `json:"id"`
	Run      string              `json:"run"`
	Rep      int                 `json:"rep"`
	Pair     string              `json:"pair"`
	Group    string              `json:"group"`
	Proposed map[string]Proposal `json:"proposed"`
	Values   map[string]any      `json:"values"`
}

type Proposal struct {
	Value any `json:"value"`
}

// Decode reads a loaded suite document (suite.Result.Doc).
func Decode(doc any) (Suite, error) {
	var s Suite
	b, err := json.Marshal(doc)
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// Metric is what ranks runs.
type Metric struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Source string  `json:"source"` // question or column
	Kind   string  `json:"kind"`   // rate or mean
	Better string  `json:"better"` // higher or lower
	Noise  float64 `json:"noise,omitempty"`
	q      *Question
}

// MetricOf is the page's rule: the primary column, else the primary question
// when it scores, else the first bounded or directed column.
func MetricOf(s Suite) (Metric, bool) {
	primaryColumn := slices.IndexFunc(s.Columns, func(c Column) bool { return c.Primary })
	if primaryColumn < 0 {
		for i, q := range s.Questions {
			if !q.Primary {
				continue
			}
			kind := ""
			switch q.Kind {
			case "choice", "toggle":
				kind = "rate"
			case "number", "scale":
				kind = "mean"
			}
			if kind != "" {
				return Metric{ID: q.ID, Label: label(q.Label, q.ID), Source: "question", Kind: kind, Better: "higher", q: &s.Questions[i]}, true
			}
			break
		}
	}
	i := primaryColumn
	if i < 0 {
		i = slices.IndexFunc(s.Columns, func(c Column) bool { return c.Min != nil && c.Max != nil })
	}
	if i < 0 {
		i = slices.IndexFunc(s.Columns, func(c Column) bool { return c.Better != "" })
	}
	if i < 0 {
		return Metric{}, false
	}
	c := s.Columns[i]
	better := c.Better
	if better == "" {
		better = "higher"
	}
	return Metric{ID: c.ID, Label: label(c.Label, c.ID), Source: "column", Kind: "mean", Better: better, Noise: c.Noise}, true
}

var tokenRe = regexp.MustCompile(`(?i)token`)

// spendOf is the column that says what a run cost: dollars, else tokens, else seconds.
func spendOf(s Suite, m Metric) (Column, bool) {
	var cols []Column
	for _, c := range s.Columns {
		if !(m.Source == "column" && c.ID == m.ID) {
			cols = append(cols, c)
		}
	}
	for _, match := range []func(Column) bool{
		func(c Column) bool { return c.Unit == "$" },
		func(c Column) bool { return tokenRe.MatchString(c.ID + " " + c.Label) },
		func(c Column) bool { return c.Unit == "s" },
	} {
		if i := slices.IndexFunc(cols, match); i >= 0 {
			return cols[i], true
		}
	}
	return Column{}, false
}

func label(l, id string) string {
	if l != "" {
		return l
	}
	return id
}

// unbox reads a value that may be {value, baseline}.
func unbox(v any) any {
	if m, ok := v.(map[string]any); ok {
		return m["value"]
	}
	return v
}

func number(v any) (float64, bool) {
	f, ok := unbox(v).(float64)
	return f, ok && !math.IsNaN(f) && !math.IsInf(f, 0)
}

var toneScore = map[string]float64{"positive": 1, "warning": 0.5, "negative": 0}

// value is the item's metric value from its proposal or its measured values;
// ok is false when the item does not count.
func (m Metric) value(it Item, runs map[string]Run) (float64, bool) {
	if m.Source == "column" {
		if v, ok := number(it.Values[m.ID]); ok {
			return v, true
		}
		return number(runs[it.Run].Values[m.ID])
	}
	p, ok := it.Proposed[m.ID]
	if !ok {
		return 0, false
	}
	if m.Kind == "mean" {
		return number(p.Value)
	}
	if m.q.Kind == "toggle" {
		b, ok := p.Value.(bool)
		if !ok {
			return 0, false
		}
		if b {
			return 1, true
		}
		return 0, true
	}
	for _, o := range m.q.Options {
		if o.Value == p.Value {
			score, ok := toneScore[o.Tone]
			return score, ok
		}
	}
	return 0, false
}

// Versus is a run paired, case by case, with the run it is compared with.
type Versus struct {
	Run       string   `json:"run"`
	Paired    int      `json:"paired"`
	N         int      `json:"n"`
	Delta     Interval `json:"delta"`
	Up        int      `json:"up"`
	Same      int      `json:"same"`
	Down      int      `json:"down"`
	P         *float64 `json:"p,omitempty"` // McNemar's exact test, for a rate
	Improved  []string `json:"improved"`
	Regressed []string `json:"regressed"`
	// Significant: the interval of the change does not cross zero.
	Significant bool `json:"significant"`
}

// RunSummary is one run's score.
type RunSummary struct {
	Run      string    `json:"run"`
	Label    string    `json:"label,omitempty"`
	Baseline bool      `json:"baseline,omitempty"`
	Items    int       `json:"items"`
	N        int       `json:"n"` // items that count toward the score
	Score    *Interval `json:"score"`
	Spend    *float64  `json:"spend,omitempty"` // mean per item
	Versus   *Versus   `json:"versus,omitempty"`
}

// Report is every run's score under one metric.
type Report struct {
	Metric *Metric      `json:"metric"`
	Spend  string       `json:"spend,omitempty"` // the spend column's id
	Groups []string     `json:"groups,omitempty"`
	Runs   []RunSummary `json:"runs"`
}

// Summarize scores each run in run order. groups limits the items to those
// groups; nil takes every item. Each run is compared with the baseline run,
// else with the run before it.
func Summarize(s Suite, groups []string, seed uint32) Report {
	rep := Report{Groups: groups, Runs: []RunSummary{}}
	metric, hasMetric := MetricOf(s)
	if hasMetric {
		rep.Metric = &metric
	}
	spend, hasSpend := spendOf(s, metric)
	if hasSpend {
		rep.Spend = spend.ID
	}
	runs := runList(s)
	byID := map[string]Run{}
	for _, r := range runs {
		byID[r.ID] = r
	}
	baseline := slices.IndexFunc(runs, func(r Run) bool { return r.Baseline })

	type scored struct {
		pair, kase string
		rep        int
		v          float64
	}
	itemsOf := func(run string) (items []Item, vals []scored) {
		for _, it := range s.Items {
			if it.Run != run || (groups != nil && !slices.Contains(groups, it.Group)) {
				continue
			}
			items = append(items, it)
			if !hasMetric {
				continue
			}
			if v, ok := metric.value(it, byID); ok {
				vals = append(vals, scored{pair: pairOf(it), kase: caseOf(it), rep: it.Rep, v: v})
			}
		}
		return items, vals
	}

	for i, r := range runs {
		items, vals := itemsOf(r.ID)
		row := RunSummary{Run: r.ID, Label: r.Label, Baseline: r.Baseline, Items: len(items), N: len(vals)}
		xs := make([]float64, len(vals))
		clusters := make([]string, len(vals))
		reps := false
		for j, v := range vals {
			xs[j], clusters[j] = v.v, v.kase
			reps = reps || v.rep > 0
		}
		var iv Interval
		var ok bool
		if metric.Kind == "rate" && !reps {
			var k float64
			for _, x := range xs {
				k += x
			}
			iv, ok = Wilson(k, len(xs))
		} else {
			iv, ok = BootstrapMean(xs, clusters, seed)
		}
		if ok {
			row.Score = &iv
		}
		if hasSpend {
			var sum float64
			var n int
			for _, it := range items {
				v, ok := number(it.Values[spend.ID])
				if !ok {
					v, ok = number(r.Values[spend.ID])
				}
				if ok {
					sum += v
					n++
				}
			}
			if n > 0 {
				mean := sum / float64(n)
				row.Spend = &mean
			}
		}

		vs := i - 1
		if baseline >= 0 {
			vs = baseline
		}
		if vs >= 0 && vs != i && hasMetric {
			baseItems, baseVals := itemsOf(runs[vs].ID)
			base := map[string]float64{}
			for _, v := range baseVals {
				base[v.pair] = v.v
			}
			basePairs := map[string]bool{}
			for _, it := range baseItems {
				basePairs[pairOf(it)] = true
			}
			v := Versus{Run: runs[vs].ID, Improved: []string{}, Regressed: []string{}}
			for _, it := range items {
				if basePairs[pairOf(it)] {
					v.Paired++
				}
			}
			var deltas []float64
			var keys []string
			for _, sv := range vals {
				b, ok := base[sv.pair]
				if !ok {
					continue
				}
				d := sv.v - b
				deltas, keys = append(deltas, d), append(keys, sv.kase)
				switch tone(d, metric) {
				case 1:
					v.Up++
					v.Improved = append(v.Improved, sv.pair)
				case -1:
					v.Down++
					v.Regressed = append(v.Regressed, sv.pair)
				}
			}
			if delta, ok := BootstrapMean(deltas, keys, seed); ok {
				v.N, v.Delta, v.Same = len(deltas), delta, len(deltas)-v.Up-v.Down
				v.Significant = delta.Lo > 0 || delta.Hi < 0
				if metric.Kind == "rate" {
					p := McNemar(v.Up, v.Down)
					v.P = &p
				}
				row.Versus = &v
			}
		}
		rep.Runs = append(rep.Runs, row)
	}
	return rep
}

// tone is 1 when the change is an improvement, -1 a regression, 0 within noise.
func tone(delta float64, m Metric) int {
	if math.Abs(delta) <= m.Noise || delta == 0 {
		return 0
	}
	if (m.Better == "lower") == (delta < 0) {
		return 1
	}
	return -1
}

// runList is the declared runs, then the runs only items name, first seen first.
func runList(s Suite) []Run {
	runs := slices.Clone(s.Runs)
	known := map[string]bool{}
	for _, r := range runs {
		known[r.ID] = true
	}
	for _, it := range s.Items {
		if it.Run != "" && !known[it.Run] {
			known[it.Run] = true
			runs = append(runs, Run{ID: it.Run})
		}
	}
	return runs
}

func pairOf(it Item) string {
	if it.Pair != "" {
		return it.Pair
	}
	return it.ID
}

// caseOf is the pair without its rep suffix: every rep of one case shares it.
func caseOf(it Item) string {
	pair := pairOf(it)
	if it.Rep > 0 {
		return strings.TrimSuffix(pair, ".r"+strconv.Itoa(it.Rep))
	}
	return pair
}
