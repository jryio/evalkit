package suite

import "strconv"

// The typed view holds only what the semantic checks need. The UI gets the raw document.

type Suite struct {
	Title      string            `json:"title"`
	View       string            `json:"view"`
	Reviewed   []string          `json:"reviewed"`
	Groups     []Group           `json:"groups"`
	Questions  []Question        `json:"questions"`
	Renderers  map[string]string `json:"renderers"`
	Columns    []Column          `json:"columns"`
	Dimensions []Dimension       `json:"dimensions"`
	Runs       []Run             `json:"runs"`
	Items      []Item            `json:"items"`
	ItemsFrom  any               `json:"items_from"`
}

type Group struct {
	ID string `json:"id"`
}

type Dimension struct {
	ID    string `json:"id"`
	Order []any  `json:"order"`
}

type Run struct {
	ID        string         `json:"id"`
	Baseline  bool           `json:"baseline"`
	ItemsFrom any            `json:"items_from"`
	Dims      map[string]any `json:"dims"`
	Values    map[string]any `json:"values"`
}

type Column struct {
	ID      string   `json:"id"`
	Min     *float64 `json:"min"`
	Max     *float64 `json:"max"`
	Primary bool     `json:"primary"`
}

type Question struct {
	ID      string           `json:"id"`
	Kind    string           `json:"kind"`
	Label   string           `json:"label"`
	Key     string           `json:"key"`
	Groups  []string         `json:"groups"`
	With    string           `json:"with"`
	Each    string           `json:"each"`
	From    string           `json:"from"`
	When    map[string][]any `json:"when"`
	Primary bool             `json:"primary"`
	Status  bool             `json:"status"`
	Options []Option         `json:"options"`
	Min     *float64         `json:"min"`
	Max     *float64         `json:"max"`
}

type Option struct {
	Value any    `json:"value"`
	Label string `json:"label"`
	Key   string `json:"key"`
}

type Item struct {
	ID       string              `json:"id"`
	Run      string              `json:"run"`
	Rep      int                 `json:"rep"`
	Group    string              `json:"group"`
	Show     []Artifact          `json:"show"`
	Context  []Artifact          `json:"context"`
	Ask      []Question          `json:"ask"`
	Proposed map[string]Proposal `json:"proposed"`
	Values   map[string]any      `json:"values"`
}

// pair is the key that pairs an item across runs: its id, plus .r<rep> for a repetition.
func (it Item) pair() string {
	if it.Rep > 0 {
		return it.ID + ".r" + strconv.Itoa(it.Rep)
	}
	return it.ID
}

// fullID is the item's id in the loaded suite: <run>/<pair> for an item in a run.
func (it Item) fullID() string {
	if it.Run == "" {
		return it.ID
	}
	return it.Run + "/" + it.pair()
}

type Artifact struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Key      string `json:"key"`
	Editable bool   `json:"editable"`
	Rows     []any  `json:"rows"`
}

type Proposal struct {
	Value    any `json:"value"`
	Baseline any `json:"baseline"`
}

// BuiltinKinds are the artifact kinds the web UI renders without a plugin.
var BuiltinKinds = []string{"text", "list", "table", "code", "compare", "diff", "tests", "metrics", "chart", "markdown", "json", "transcript", "image", "tally"}

// ReservedKeys are bound by the kit itself.
var ReservedKeys = map[string]string{
	"j": "next item",
	"k": "previous item",
}
