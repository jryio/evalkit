package suite

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// check runs the semantic rules JSON Schema cannot express: references,
// uniqueness and keyboard conflicts. hasItemsKey tells whether the suite file
// itself has an items key, before items_from lines are merged in.
func check(s Suite, dir string, hasItemsKey bool) []issue {
	c := checker{suite: s, dir: dir, keys: map[string]string{}, items: map[string]int{}}
	runsFrom := slices.ContainsFunc(s.Runs, func(r Run) bool { return r.ItemsFrom != nil })
	if !hasItemsKey && s.ItemsFrom == nil && !runsFrom {
		c.add("a suite needs items or items_from (at the top level or in runs)")
	}
	c.suiteLevel()
	c.runs()
	for i, it := range s.Items {
		c.item(i, it)
	}
	c.runItems()
	return c.issues
}

type checker struct {
	suite  Suite
	dir    string
	keys   map[string]string // suite-level key -> owner
	items  map[string]int    // item id in the loaded suite -> index of its first item
	issues []issue
}

// add and warn copy ptr: callers build it with append, which may share a backing array.
func (c *checker) add(msg string, ptr ...string) {
	c.issues = append(c.issues, issue{ptr: slices.Clone(ptr), msg: msg})
}

func (c *checker) warn(msg string, ptr ...string) {
	c.issues = append(c.issues, issue{ptr: slices.Clone(ptr), msg: msg, warning: true})
}

func idx(i int) string { return strconv.Itoa(i) }

func (c *checker) suiteLevel() {
	s := c.suite
	groups := map[string]bool{}
	for i, g := range s.Groups {
		if groups[g.ID] {
			c.add(fmt.Sprintf("duplicate group id %q", g.ID), "groups", idx(i), "id")
		}
		groups[g.ID] = true
	}

	for k, owner := range ReservedKeys {
		c.keys[k] = "the kit (" + owner + ")"
	}
	ids := map[string]bool{}
	var primary, status []string
	for i, q := range s.Questions {
		at := []string{"questions", idx(i)}
		if ids[q.ID] {
			c.add(fmt.Sprintf("duplicate question id %q", q.ID), append(at, "id")...)
		}
		ids[q.ID] = true
		if q.Primary {
			primary = append(primary, q.ID)
		}
		if q.Status {
			status = append(status, q.ID)
		}
		for _, g := range q.Groups {
			if !groups[g] {
				c.add(fmt.Sprintf("unknown group %q (want one of %s)", g, list(groupIDs(s.Groups))), append(at, "groups")...)
			}
		}
		c.options(q, at)
		c.questionRange(q, at)
	}
	if len(primary) > 1 {
		c.add(fmt.Sprintf("only one question can be primary, got %s", list(primary)), "questions")
	}
	if len(status) > 1 {
		c.add(fmt.Sprintf("only one question can drive status, got %s", list(status)), "questions")
	}
	if len(primary) == 1 {
		for d := 1; d <= 9; d++ {
			c.keys[strconv.Itoa(d)] = "the primary question " + strconv.Quote(primary[0])
		}
	}
	byID := map[string]Question{}
	for _, q := range s.Questions {
		byID[q.ID] = q
	}
	for i, q := range s.Questions {
		c.questionKeys(q, []string{"questions", idx(i)}, c.keys)
		c.with(q, s.Questions, []string{"questions", idx(i)})
		c.when(q, byID, []string{"questions", idx(i), "when"})
	}

	for i, id := range s.Reviewed {
		if !ids[id] {
			c.add(fmt.Sprintf("unknown question %q", id), "reviewed", idx(i))
		}
	}
	c.columns(ids)
	for kind, file := range s.Renderers {
		at := []string{"renderers", kind}
		if slices.Contains(BuiltinKinds, kind) {
			c.add(fmt.Sprintf("%q is a built-in kind; pick another name", kind), at...)
		}
		if !strings.HasSuffix(file, ".js") {
			c.add(fmt.Sprintf("renderer module %q must be a .js file", file), at...)
		} else if _, err := os.Stat(filepath.Join(c.dir, file)); err != nil {
			c.add(fmt.Sprintf("renderer module %q not found next to the suite", file), at...)
		}
	}
}

