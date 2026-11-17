//go:build unix

package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fixture(tb testing.TB, name string) []byte {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		tb.Fatal(err)
	}
	return b
}

func roles(turns []Turn) []string {
	var out []string
	for _, t := range turns {
		out = append(out, t.Role)
	}
	return out
}

// The fixtures are real sessions: each agent was asked to write hello.txt and cat it.
func TestParse_RealSessions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		parse     func([]byte, Request) (Result, error)
		file      string
		wantModel string
		wantRoles []string
		wantCost  bool
	}{
		{"claude", parseClaude, "claude.jsonl", "claude-opus-5-5", []string{"tool_call", "tool_result", "assistant"}, true},
		{"codex", parseCodex, "codex.jsonl", "", []string{"assistant", "tool_call", "tool_result", "assistant"}, false},
		{"pi", parsePi, "pi.jsonl", "claude-opus-5-5", []string{"tool_call", "tool_result", "tool_call", "tool_result", "assistant"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := tc.parse(fixture(t, tc.file), Request{})
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !strings.Contains(res.Output, "hi") {
				t.Errorf("Output = %q, want the final answer", res.Output)
			}
			if res.Model != tc.wantModel {
				t.Errorf("Model = %q, want %q", res.Model, tc.wantModel)
			}
			if got := roles(res.Transcript); !slices.Equal(got, tc.wantRoles) {
				t.Errorf("transcript roles = %v, want %v", got, tc.wantRoles)
			}
			if (res.CostUSD > 0) != tc.wantCost {
				t.Errorf("CostUSD = %v, want reported = %v", res.CostUSD, tc.wantCost)
			}
			if res.Turns == 0 || res.Tokens.Out == 0 {
				t.Errorf("Turns = %d, Tokens = %+v, want both counted", res.Turns, res.Tokens)
			}
		})
	}
}

func TestParse_Failures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		parse     func([]byte, Request) (Result, error)
		stdout    string
		wantClass string
	}{
		{"claude with no result is a harness failure", parseClaude, `{"type":"system","subtype":"init"}`, Harness},
		{"claude error result", parseClaude, `{"type":"result","subtype":"error_max_turns","is_error":true,"total_cost_usd":0.2}`, AgentError},
		{"codex with no turn", parseCodex, `{"type":"thread.started"}`, Harness},
		{"codex failed turn", parseCodex, `{"type":"turn.failed","error":{"message":"rate limit"}}`, AgentError},
		{"pi cut short", parsePi, `{"type":"agent_start"}`, Harness},
		{"pi aborted", parsePi, `{"type":"agent_settled","aborted":true}`, AgentError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.parse([]byte(tc.stdout), Request{})
			f, ok := errors.AsType[*Failure](err)
			if !ok || f.Class != tc.wantClass {
				t.Errorf("error = %v, want a %s failure", err, tc.wantClass)
			}
		})
	}
}

func TestParseClaude_KeepsSpendOfAFailedAttempt(t *testing.T) {
	t.Parallel()
	res, _ := parseClaude([]byte(`{"type":"result","subtype":"error_max_turns","is_error":true,"total_cost_usd":0.2}`), Request{})
	if res.CostUSD != 0.2 {
		t.Errorf("CostUSD = %v, want the failed attempt's 0.2", res.CostUSD)
	}
}

func TestBuild_IsolationFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		spec     Spec
		want     []string
		wantNone []string
	}{
		{"claude isolated", Spec{Adapter: Claude, Model: "m", Effort: "low", BudgetUSD: 2}, []string{"--setting-sources", "--strict-mcp-config", "--max-budget-usd", "--effort"}, nil},
		{"claude with the user's config", Spec{Adapter: Claude, Isolate: "none"}, nil, []string{"--setting-sources", "--strict-mcp-config"}},
		{"claude judge without tools", Spec{Adapter: Claude, Tools: "none"}, []string{"--tools"}, nil},
		{"codex sandboxed", Spec{Adapter: Codex}, []string{"workspace-write", "--ignore-user-config", "--ephemeral"}, []string{"--dangerously-bypass-approvals-and-sandbox"}},
		{"codex unsandboxed", Spec{Adapter: Codex, Isolate: "none"}, []string{"--dangerously-bypass-approvals-and-sandbox"}, []string{"workspace-write"}},
		{"codex judge without tools", Spec{Adapter: Codex, Tools: "none"}, []string{"read-only"}, nil},
		{"pi isolated", Spec{Adapter: Pi, Effort: "low"}, []string{"--no-extensions", "--no-mcp", "--no-session", "--thinking"}, nil},
		{"pi with the user's config", Spec{Adapter: Pi, Isolate: "none"}, nil, []string{"--no-extensions", "--no-mcp"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inv, err := build(Request{Spec: tc.spec, Prompt: "do it"})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			for _, w := range tc.want {
				if !slices.Contains(inv.args, w) {
					t.Errorf("args %v lack %q", inv.args, w)
				}
			}
			for _, w := range tc.wantNone {
				if slices.Contains(inv.args, w) {
					t.Errorf("args %v have %q", inv.args, w)
				}
			}
		})
	}
}

