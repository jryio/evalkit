// Package bench runs a benchmark: every case of a suite through an agent in an
// isolated workspace, then through scorers and a judge, into one run of rows
// the review page reads.
package bench

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jryio/evalkit/internal/agent"
	"github.com/jryio/evalkit/internal/stats"
	"github.com/jryio/evalkit/internal/suite"
	"github.com/jryio/evalkit/internal/workspace"
)

// Config is the bench block of a suite.
type Config struct {
	Cases       string           `json:"cases"`       // JSONL file, one case per line
	Reps        int              `json:"reps"`        // attempts per case; default 1
	Concurrency int              `json:"concurrency"` // attempts at once; default 4
	Timeout     float64          `json:"timeout"`     // seconds per agent or stage; default 600
	BudgetUSD   float64          `json:"budget_usd"`  // stop starting attempts once the run has spent this
	Retries     int              `json:"retries"`     // extra tries after a harness failure or timeout
	Env         []string         `json:"env"`         // variables passed through beside the defaults; "*" passes all
	Home        string           `json:"home"`        // inherit (default) or private: a fresh HOME per attempt
	Sandbox     bool             `json:"sandbox"`     // wrap the agent in an OS sandbox (macOS)
	Workspace   workspace.Config `json:"workspace"`
	Cwd         string           `json:"cwd"`     // the agent's directory in the slot; default: the only source, else the slot
	Overlay     []Overlay        `json:"overlay"` // suite files copied into the slot before each attempt
	Prepare     string           `json:"prepare"` // shell stage before the agent
	Agent       AgentStage       `json:"agent"`
	Collect     string           `json:"collect"` // shell stage after the agent: what the reviewer sees
	Score       []string         `json:"score"`   // shell stages after collect: measured values and verdicts
	Judge       *JudgeStage      `json:"judge"`
	Oracle      string           `json:"oracle"` // shell agent that must score every case as a pass
	Null        string           `json:"null"`   // shell agent that must score every case as a fail
	Climb       *ClimbConfig     `json:"climb"`
}

// Overlay copies a file or directory of the suite into the slot: the thing
// under test (a prompt, a skill, a config).
type Overlay struct {
	From string `json:"from"` // relative to the suite
	To   string `json:"to"`   // relative to the slot
}

// AgentStage is the agent under test.
type AgentStage struct {
	agent.Spec
	Prompt        string   `json:"prompt"`         // template over the case; default {{.prompt}}
	System        string   `json:"system"`         // suite file added to the agent's system prompt
	RequireSkills []string `json:"require_skills"` // skills the agent must have loaded (claude)
}

// JudgeStage grades each attempt with a model and a rubric.
type JudgeStage struct {
	agent.Spec
	Rubric    string   `json:"rubric"`    // suite file: the judge's instructions
	Questions []string `json:"questions"` // suite question ids to answer; default: every choice, number, scale and toggle
}

// ClimbConfig is the hill-climbing block: what may change between rounds and how.
type ClimbConfig struct {
	Targets []string   `json:"targets"` // suite files a round may edit
	Train   []string   `json:"train"`   // groups the proposer sees and rounds are accepted on
	Test    []string   `json:"test"`    // held-out groups: scored every round, never shown to the proposer
	Propose AgentStage `json:"propose"` // the agent that edits the targets
	Rounds  int        `json:"rounds"`  // default 3
	Target  float64    `json:"target"`  // stop once the train score reaches this
}

// Case is one line of the cases file.
type Case map[string]any

func (c Case) ID() string { s, _ := c["id"].(string); return s }

// withoutAnswers is the case an agent may see: no expect.
func (c Case) withoutAnswers() Case {
	out := Case{}
	for k, v := range c {
		if k != "expect" {
			out[k] = v
		}
	}
	return out
}

// pins is the case's commit per git source, if it sets pin.
func (c Case) pins() map[string]string {
	m, _ := c["pin"].(map[string]any)
	if len(m) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// Loaded is a suite ready to run.
type Loaded struct {
	Path     string
	Dir      string
	Config   Config
	Suite    suite.Suite
	Stats    stats.Suite // the suite as the score reads it
	Columns  map[string]bool
	Cases    []Case
	RunsFrom bool // the suite reads run manifests
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// Load reads the suite at path, its bench block and its cases.
func Load(path string) (*Loaded, error) {
	res, err := suite.Load(path)
	if err != nil {
		return nil, err
	}
	if res.Fatal {
		var first []string
		for _, p := range res.Problems {
			if !p.Warning && len(first) < 3 {
				first = append(first, p.String())
			}
		}
		return nil, fmt.Errorf("%s has errors:\n  %s", path, strings.Join(first, "\n  "))
	}
	js, err := json.Marshal(res.Doc)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Bench    *Config `json:"bench"`
		RunsFrom any     `json:"runs_from"`
	}
	if err := json.Unmarshal(js, &doc); err != nil {
		return nil, err
	}
	if doc.Bench == nil {
		return nil, fmt.Errorf("%s has no bench block: see \"evalkit docs\"", path)
	}
	l := &Loaded{Path: path, Dir: filepath.Dir(path), Config: *doc.Bench, Columns: map[string]bool{}, RunsFrom: doc.RunsFrom != nil}
	if l.Dir, err = filepath.Abs(l.Dir); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(js, &l.Suite); err != nil {
		return nil, err
	}
	if l.Stats, err = stats.Decode(res.Doc); err != nil {
		return nil, err
	}
	for _, c := range l.Suite.Columns {
		l.Columns[c.ID] = true
	}
	c := &l.Config
	if c.Reps <= 0 {
		c.Reps = 1
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 4
	}
	if c.Timeout <= 0 {
		c.Timeout = 600
	}
	if c.Agent.Adapter == "" {
		return nil, errors.New("bench.agent.adapter: want claude, codex, pi or command")
	}
	if c.Cases == "" {
		return nil, errors.New("bench.cases: name the JSONL file of cases")
	}
	if l.Cases, err = readCases(filepath.Join(l.Dir, c.Cases)); err != nil {
		return nil, err
	}
	return l, nil
}

func readCases(path string) ([]Case, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []Case
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var c Case
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", filepath.Base(path), line, err)
		}
		id := c.ID()
		if !idRe.MatchString(id) {
			return nil, fmt.Errorf("%s:%d: a case needs an id of letters, digits, '_', '.' and '-'", filepath.Base(path), line)
		}
		if seen[id] {
			return nil, fmt.Errorf("%s:%d: case %q appears twice", filepath.Base(path), line, id)
		}
		seen[id] = true
		cases = append(cases, c)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("%s has no cases", filepath.Base(path))
	}
	return cases, sc.Err()
}
