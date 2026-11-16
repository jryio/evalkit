// Package suite loads, validates and normalizes an evalkit suite file.
package suite

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"
)

//go:embed suite.schema.json
var SchemaJSON []byte

var compiledSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(SchemaJSON))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("suite.schema.json", doc); err != nil {
		return nil, err
	}
	return c.Compile("suite.schema.json")
})

// Problem is one validation finding, located in the source file.
type Problem struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Col     int    `json:"col,omitempty"`
	Path    string `json:"path,omitempty"`
	Item    string `json:"item,omitempty"`
	Message string `json:"message"`
	Warning bool   `json:"warning,omitempty"`
}

func (p Problem) String() string {
	loc := p.File
	if p.Line > 0 {
		loc = fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
	}
	where := p.Path
	if p.Item != "" {
		where += " (item " + p.Item + ")"
	}
	level := "error"
	if p.Warning {
		level = "warning"
	}
	if where == "" {
		return fmt.Sprintf("%s: %s: %s", loc, level, p.Message)
	}
	return fmt.Sprintf("%s: %s: %s: %s", loc, level, where, p.Message)
}

// Result is a loaded suite. Doc is the normalized document for the UI: items with
// errors are removed and every item carries a revision. Doc is nil when Fatal.
type Result struct {
	Doc      *Map      `json:"suite,omitempty"`
	Problems []Problem `json:"problems"`
	Version  string    `json:"version"`
	Fatal    bool      `json:"fatal"`
}

// issue is a problem before it is located in the source.
type issue struct {
	ptr     []string
	msg     string
	warning bool
	echo    bool // a "false schema" left by unevaluatedProperties after another rule failed
}

