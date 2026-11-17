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
	"strings"
	"time"
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
	Isolate   string   `json:"isolate"`    // agent (default): ignore the user's own config and use the CLI's sandbox; none
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
	name  string
	args  []string
	stdin []byte
	parse func(stdout []byte, req Request) (Result, error)
}

// Run runs one attempt. ctx's deadline is the attempt's wall-clock limit: when
// it passes, the agent's whole process group is killed.
func Run(ctx context.Context, req Request) (Result, error) {
	inv, err := build(req)
	if err != nil {
		return Result{}, err
	}
	cmd := exec.Command(inv.name, inv.args...)
	cmd.Dir = req.Dir
	cmd.Env = environ(req.Env)
	cmd.Stdin = bytes.NewReader(inv.stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	ownGroup(cmd)
	// A child that outlives the agent and holds its output open must not hang the attempt.
	cmd.WaitDelay = 5 * time.Second

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return Result{}, fail(Harness, "start %s: %v", inv.name, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		killGroup(cmd)
		<-done
		res, _ := inv.parse(stdout.Bytes(), req)
		res.Seconds = time.Since(start).Seconds()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return res, fail(Timeout, "no result after %.0fs", res.Seconds)
		}
		return res, ctx.Err()
	}
	// Children the agent left running (a dev server, a watcher) end with it.
	killGroup(cmd)

	res, err := inv.parse(stdout.Bytes(), req)
	res.Seconds = time.Since(start).Seconds()
	if err != nil {
		return res, err
	}
	if waitErr != nil && !errors.Is(waitErr, exec.ErrWaitDelay) {
		return res, fail(AgentError, "%s: %v: %s", inv.name, waitErr, tail(stderr.String(), 400))
	}
	if len(req.Schema) > 0 && res.Structured == nil {
		if res.Structured = extractJSON(res.Output); res.Structured == nil {
			return res, fail(AgentError, "the answer is not the JSON object that was asked for: %s", tail(res.Output, 200))
		}
	}
	return res, nil
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
