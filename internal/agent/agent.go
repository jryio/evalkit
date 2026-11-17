// Package agent runs one agent CLI headless (claude, codex, pi, or any command)
// and turns what it printed into one Result.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/jryio/evalkit/internal/proc"
)

// Adapters.
const (
	Claude  = "claude"
	Codex   = "codex"
	Pi      = "pi"
	Command = "command"
)

// Spec says which agent to run and how.
type Spec struct {
	Adapter   string   `json:"adapter"`
	Model     string   `json:"model"`
	Effort    string   `json:"effort"`
	Command   string   `json:"command"`    // adapter command: a shell command that reads the prompt on stdin
	Args      []string `json:"args"`       // extra arguments for the CLI
	Tools     string   `json:"tools"`      // none: the agent gets no tools
	BudgetUSD float64  `json:"budget_usd"` // spend limit for one attempt, where the CLI has one
	Isolate   string   `json:"isolate"`    // agent (default): nothing of the user's loads; project: claude also loads the workspace's own settings; none
}

// Request is one attempt.
type Request struct {
	Spec   Spec
	Prompt string
	System string   // added to the agent's system prompt
	Schema []byte   // a JSON Schema the final answer must match
	Dir    string   // working directory
	Env    []string // the whole environment; nil: this process's
	Stdin  []byte   // adapter command: what it reads instead of the prompt
	Wrap   []string // a command line put in front of the agent's, such as a sandbox
}

