package suite

// The typed view holds only what the semantic checks need. The UI gets the raw document.

type Suite struct {
	Title     string            `json:"title"`
	Reviewed  []string          `json:"reviewed"`
	Groups    []Group           `json:"groups"`
	Questions []Question        `json:"questions"`
	Renderers map[string]string `json:"renderers"`
	Items     []Item            `json:"items"`
}

type Group struct {
	ID string `json:"id"`
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
	Group    string              `json:"group"`
	Show     []Artifact          `json:"show"`
	Context  []Artifact          `json:"context"`
	Ask      []Question          `json:"ask"`
	Proposed map[string]Proposal `json:"proposed"`
}

type Artifact struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Key      string `json:"key"`
	Editable bool   `json:"editable"`
	Rows     []any  `json:"rows"`
}

type Proposal struct {
	Value any `json:"value"`
}

// BuiltinKinds are the artifact kinds the web UI renders without a plugin.
var BuiltinKinds = []string{"text", "list", "table", "code", "diff", "tests", "metrics", "chart", "markdown", "json", "transcript", "image", "tally"}

// ReservedKeys are bound by the kit itself.
var ReservedKeys = map[string]string{
	"j": "next item",
	"k": "previous item",
}
