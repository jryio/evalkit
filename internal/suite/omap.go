package suite

import (
	"bytes"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Map is a JSON object that keeps the key order of the YAML it came from, so
// agent-written data (tool arguments, JSON trees) renders in the order written.
type Map struct {
	keys []string
	vals map[string]any
}

func (m *Map) Get(k string) (any, bool) {
	v, ok := m.vals[k]
	return v, ok
}

// Set replaces a value in place or appends a new key.
func (m *Map) Set(k string, v any) {
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
}

func (m *Map) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		vb, err := json.Marshal(m.vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(kb)
		b.WriteByte(':')
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// fromNode converts YAML to JSON-ready values: mappings become *Map, sequences
// []any, scalars their decoded value. Timestamps keep their source text.
func fromNode(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return fromNode(n.Content[0])
	case yaml.AliasNode:
		return fromNode(n.Alias)
	case yaml.MappingNode:
		m := &Map{vals: make(map[string]any, len(n.Content)/2)}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("line %d: keys must be strings", k.Line)
			}
			v, err := fromNode(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m.Set(k.Value, v)
		}
		return m, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := fromNode(c)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	default:
		if n.Tag == "!!timestamp" {
			return n.Value, nil
		}
		var v any
		err := n.Decode(&v)
		return v, err
	}
}