// Turn is one row of the transcript, in the shape of a transcript artifact row.
type Turn struct {
	ID     string `json:"id"`
	Role   string `json:"role"` // user, assistant, tool_call or tool_result
	Text   string `json:"text,omitempty"`
	Name   string `json:"name,omitempty"`
	Input  any    `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
	Error  bool   `json:"error,omitempty"`
}

// Tokens counts what the attempt used.
type Tokens struct {
	In         int `json:"in"`
	Out        int `json:"out"`
	CacheRead  int `json:"cache_read"`
	CacheWrite int `json:"cache_write"`
}

// Result is what one attempt produced.
type Result struct {
	Output     string          `json:"output"`
	Structured json.RawMessage `json:"structured,omitempty"`
	Transcript []Turn          `json:"transcript,omitempty"`
	Model      string          `json:"model,omitempty"`
	CostUSD    float64         `json:"cost_usd"`
	Tokens     Tokens          `json:"tokens"`
	Turns      int             `json:"turns"`
	Seconds    float64         `json:"seconds"`
	Denied     int             `json:"denied,omitempty"` // tool calls the agent was refused
	Skills     []string        `json:"skills,omitempty"`
}

// Failure classes: why an attempt has no scorable output.
const (
	Timeout    = "timeout"     // the wall-clock limit ended it
	AgentError = "agent_error" // the agent ran and reported an error
	Harness    = "harness"     // the CLI did not run or printed nothing usable
)

// Failure is an attempt that did not finish. The Result returned beside it
// still carries what the attempt spent.
type Failure struct {
	Class string
	Msg   string
}

func (f *Failure) Error() string { return f.Class + ": " + f.Msg }

func fail(class, format string, a ...any) *Failure {
	return &Failure{Class: class, Msg: fmt.Sprintf(format, a...)}
}

// invocation is a built command line.
type invocation struct {
	name    string
	args    []string
	stdin   []byte
	env     []string // variables set over the request's environment
	cleanup func()   // removes what the invocation made, if anything
	parse   func(stdout []byte, req Request) (Result, error)
}

// Run runs one attempt. ctx's deadline is the attempt's wall-clock limit: when
// it passes, the agent's whole process group is killed.
func Run(ctx context.Context, req Request) (Result, error) {
	inv, err := build(req)
	if err != nil {
		return Result{}, err
	}
	if inv.cleanup != nil {
		defer inv.cleanup()
	}
	argv := append(append(slices.Clone(req.Wrap), inv.name), inv.args...)
	stdout, stderr, seconds, runErr := execute(ctx, argv, inv.stdin, req.Dir, override(req.Env, inv.env))
	res, parseErr := inv.parse(stdout, req)
	res.Seconds = seconds
	// A timeout or a cancelled run wins over whatever the cut-off output parses to.
	if f, ok := errors.AsType[*Failure](runErr); runErr != nil && (!ok || f.Class != AgentError) {
		return res, runErr
	}
	if parseErr != nil {
		return res, parseErr
	}
	if runErr != nil {
		return res, fail(AgentError, "%s: %s", inv.name, tail(string(stderr), 400))
	}
	if len(req.Schema) > 0 && res.Structured == nil {
		if res.Structured = extractJSON(res.Output); res.Structured == nil {
			return res, fail(AgentError, "the answer is not the JSON object that was asked for: %s", tail(res.Output, 200))
		}
	}
	return res, nil
}

// Exec runs a shell command the way an agent runs: in its own process group,
// killed whole at ctx's deadline. It returns what the command printed.
func Exec(ctx context.Context, command string, stdin []byte, dir string, env []string) (string, error) {
	stdout, stderr, _, err := execute(ctx, []string{"sh", "-c", command}, stdin, dir, env)
	if f, ok := errors.AsType[*Failure](err); ok && f.Class == AgentError {
		return string(stdout), fail(Harness, "%s: %s: %s", command, f.Msg, tail(string(stderr)+string(stdout), 400))
	}
	return string(stdout), err
}

// execute runs argv through proc.Run and names what went wrong: a deadline is
// a Timeout failure, a non-zero exit an AgentError, a command that would not
// start a Harness failure. A cancelled context stays ctx's own error.
func execute(ctx context.Context, argv []string, stdin []byte, dir string, env []string) (stdout, stderr []byte, seconds float64, err error) {
	out, err := proc.Run(ctx, argv, stdin, dir, environ(env))
	switch {
	case err == nil, ctx.Err() != nil && !errors.Is(err, proc.ErrTimeout):
	case errors.Is(err, proc.ErrTimeout):
		err = fail(Timeout, "no result after %.0fs", out.Seconds)
	case errors.As(err, new(*exec.ExitError)):
		err = fail(AgentError, "%v", err)
	default:
		err = fail(Harness, "start %s: %v", argv[0], err)
	}
	return out.Stdout, out.Stderr, out.Seconds, err
}

func build(req Request) (invocation, error) {
	switch req.Spec.Adapter {
	case Claude:
		return claudeInvocation(req), nil
	case Codex:
		return codexInvocation(req)
	case Pi:
		return piInvocation(req), nil
	case Command:
		if req.Spec.Command == "" {
			return invocation{}, errors.New("adapter command needs a command")
		}
		stdin := req.Stdin
		if stdin == nil {
			stdin = []byte(req.Prompt)
		}
		return invocation{name: "sh", args: []string{"-c", req.Spec.Command}, stdin: stdin, parse: parseCommand}, nil
	}
	return invocation{}, fmt.Errorf("adapter %q: want claude, codex, pi or command", req.Spec.Adapter)
}

// override returns env (nil: this process's) with the set variables replacing
// any of the same name.
func override(env, set []string) []string {
	if len(set) == 0 {
		return env
	}
	if env == nil {
		env = os.Environ()
	}
	names := map[string]bool{}
	for _, kv := range set {
		k, _, _ := strings.Cut(kv, "=")
		names[k] = true
	}
	out := make([]string, 0, len(env)+len(set))
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); !names[k] {
			out = append(out, kv)
		}
	}
	return append(out, set...)
}

// environ drops the variables that make a nested claude think it is a child session.
func environ(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "CLAUDECODE=") || strings.HasPrefix(kv, "CLAUDE_CODE_ENTRYPOINT=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Version is the CLI's own version line, for a run's provenance.
func Version(ctx context.Context, adapter string) string {
	if adapter == Command || adapter == "" {
		return ""
	}
	out, err := exec.CommandContext(ctx, adapter, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}

// parseCommand reads a command's output: a JSON Result, or plain text.
func parseCommand(stdout []byte, _ Request) (Result, error) {
	var res Result
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) > 0 && trimmed[0] == '{' && json.Unmarshal(trimmed, &res) == nil && (res.Output != "" || res.Structured != nil || len(res.Transcript) > 0) {
		return res, nil
	}
	return Result{Output: string(stdout)}, nil
}

// extractJSON finds the JSON object in an answer that may wrap it in prose or a code fence.
func extractJSON(s string) json.RawMessage {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end < start {
		return nil
	}
	raw := json.RawMessage(s[start : end+1])
	if !json.Valid(raw) {
		return nil
	}
	return raw
}

func schemaPrompt(prompt string, schema []byte) string {
	if len(schema) == 0 {
		return prompt
	}
	return prompt + "\n\nReply with only one JSON object that matches this JSON Schema, and nothing else:\n" + string(schema)
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

// eachLine calls f with every JSON object line of a JSONL stream.
func eachLine(stdout []byte, f func(json.RawMessage)) {
	for line := range bytes.SplitSeq(stdout, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) > 0 && line[0] == '{' && json.Valid(line) {
			f(line)
		}
	}
}

func turnID(turns []Turn) string { return fmt.Sprintf("t%d", len(turns)+1) }