// columns checks the table view's value columns against each other and the
// suite's question ids.
func (c *checker) columns(questions map[string]bool) {
	seen := map[string]bool{}
	primary := ""
	for i, col := range c.suite.Columns {
		at := []string{"columns", idx(i)}
		if col.Primary && primary != "" {
			c.add(fmt.Sprintf("only one column can be primary; %q already is", primary), append(at, "primary")...)
		}
		if col.Primary && primary == "" {
			primary = col.ID
		}
		if seen[col.ID] {
			c.add(fmt.Sprintf("duplicate column id %q", col.ID), append(at, "id")...)
		}
		seen[col.ID] = true
		if questions[col.ID] {
			c.add(fmt.Sprintf("column %q has the same id as a question", col.ID), append(at, "id")...)
		}
		if col.Min != nil && col.Max != nil && *col.Min >= *col.Max {
			c.add("min must be below max", append(at, "min")...)
		}
	}
}

// values checks that an item sets only the suite's columns.
func (c *checker) values(values map[string]any, at []string) {
	if values == nil {
		return
	}
	cols := c.suite.Columns
	if len(cols) == 0 {
		c.add("values set but the suite defines no columns", at...)
		return
	}
	for _, k := range slices.Sorted(maps.Keys(values)) {
		if !slices.ContainsFunc(cols, func(col Column) bool { return col.ID == k }) {
			c.add(fmt.Sprintf("unknown column %q (want one of %s)", k, list(columnIDs(cols))), append(slices.Clone(at), k)...)
		}
	}
}

// runs checks the runs and the dimensions they vary along.
func (c *checker) runs() {
	s := c.suite
	dims := map[string]bool{}
	for i, d := range s.Dimensions {
		at := []string{"dimensions", idx(i)}
		if dims[d.ID] {
			c.add(fmt.Sprintf("duplicate dimension id %q", d.ID), append(at, "id")...)
		}
		dims[d.ID] = true
		order := map[string]bool{}
		for j, v := range d.Order {
			k := fmt.Sprint(v)
			if order[k] {
				c.add(fmt.Sprintf("duplicate value %q in order", k), append(at, "order", idx(j))...)
			}
			order[k] = true
		}
	}
	ids := map[string]bool{}
	baseline := -1
	for i, r := range s.Runs {
		at := []string{"runs", idx(i)}
		if ids[r.ID] {
			c.add(fmt.Sprintf("duplicate run id %q", r.ID), append(at, "id")...)
		}
		ids[r.ID] = true
		switch {
		case r.Baseline && baseline >= 0:
			c.add(fmt.Sprintf("only one run can be the baseline; runs[%d] %q already is", baseline, s.Runs[baseline].ID), append(at, "baseline")...)
		case r.Baseline:
			baseline = i
		}
		if s.Dimensions != nil {
			for _, k := range slices.Sorted(maps.Keys(r.Dims)) {
				if !dims[k] {
					c.warn(fmt.Sprintf("unknown dimension %q (want one of %s)", k, list(dimensionIDs(s.Dimensions))), append(at, "dims", k)...)
				}
			}
		}
		c.values(r.Values, append(at, "values"))
	}
}

// runItems checks items against the suite's runs.
func (c *checker) runItems() {
	s := c.suite
	declared := map[string]bool{}
	for _, r := range s.Runs {
		declared[r.ID] = true
	}
	inRuns := false
	for i, it := range s.Items {
		if it.Run == "" {
			continue
		}
		inRuns = true
		if s.Runs != nil && !declared[it.Run] {
			c.warn(fmt.Sprintf("run %q is not declared under runs", it.Run), "items", idx(i), "run")
			declared[it.Run] = true
		}
	}
	if s.Runs != nil || inRuns {
		c.inlineBaseline()
	}
	if s.View == "runs" && len(s.Runs) == 0 && !inRuns {
		c.warn("view: runs needs runs; the table shows instead", "view")
	}
}

// inlineBaseline warns once, at the first baseline an item sets: in a suite with
// runs, the baseline run supplies it.
func (c *checker) inlineBaseline() {
	const msg = "baseline is ignored when the suite has runs: the baseline run supplies it"
	for i, it := range c.suite.Items {
		at := []string{"items", idx(i)}
		for _, k := range slices.Sorted(maps.Keys(it.Values)) {
			if box, ok := it.Values[k].(map[string]any); ok {
				if _, ok := box["baseline"]; ok {
					c.warn(msg, append(at, "values", k, "baseline")...)
					return
				}
			}
		}
		for _, q := range slices.Sorted(maps.Keys(it.Proposed)) {
			if it.Proposed[q].Baseline != nil {
				c.warn(msg, append(at, "proposed", q, "baseline")...)
				return
			}
		}
	}
}