// Load reads and validates the suite at path. It returns an error only when the
// file cannot be read; everything else is reported as a Problem.
func Load(path string) (Result, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	res := Result{Version: version(src, nil), Problems: []Problem{}}
	name := filepath.Base(path)
	dir := filepath.Dir(path)

	fatal := func(line, col int, msg string) (Result, error) {
		res.Problems = append(res.Problems, Problem{File: name, Line: line, Col: col, Message: msg})
		res.Fatal = true
		return res, nil
	}

	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		line, msg := yamlLine(err.Error())
		return fatal(line, 1, "YAML syntax: "+msg)
	}
	if len(root.Content) == 0 {
		return fatal(0, 0, "the file is empty")
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return fatal(root.Line, root.Column, "the top level must be a mapping")
	}
	sources, issues := readSources(top, dir)
	res.Version = version(src, sources)
	hasItemsKey := child(top, "items") != nil
	itemFiles, lineProblems := mergeItems(top, name, sources)
	res.Problems = append(res.Problems, lineProblems...)

	raw, err := fromNode(&root)
	if err != nil {
		return fatal(0, 0, "decode: "+err.Error())
	}
	doc := raw.(*Map)
	js, err := json.Marshal(doc)
	if err != nil {
		return fatal(0, 0, "encode: "+err.Error())
	}

	schema, err := compiledSchema()
	if err != nil {
		return Result{}, fmt.Errorf("compile embedded schema: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(js))
	if err != nil {
		return Result{}, err
	}
	if err := schema.Validate(inst); err != nil {
		ve, ok := errors.AsType[*jsonschema.ValidationError](err)
		if !ok {
			return Result{}, err
		}
		issues = append(issues, dropEchoes(schemaIssues(ve))...)
	}

	var typed Suite
	_ = json.Unmarshal(js, &typed) // type mismatches are already schema issues
	issues = append(issues, check(typed, dir, hasItemsKey)...)

	itemsRaw, _ := doc.Get("items")
	items, _ := itemsRaw.([]any)
	badItems := map[int]bool{}
	for _, is := range issues {
		p := Problem{File: name, Path: pathString(is.ptr), Message: is.msg, Warning: is.warning}
		if n := locate(&root, is.ptr); n != nil {
			p.Line, p.Col = n.Line, n.Column
		}
		idx, inItem := itemIndex(is.ptr)
		if inItem && idx < len(items) {
			if idx < len(itemFiles) {
				p.File = itemFiles[idx]
			}
			p.Item = typed.Items[idx].fullID()
			if !is.warning {
				badItems[idx] = true
			}
		} else if !is.warning {
			res.Fatal = true
		}
		res.Problems = append(res.Problems, p)
	}
	if res.Fatal {
		return res, nil
	}

	kept := make([]any, 0, len(items))
	var runs []string // the run of each kept item that has one
	for i, it := range items {
		if badItems[i] {
			continue
		}
		m := it.(*Map)
		if t := typed.Items[i]; t.Run != "" {
			m.Set("id", t.fullID())
			m.Set("pair", t.pair())
			runs = append(runs, t.Run)
		}
		dropDuplicateQuestions(m)
		show, _ := m.Get("show")
		m.Set("revision", revision(show))
		kept = append(kept, m)
	}
	doc.Set("items", kept)
	declareRuns(doc, typed.Runs, runs)
	res.Doc = doc
	return res, nil
}

// declareRuns appends {id} to the doc's runs for each run that items name but
// runs does not declare, in first-seen order.
func declareRuns(doc *Map, declared []Run, named []string) {
	seen := map[string]bool{}
	for _, r := range declared {
		seen[r.ID] = true
	}
	v, _ := doc.Get("runs")
	runs, _ := v.([]any)
	n := len(runs)
	for _, id := range named {
		if !seen[id] {
			seen[id] = true
			runs = append(runs, &Map{keys: []string{"id"}, vals: map[string]any{"id": id}})
		}
	}
	if len(runs) > n {
		doc.Set("runs", runs)
	}
}

// Version is Load's version without validating: a hash of the suite file and the
// JSONL files its items_from and its runs' items_from name.
func Version(path string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var root yaml.Node
	if yaml.Unmarshal(src, &root) != nil || len(root.Content) == 0 {
		return version(src, nil), nil
	}
	sources, _ := readSources(root.Content[0], filepath.Dir(path))
	return version(src, sources), nil
}

func version(src []byte, sources []source) string {
	h := sha256.New()
	h.Write(src)
	for _, s := range sources {
		h.Write([]byte(s.rel + "\x00"))
		h.Write(s.data)
	}
	return hex.EncodeToString(h.Sum(nil)[:6])
}

// source is one JSONL file named by items_from.
type source struct {
	rel      string // relative to the suite's dir, slash-separated
	data     []byte
	run      string // the run whose items_from names the file; "" for the top level
	runIndex int    // the run's index in runs
}

// readSources reads the JSONL files named under the suite's top-level
// items_from, then under each run's items_from. Files that cannot be read are
// skipped.
func readSources(top *yaml.Node, dir string) ([]source, []issue) {
	sources, issues := readItemsFrom(child(top, "items_from"), dir, []string{"items_from"}, source{})
	runs := child(top, "runs")
	if runs == nil || runs.Kind != yaml.SequenceNode {
		return sources, issues
	}
	for i, r := range runs.Content {
		id, ok := stringID(r)
		if !ok {
			continue // the schema reports it
		}
		s, is := readItemsFrom(child(r, "items_from"), dir, []string{"runs", idx(i), "items_from"}, source{run: id, runIndex: i})
		sources, issues = append(sources, s...), append(issues, is...)
	}
	return sources, issues
}

// readItemsFrom reads the files one items_from node at ptr names, each as a
// copy of owner.
func readItemsFrom(n *yaml.Node, dir string, ptr []string, owner source) ([]source, []issue) {
	if n == nil {
		return nil, nil
	}
	var itemsFrom any
	if n.Decode(&itemsFrom) != nil {
		return nil, nil
	}
	files, issues := jsonlFiles(dir, itemsFrom, ptr)
	var out []source
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			rel = f
		}
		s := owner
		s.rel, s.data = filepath.ToSlash(rel), data
		out = append(out, s)
	}
	return out, issues
}

