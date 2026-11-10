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
	"os"
	"path/filepath"
	"regexp"
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
}

// Load reads and validates the suite at path. It returns an error only when the
// file cannot be read; everything else is reported as a Problem.
func Load(path string) (Result, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	sum := sha256.Sum256(src)
	res := Result{Version: hex.EncodeToString(sum[:6]), Problems: []Problem{}}
	name := filepath.Base(path)

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
	raw, err := fromNode(&root)
	if err != nil {
		return fatal(0, 0, "decode: "+err.Error())
	}
	doc, ok := raw.(*Map)
	if !ok {
		return fatal(root.Line, root.Column, "the top level must be a mapping")
	}
	js, err := json.Marshal(doc)
	if err != nil {
		return fatal(0, 0, "encode: "+err.Error())
	}

	var issues []issue
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
		issues = append(issues, schemaIssues(ve)...)
	}

	var typed Suite
	_ = json.Unmarshal(js, &typed) // type mismatches are already schema issues
	issues = append(issues, check(typed, filepath.Dir(path))...)

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
			if m, ok := items[idx].(*Map); ok {
				id, _ := m.Get("id")
				p.Item, _ = id.(string)
			}
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
	for i, it := range items {
		if badItems[i] {
			continue
		}
		m := it.(*Map)
		dropDuplicateQuestions(m)
		show, _ := m.Get("show")
		m.Set("revision", revision(show))
		kept = append(kept, m)
	}
	doc.Set("items", kept)
	res.Doc = doc
	return res, nil
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
		return []issue{{ptr: ptr, msg: friendly(ve.ErrorKind.LocalizedString(printer))}}
	}
	var out []issue
	for _, c := range ve.Causes {
		out = append(out, schemaIssues(c)...)
	}
	return out
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