// duplicateItem describes an item whose id in the loaded suite repeats an
// earlier item's.
func duplicateItem(it Item) string {
	switch {
	case it.Run == "":
		return fmt.Sprintf("duplicate item id %q", it.ID)
	case it.Rep > 0:
		return fmt.Sprintf("duplicate item %q rep %d in run %q", it.ID, it.Rep, it.Run)
	}
	return fmt.Sprintf("duplicate item %q in run %q", it.ID, it.Run)
}

// options checks option values and keys within one question.
func (c *checker) options(q Question, at []string) {
	values := map[string]bool{}
	for j, o := range q.Options {
		v := fmt.Sprint(o.Value)
		if values[v] {
			c.add(fmt.Sprintf("duplicate option value %q", v), append(at, "options", idx(j), "value")...)
		}
		values[v] = true
	}
	if q.Kind == "choice" && q.Primary && len(q.Options) > 9 {
		c.add(fmt.Sprintf("a primary question can have at most 9 options, got %d", len(q.Options)), append(at, "options")...)
	}
	if q.Status && q.Kind != "choice" {
		c.add("status needs a choice question", append(at, "status")...)
	}
}

// questionKeys claims a question's keys in taken, reporting conflicts.
func (c *checker) questionKeys(q Question, at []string, taken map[string]string) {
	claim := func(key, owner string, ptr []string) {
		if key == "" {
			return
		}
		if prev, ok := taken[key]; ok {
			c.add(fmt.Sprintf("key %q is already used by %s", key, prev), ptr...)
			return
		}
		taken[key] = owner
	}
	claim(q.Key, "question "+strconv.Quote(q.ID), append(at, "key"))
	for j, o := range q.Options {
		claim(o.Key, fmt.Sprintf("option %q of question %q", fmt.Sprint(o.Value), q.ID), append(at, "options", idx(j), "key"))
	}
}

func (c *checker) with(q Question, scope []Question, at []string) {
	if q.With == "" {
		return
	}
	i := slices.IndexFunc(scope, func(o Question) bool { return o.ID == q.With })
	switch {
	case q.With == q.ID:
		c.add("a question cannot render with itself", append(at, "with")...)
	case i < 0:
		c.add(fmt.Sprintf("unknown question %q", q.With), append(at, "with")...)
	case scope[i].With != "":
		c.add(fmt.Sprintf("question %q already renders with %q", q.With, scope[i].With), append(at, "with")...)
	case q.Each != "":
		c.add("a per-row question cannot render with another question", append(at, "with")...)
	case scope[i].Kind == "text":
		c.add(fmt.Sprintf("cannot render with %q: it is a text question", q.With), append(at, "with")...)
	case scope[i].Each != "":
		c.add(fmt.Sprintf("cannot render with %q: it is asked per row", q.With), append(at, "with")...)
	}
}