// jsonlFiles expands items_from at ptr, a path or glob or a list of them
// relative to dir, into files in entry order. Anything else returns nothing:
// the schema reports it.
func jsonlFiles(dir string, itemsFrom any, ptr []string) (files []string, issues []issue) {
	var entries []string
	var ptrs [][]string
	switch v := itemsFrom.(type) {
	case string:
		entries, ptrs = []string{v}, [][]string{ptr}
	case []any:
		for i, e := range v {
			s, ok := e.(string)
			if !ok {
				return nil, nil
			}
			entries = append(entries, s)
			ptrs = append(ptrs, append(slices.Clone(ptr), idx(i)))
		}
	default:
		return nil, nil
	}
	seen := map[string]bool{}
	for i, entry := range entries {
		matches, err := expand(dir, entry)
		if err != nil {
			issues = append(issues, issue{ptr: ptrs[i], msg: fmt.Sprintf("bad glob pattern %q", entry)})
			continue
		}
		if len(matches) == 0 {
			issues = append(issues, issue{ptr: ptrs[i], msg: fmt.Sprintf("%q matches no file yet", entry), warning: true})
			continue
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				files = append(files, m)
			}
		}
	}
	return files, issues
}

// expand returns the files entry, a path or glob relative to dir, names, sorted.
// Only entry is a pattern: dir may contain glob meta characters.
func expand(dir, entry string) ([]string, error) {
	path := filepath.Join(dir, entry)
	if !strings.ContainsAny(entry, `*?[\`) {
		if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
			return []string{path}, nil
		}
		return nil, nil
	}
	matches, err := filepath.Glob(filepath.Join(escapeGlob(dir), entry))
	slices.Sort(matches)
	return matches, err
}

// escapeGlob quotes the glob meta characters in a literal path. A backslash is a
// separator on Windows and an escape elsewhere.
func escapeGlob(path string) string {
	var b strings.Builder
	for _, r := range path {
		switch {
		case r == '*' || r == '?' || r == '[':
			b.WriteString("[" + string(r) + "]")
		case r == '\\' && runtime.GOOS != "windows":
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// mergeItems appends the items of each JSONL source to the items sequence of the
// suite's top-level mapping, setting run on the lines a run's items_from reads.
// A later line replaces an earlier items_from line with the same (run, id, rep)
// in place. It returns the file each item in the sequence came from.
func mergeItems(top *yaml.Node, name string, sources []source) ([]string, []Problem) {
	seq := child(top, "items")
	if seq == nil {
		seq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		top.Content = append(top.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "items"}, seq)
	}
	if seq.Kind != yaml.SequenceNode {
		return nil, nil // the schema reports it
	}
	files := slices.Repeat([]string{name}, len(seq.Content))
	merged := map[string]int{} // identity -> index in seq, for items_from lines only
	var problems []Problem
	for _, src := range sources {
		nodes, ps := parseLines(src)
		problems = append(problems, ps...)
		for _, n := range nodes {
			if err := setRun(n, src); err != nil {
				problems = append(problems, Problem{File: src.rel, Line: n.Line, Col: 1, Message: err.Error()})
				continue
			}
			key, hasID := identity(n)
			if i, ok := merged[key]; hasID && ok {
				seq.Content[i], files[i] = n, src.rel
				continue
			}
			if hasID {
				merged[key] = len(seq.Content)
			}
			seq.Content = append(seq.Content, n)
			files = append(files, src.rel)
		}
	}
	return files, problems
}

// setRun gives an item line read by a run's items_from that run. A line that
// names another run is an error.
func setRun(item *yaml.Node, src source) error {
	if src.run == "" {
		return nil
	}
	n := child(item, "run")
	if n == nil {
		item.Content = append(item.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "run", Line: item.Line, Column: 1},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: src.run, Line: item.Line, Column: 1})
		return nil
	}
	if n.Kind != yaml.ScalarNode || n.Value != src.run {
		return fmt.Errorf("line says run %q but runs[%d] %q reads it", n.Value, src.runIndex, src.run)
	}
	return nil
}

// identity is an item's (run, id, rep) as one key. ok is false without a string id.
func identity(item *yaml.Node) (key string, ok bool) {
	id, ok := stringID(item)
	if !ok {
		return "", false
	}
	var run string
	if n := child(item, "run"); n != nil {
		run = n.Value
	}
	var rep int
	if n := child(item, "rep"); n != nil {
		_ = n.Decode(&rep) // the schema reports a bad rep
	}
	return run + "\x00" + id + "\x00" + strconv.Itoa(rep), true
}

// stringID returns item's id as the loaded document holds it: a string, or the text
// of an unquoted date, which YAML reads as a timestamp and fromNode keeps as text.
func stringID(item *yaml.Node) (string, bool) {
	id := child(item, "id")
	if id == nil || id.Kind != yaml.ScalarNode || (id.ShortTag() != "!!str" && id.ShortTag() != "!!timestamp") {
		return "", false
	}
	return id.Value, true
}

// parseLines parses each complete, non-blank line of a JSONL source into a mapping
// node whose positions point at that line. A bad line is reported and skipped.
func parseLines(src source) ([]*yaml.Node, []Problem) {
	lines := strings.Split(string(src.data), "\n")
	// The last segment is empty, or a line still being written.
	lines = lines[:len(lines)-1]
	var nodes []*yaml.Node
	var problems []Problem
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		bad := func(msg string) {
			problems = append(problems, Problem{File: src.rel, Line: i + 1, Col: 1, Message: msg})
		}
		doc, err := parseLine(line)
		if err != nil {
			bad("JSON syntax: " + err.Error())
			continue
		}
		if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
			bad("each line must be a JSON object")
			continue
		}
		setPos(doc.Content[0], i+1)
		nodes = append(nodes, doc.Content[0])
	}
	return nodes, problems
}

// parseLine parses one JSONL line strictly as JSON, then reads its canonical
// form as YAML.
func parseLine(line string) (*yaml.Node, error) {
	canon, err := canonicalJSON(line)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(canon), &doc); err != nil {
		_, msg := yamlLine(err.Error())
		return nil, errors.New(msg)
	}
	return &doc, nil
}

// canonicalJSON re-encodes one JSON value with encoding/json, keeping key and
// element order, into text yaml.v3 reads as the same value: surrogate pairs
// become runes, and runes YAML does not allow raw are escaped.
func canonicalJSON(line string) (string, error) {
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	var b strings.Builder
	if err := copyJSON(dec, &b); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF // the line ends inside a value
		}
		return "", err
	}
	if _, err := dec.Token(); err != io.EOF {
		return "", errors.New("unexpected data after the JSON value")
	}
	return b.String(), nil
}

// copyJSON writes the next value of dec's token stream to b.
func copyJSON(dec *json.Decoder, b *strings.Builder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch t := tok.(type) {
	case json.Delim:
		b.WriteString(t.String())
		for i := 0; dec.More(); i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			if t == '{' {
				// Token returns an object key as a string value.
				if err := copyJSON(dec, b); err != nil {
					return err
				}
				b.WriteByte(':')
			}
			if err := copyJSON(dec, b); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		b.WriteString(end.(json.Delim).String())
	case string:
		s, _ := json.Marshal(t)
		for _, r := range string(s) {
			if yamlPrintable(r) {
				b.WriteRune(r)
			} else {
				fmt.Fprintf(b, `\u%04x`, r)
			}
		}
	case json.Number:
		b.WriteString(t.String())
	case bool:
		b.WriteString(strconv.FormatBool(t))
	case nil:
		b.WriteString("null")
	}
	return nil
}

// yamlPrintable reports whether yaml.v3 keeps r as is inside a double-quoted
// scalar. YAML allows NEL (#x85) raw, but yaml.v3 folds it like a line break.
func yamlPrintable(r rune) bool {
	switch {
	case r == '\t' || r == '\n' || r == '\r':
		return true
	case r >= 0x20 && r <= 0x7E, r >= 0xA0 && r <= 0xD7FF, r >= 0xE000 && r <= 0xFFFD, r >= 0x10000 && r <= 0x10FFFF:
		return true
	}
	return false
}

// setPos points n and its subtree at a JSONL line. Columns in the canonical text
// do not match the original line, so they are all 1.
func setPos(n *yaml.Node, line int) {
	n.Line, n.Column = line, 1
	for _, c := range n.Content {
		setPos(c, line)
	}
}

// revision hashes what is under review, so context or metadata edits keep answers.
func revision(show any) string {
	b, _ := json.Marshal(show)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:6])
}

// dropDuplicateQuestions removes ask questions whose label repeats an earlier one;
// several agents often ask the same thing. check reports them as warnings.
func dropDuplicateQuestions(item *Map) {
	v, _ := item.Get("ask")
	ask, _ := v.([]any)
	if ask == nil {
		return
	}
	seen := map[string]bool{}
	kept := ask[:0]
	for _, q := range ask {
		m, _ := q.(*Map)
		var label string
		if m != nil {
			l, _ := m.Get("label")
			label = normalizeLabel(l)
		}
		if label != "" && seen[label] {
			continue
		}
		seen[label] = true
		kept = append(kept, q)
	}
	item.Set("ask", kept)
}

func normalizeLabel(v any) string {
	s, _ := v.(string)
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

var printer = message.NewPrinter(language.English)

func schemaIssues(ve *jsonschema.ValidationError) []issue {
	if len(ve.Causes) == 0 {
		ptr := ve.InstanceLocation
		// Point unknown-key errors at the key itself, not the mapping that holds it.
		if k, ok := ve.ErrorKind.(*kind.AdditionalProperties); ok && len(k.Properties) > 0 {
			ptr = append(slices.Clone(ptr), k.Properties[0])
		}
		_, echo := ve.ErrorKind.(*kind.FalseSchema)
		return []issue{{ptr: ptr, msg: friendly(ve.ErrorKind.LocalizedString(printer)), echo: echo}}
	}
	var out []issue
	for _, c := range ve.Causes {
		out = append(out, schemaIssues(c)...)
	}
	return out
}

// dropEchoes removes the "false schema" a failed kind rule leaves on its own keys: when a
// kind's rule fails, unevaluatedProperties also rejects every key that rule declares, the
// failing key's siblings included. The real error inside the same object is the one worth
// reading.
func dropEchoes(issues []issue) []issue {
	var kept []issue
	for _, is := range issues {
		parent := is.ptr[:max(0, len(is.ptr)-1)]
		shadowed := is.echo && slices.ContainsFunc(issues, func(o issue) bool {
			return !o.echo && len(o.ptr) >= len(parent) && slices.Equal(o.ptr[:len(parent)], parent)
		})
		if !shadowed {
			kept = append(kept, is)
		}
	}
	return kept
}

// friendly rewrites the library's wording for the common cases.
func friendly(msg string) string {
	for _, prefix := range []string{"additional properties ", "additionalProperties ", "unevaluated properties ", "unevaluatedProperties "} {
		if rest, ok := strings.CutPrefix(msg, prefix); ok {
			rest = strings.TrimSuffix(strings.TrimSuffix(rest, " not allowed"), " not evaluated")
			return "unknown key " + rest
		}
	}
	return msg
}

var yamlLineRe = regexp.MustCompile(`^yaml: line (\d+): (.*)$`)

// yamlLine splits "yaml: line 343: did not find ..." into the line and the message.
func yamlLine(msg string) (int, string) {
	m := yamlLineRe.FindStringSubmatch(msg)
	if m == nil {
		return 0, strings.TrimPrefix(msg, "yaml: ")
	}
	n, _ := strconv.Atoi(m[1])
	return n, m[2]
}

func itemIndex(ptr []string) (int, bool) {
	if len(ptr) < 2 || ptr[0] != "items" {
		return 0, false
	}
	n, err := strconv.Atoi(ptr[1])
	return n, err == nil
}

func pathString(ptr []string) string {
	var b strings.Builder
	for _, t := range ptr {
		if _, err := strconv.Atoi(t); err == nil {
			b.WriteString("[" + t + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(t)
	}
	return b.String()
}

// locate returns the deepest YAML node on the pointer path.
func locate(root *yaml.Node, ptr []string) *yaml.Node {
	n := root
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	for _, tok := range ptr {
		next := child(n, tok)
		if next == nil {
			return n
		}
		n = next
	}
	return n
}

func child(n *yaml.Node, tok string) *yaml.Node {
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == tok {
				return n.Content[i+1]
			}
		}
	case yaml.SequenceNode:
		if i, err := strconv.Atoi(tok); err == nil && i < len(n.Content) {
			return n.Content[i]
		}
	}
	return nil
}