func TestBuild_UnknownAdapter(t *testing.T) {
	t.Parallel()
	if _, err := build(Request{Spec: Spec{Adapter: "gpt"}}); err == nil {
		t.Error("build accepted an unknown adapter")
	}
	if _, err := build(Request{Spec: Spec{Adapter: Command}}); err == nil {
		t.Error("build accepted adapter command with no command")
	}
}

func command(cmd string) Spec { return Spec{Adapter: Command, Command: cmd} }

func TestRun_Command(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		req        Request
		wantOutput string
		wantCost   float64
		wantStruct string
	}{
		{"plain text is the output", Request{Spec: command("cat"), Prompt: "echo me"}, "echo me", 0, ""},
		{"a JSON result is read as one", Request{Spec: command(`echo '{"output":"done","cost_usd":0.5,"turns":3}'`)}, "done", 0.5, ""},
		{"stdin replaces the prompt", Request{Spec: command("cat"), Prompt: "p", Stdin: []byte("payload")}, "payload", 0, ""},
		{"a schema pulls the JSON out of prose", Request{Spec: command(`echo 'Sure: {"verdict":"pass"} done'`), Schema: []byte(`{}`)}, "", 0, `{"verdict":"pass"}`},
		{"the environment is the request's", Request{Spec: command(`printf %s "$ONLY"`), Env: []string{"ONLY=this", "PATH=" + os.Getenv("PATH")}}, "this", 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.req.Dir = t.TempDir()
			res, err := Run(t.Context(), tc.req)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if tc.wantOutput != "" && strings.TrimSpace(res.Output) != tc.wantOutput {
				t.Errorf("Output = %q, want %q", res.Output, tc.wantOutput)
			}
			if res.CostUSD != tc.wantCost {
				t.Errorf("CostUSD = %v, want %v", res.CostUSD, tc.wantCost)
			}
			if string(res.Structured) != tc.wantStruct {
				t.Errorf("Structured = %s, want %s", res.Structured, tc.wantStruct)
			}
		})
	}
}

func TestRun_CommandFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		req       Request
		wantClass string
	}{
		{"a non-zero exit is an agent error", Request{Spec: command("echo oops >&2; exit 3")}, AgentError},
		{"an answer that is not the JSON asked for", Request{Spec: command("echo no json here"), Schema: []byte(`{}`)}, AgentError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.req.Dir = t.TempDir()
			_, err := Run(t.Context(), tc.req)
			f, ok := errors.AsType[*Failure](err)
			if !ok || f.Class != tc.wantClass {
				t.Errorf("error = %v, want a %s failure", err, tc.wantClass)
			}
		})
	}
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func readPID(tb testing.TB, path string) int {
	tb.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	tb.Fatalf("no pid in %s", path)
	return 0
}

func waitDead(pid int) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestRun_TimeoutKillsTheWholeProcessGroup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	_, err := Run(ctx, Request{Spec: command("sleep 60 & echo $! > " + pidFile + "; wait"), Dir: dir})
	f, ok := errors.AsType[*Failure](err)
	if !ok || f.Class != Timeout {
		t.Fatalf("error = %v, want a timeout failure", err)
	}
	if pid := readPID(t, pidFile); !waitDead(pid) {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("the agent's child %d outlived the timeout", pid)
	}
}

func TestRun_KillsChildrenLeftBehind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	// The agent exits at once and leaves a background server with its output detached.
	res, err := Run(t.Context(), Request{Spec: command("(sleep 60 >/dev/null 2>&1 & echo $! > " + pidFile + "); echo done"), Dir: dir})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(res.Output) != "done" {
		t.Errorf("Output = %q", res.Output)
	}
	if pid := readPID(t, pidFile); !waitDead(pid) {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("the agent's leftover child %d is still running", pid)
	}
}