func (c *checker) item(i int, it Item) {
	s := c.suite
	at := []string{"items", idx(i)}
	if j, dup := c.items[it.fullID()]; dup {
		c.add(fmt.Sprintf("%s (first at items[%d])", duplicateItem(it), j), append(at, "id")...)
	} else {
		c.items[it.fullID()] = i
	}
	switch {
	case len(s.Groups) > 0 && it.Group == "":
		c.add(fmt.Sprintf("missing group (want one of %s)", list(groupIDs(s.Groups))), at...)
	case len(s.Groups) > 0 && !slices.ContainsFunc(s.Groups, func(g Group) bool { return g.ID == it.Group }):
		c.add(fmt.Sprintf("unknown group %q (want one of %s)", it.Group, list(groupIDs(s.Groups))), append(at, "group")...)
	case len(s.Groups) == 0 && it.Group != "":
		c.add("group set but the suite defines no groups", append(at, "group")...)
	}
	c.values(it.Values, append(slices.Clone(at), "values"))

	artifacts := map[string]located{}
	editable := 0
	keys := maps.Clone(c.keys)
	for _, section := range []struct {
		name string
		list []Artifact
	}{{"show", it.Show}, {"context", it.Context}} {
		for j, a := range section.list {
			ap := append(slices.Clone(at), section.name, idx(j))
			if !slices.Contains(BuiltinKinds, a.Kind) && s.Renderers[a.Kind] == "" {
				c.add(fmt.Sprintf("unknown kind %q (want one of %s, or declare it under renderers)", a.Kind, list(BuiltinKinds)), append(ap, "kind")...)
			}
			if a.ID != "" {
				if _, dup := artifacts[a.ID]; dup {
					c.add(fmt.Sprintf("duplicate artifact id %q", a.ID), append(ap, "id")...)
				}
				artifacts[a.ID] = located{a, ap}
			}
			if a.Editable {
				editable++
				if editable > 1 {
					c.add("only one artifact per item can be editable", append(ap, "editable")...)
				}
				key := a.Key
				if key == "" {
					key = "e"
				}
				if prev, ok := keys[key]; ok {
					c.add(fmt.Sprintf("edit key %q is already used by %s", key, prev), append(ap, "key")...)
				}
				keys[key] = "the editable artifact"
			}
			c.rowIDs(a, ap)
		}
	}

	applicable := map[string]Question{}
	var asked []Question // applicable, in order
	for _, q := range s.Questions {
		if len(q.Groups) == 0 || slices.Contains(q.Groups, it.Group) {
			applicable[q.ID] = q
			asked = append(asked, q)
		}
	}
	labels := map[string]bool{}
	for j, q := range it.Ask {
		qp := append(slices.Clone(at), "ask", idx(j))
		if _, dup := applicable[q.ID]; dup {
			c.add(fmt.Sprintf("duplicate question id %q (already asked)", q.ID), append(qp, "id")...)
		}
		if q.Primary || q.Status {
			c.add("primary and status are suite-level settings", qp...)
		}
		if l := normalizeLabel(q.Label); l != "" && labels[l] {
			c.warn("duplicate question label; dropped", append(qp, "label")...)
			continue
		}
		labels[normalizeLabel(q.Label)] = true
		applicable[q.ID] = q
		asked = append(asked, q)
		c.options(q, qp)
		c.questionRange(q, qp)
		c.questionKeys(q, qp, keys)
		c.with(q, it.Ask, qp)
		c.rowSource(q.Each, artifacts, append(qp, "each"))
		c.rowSource(q.From, artifacts, append(qp, "from"))
	}
	for j, q := range it.Ask {
		c.when(q, applicable, append(slices.Clone(at), "ask", idx(j), "when"))
	}
	for i, q := range s.Questions {
		if applicable[q.ID].ID == "" {
			continue
		}
		// Suite-level per-row questions only apply to items that have the artifact.
		for _, ref := range []string{q.Each, q.From} {
			if a, ok := artifacts[ref]; ok && a.Rows == nil {
				c.add(fmt.Sprintf("question %q (questions[%d]) needs rows, but artifact %q has none", q.ID, i, ref), at...)
			}
		}
	}
	c.rowsHaveIDs(asked, artifacts)

	for _, qid := range slices.Sorted(maps.Keys(it.Proposed)) {
		pp := append(slices.Clone(at), "proposed", qid)
		q, ok := applicable[qid]
		if !ok {
			c.add(fmt.Sprintf("unknown question %q for this item", qid), pp...)
			continue
		}
		if q.Kind == "action" {
			c.add("an action cannot be proposed; the human approves it", pp...)
			continue
		}
		if b := it.Proposed[qid].Baseline; b != nil {
			if q.Each != "" {
				c.add("baseline is not supported for per-row questions", append(pp, "baseline")...)
			} else {
				c.proposal(q, b, append(pp, "baseline"))
			}
		}
		v := it.Proposed[qid].Value
		if q.Each == "" {
			c.proposal(q, v, append(pp, "value"))
			continue
		}
		rows, ok := v.(map[string]any)
		if !ok {
			c.add(fmt.Sprintf("%q is asked per row, so its proposal must map row ids to values", qid), append(pp, "value")...)
			continue
		}
		a, hasArtifact := artifacts[q.Each]
		ids := map[string]bool{}
		for _, r := range a.Rows {
			ids[rowID(r)] = true
		}
		for _, row := range slices.Sorted(maps.Keys(rows)) {
			rp := append(slices.Clone(pp), "value", row)
			if hasArtifact && !ids[row] {
				c.add(fmt.Sprintf("unknown row %q in artifact %q", row, q.Each), rp...)
				continue
			}
			c.proposal(q, rows[row], rp)
		}
	}
}

// located is an artifact with its pointer in the suite.
type located struct {
	Artifact
	at []string
}

// rowsHaveIDs checks that every artifact a question asks about row by row gives
// each row an id: answers are keyed by row id.
func (c *checker) rowsHaveIDs(asked []Question, artifacts map[string]located) {
	checked := map[string]bool{}
	for _, q := range asked {
		for _, ref := range []string{q.Each, q.From} {
			a, ok := artifacts[ref]
			if !ok || checked[ref] {
				continue
			}
			checked[ref] = true
			for k, r := range a.Rows {
				if rowID(r) == "" {
					c.add(fmt.Sprintf("row needs an id: question %q asks about each row", q.ID), append(slices.Clone(a.at), "rows", idx(k))...)
				}
			}
		}
	}
}

