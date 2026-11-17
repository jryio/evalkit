package suite

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// readRunManifests appends the run each runs_from file holds to the suite's
// runs, in started_at order, so a runner can add a run without editing the
// suite. It returns the files read, for the version hash.
func readRunManifests(top *yaml.Node, dir string) ([]source, []issue) {
	n := child(top, "runs_from")
	if n == nil {
		return nil, nil
	}
	var from any
	if n.Decode(&from) != nil {
		return nil, nil // the schema reports it
	}
	files, issues := jsonlFiles(dir, from, []string{"runs_from"})
	var read []source
	var runs []*yaml.Node
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			rel = f
		}
		rel = filepath.ToSlash(rel)
		doc, err := parseLine(string(data))
		if err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
			issues = append(issues, issue{ptr: []string{"runs_from"}, msg: rel + " is not a JSON object", warning: true})
			continue
		}
		read = append(read, source{rel: rel, data: data})
		runs = append(runs, doc.Content[0])
	}
	if len(runs) == 0 {
		return read, issues
	}
	slices.SortStableFunc(runs, func(a, b *yaml.Node) int {
		return strings.Compare(scalar(a, "started_at"), scalar(b, "started_at"))
	})
	seq := child(top, "runs")
	if seq == nil {
		seq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		top.Content = append(top.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "runs"}, seq)
	}
	if seq.Kind == yaml.SequenceNode {
		seq.Content = append(seq.Content, runs...)
	}
	return read, issues
}

func scalar(n *yaml.Node, key string) string {
	if c := child(n, key); c != nil {
		return c.Value
	}
	return ""
}