func (c *checker) proposal(q Question, v any, at []string) {
	switch q.Kind {
	case "choice":
		c.option(q, v, at)
	case "multi":
		vs, ok := v.([]any)
		if !ok {
			c.add(fmt.Sprintf("%q takes several options, so its proposal must be a list of option values", q.ID), at...)
			return
		}
		for k, e := range vs {
			c.option(q, e, append(slices.Clone(at), idx(k)))
		}
	case "scale", "number":
		c.number(q, v, at)
	}
}

// bounds returns a scale or number question's range; nil is unbounded.
func bounds(q Question) (lo, hi *float64) {
	if q.Kind == "scale" {
		return cmp.Or(q.Min, new(1.0)), cmp.Or(q.Max, new(5.0))
	}
	return q.Min, q.Max
}

// questionRange checks that a scale or number question's range is not empty.
func (c *checker) questionRange(q Question, at []string) {
	if q.Kind != "scale" && q.Kind != "number" {
		return
	}
	if lo, hi := bounds(q); lo != nil && hi != nil && *lo >= *hi {
		c.add("min must be below max", append(at, "min")...)
	}
}

func (c *checker) option(q Question, v any, at []string) {
	if !slices.ContainsFunc(q.Options, func(o Option) bool { return reflect.DeepEqual(o.Value, v) }) {
		c.add(fmt.Sprintf("proposed value %v is not an option of %q (want one of %s)", v, q.ID, list(optionValues(q))), at...)
	}
}

// number checks v is a number within the question's bounds.
func (c *checker) number(q Question, v any, at []string) {
	lo, hi := bounds(q)
	n, ok := v.(float64)
	switch {
	case !ok:
		c.add(fmt.Sprintf("proposed value %v for %q must be a number", v, q.ID), at...)
	case lo != nil && n < *lo:
		c.add(fmt.Sprintf("proposed value %v for %q is below the minimum %v", v, q.ID, *lo), at...)
	case hi != nil && n > *hi:
		c.add(fmt.Sprintf("proposed value %v for %q is above the maximum %v", v, q.ID, *hi), at...)
	}
}

// when checks that a condition names a question in scope and valid option values.
func (c *checker) when(q Question, scope map[string]Question, at []string) {
	for _, qid := range slices.Sorted(maps.Keys(q.When)) {
		target, ok := scope[qid]
		switch {
		case !ok:
			c.add(fmt.Sprintf("unknown question %q", qid), append(at, qid)...)
		case qid == q.ID:
			c.add("a question cannot depend on itself", append(at, qid)...)
		case target.Each != q.Each:
			c.add(fmt.Sprintf("%q and %q must both be per-row on the same artifact, or both not", q.ID, qid), append(at, qid)...)
		case len(target.Options) > 0:
			for j, v := range q.When[qid] {
				if !slices.ContainsFunc(target.Options, func(o Option) bool { return reflect.DeepEqual(o.Value, v) }) {
					c.add(fmt.Sprintf("%v is not an option of %q (want one of %s)", v, qid, list(optionValues(target))), append(at, qid, idx(j))...)
				}
			}
		}
	}
}

// rowSource checks that an item question's each/from names an artifact with rows.
func (c *checker) rowSource(ref string, artifacts map[string]located, at []string) {
	if ref == "" {
		return
	}
	if a, ok := artifacts[ref]; !ok {
		c.add(fmt.Sprintf("unknown artifact %q", ref), at...)
	} else if a.Rows == nil {
		c.add(fmt.Sprintf("artifact %q has no rows", ref), at...)
	}
}

func (c *checker) rowIDs(a Artifact, at []string) {
	seen := map[string]bool{}
	for j, r := range a.Rows {
		id := rowID(r)
		if id == "" {
			continue
		}
		if seen[id] {
			c.add(fmt.Sprintf("duplicate row id %q", id), append(at, "rows", idx(j), "id")...)
		}
		seen[id] = true
	}
}

func rowID(r any) string {
	m, _ := r.(map[string]any)
	id, _ := m["id"].(string)
	return id
}

func groupIDs(gs []Group) []string {
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.ID
	}
	return out
}

func dimensionIDs(ds []Dimension) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.ID
	}
	return out
}

func columnIDs(cs []Column) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func optionValues(q Question) []string {
	out := make([]string, len(q.Options))
	for i, o := range q.Options {
		out[i] = fmt.Sprint(o.Value)
	}
	return out
}

func list(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = strconv.Quote(x)
	}
	return strings.Join(q, ", ")
}
