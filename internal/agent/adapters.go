package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const isolateNone = "none"

func claudeInvocation(req Request) invocation {
	s := req.Spec
	args := []string{"-p", req.Prompt, "--output-format", "stream-json", "--verbose",
		"--no-session-persistence", "--permission-mode", "bypassPermissions"}
	if s.Isolate != isolateNone {
		// Project settings only: the user's own skills, memory and MCP servers stay out.
		args = append(args, "--setting-sources", "project", "--strict-mcp-config")
	}
	if s.Model != "" {
		args = append(args, "--model", s.Model)
	}
	if s.Effort != "" {
		args = append(args, "--effort", s.Effort)
	}
	if req.System != "" {
		args = append(args, "--append-system-prompt", req.System)
	}
	if s.Tools == "none" {
		args = append(args, "--tools", "")
	}
	if s.BudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprint(s.BudgetUSD))
	}
	if len(req.Schema) > 0 {
		args = append(args, "--json-schema", string(req.Schema))
	}
	return invocation{name: "claude", args: append(args, s.Args...), parse: parseClaude}
}

func parseClaude(stdout []byte, _ Request) (Result, error) {
	var res Result
	var failure error
	sawResult := false
	eachLine(stdout, func(line json.RawMessage) {
		var e struct {
			Type    string   `json:"type"`
			Subtype string   `json:"subtype"`
			Model   string   `json:"model"`
			Skills  []string `json:"skills"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
			Result     string          `json:"result"`
			Structured json.RawMessage `json:"structured_output"`
			IsError    bool            `json:"is_error"`
			Cost       float64         `json:"total_cost_usd"`
			Turns      int             `json:"num_turns"`
			Denials    []any           `json:"permission_denials"`
			Usage      struct {
				In         int `json:"input_tokens"`
				Out        int `json:"output_tokens"`
				CacheRead  int `json:"cache_read_input_tokens"`
				CacheWrite int `json:"cache_creation_input_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(line, &e) != nil {
			return
		}
		switch e.Type {
		case "system":
			if e.Subtype == "init" {
				res.Model, res.Skills = e.Model, e.Skills
			}
		case "assistant", "user":
			res.Transcript = appendBlocks(res.Transcript, e.Message.Content)
		case "result":
			sawResult = true
			res.Output, res.CostUSD, res.Turns, res.Denied = e.Result, e.Cost, e.Turns, len(e.Denials)
			res.Tokens = Tokens{In: e.Usage.In, Out: e.Usage.Out, CacheRead: e.Usage.CacheRead, CacheWrite: e.Usage.CacheWrite}
			if len(e.Structured) > 0 && string(e.Structured) != "null" {
				res.Structured = e.Structured
			}
			if e.IsError || e.Subtype != "success" {
				failure = fail(AgentError, "claude ended with %s: %s", e.Subtype, tail(e.Result, 300))
			}
		}
	})
	if !sawResult {
		return res, fail(Harness, "claude printed no result")
	}
	return res, failure
}

// appendBlocks adds the text, tool calls and tool results of one Anthropic
// message's content to the transcript.
func appendBlocks(turns []Turn, content json.RawMessage) []Turn {
	var blocks []struct {
		Type    string          `json:"type"`
		Text    string          `json:"text"`
		ID      string          `json:"id"`
		Name    string          `json:"name"`
		Input   any             `json:"input"`
		Content json.RawMessage `json:"content"`
		IsError bool            `json:"is_error"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return turns
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				turns = append(turns, Turn{ID: turnID(turns), Role: "assistant", Text: b.Text})
			}
		case "tool_use":
			turns = append(turns, Turn{ID: turnID(turns), Role: "tool_call", Name: b.Name, Input: b.Input})
		case "tool_result":
			turns = append(turns, Turn{ID: turnID(turns), Role: "tool_result", Output: blockText(b.Content), Error: b.IsError})
		}
	}
	return turns
}

// blockText reads content that is a string or a list of text blocks.
func blockText(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &parts) != nil {
		return string(content)
	}
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

func codexInvocation(req Request) (invocation, error) {
	s := req.Spec
	args := []string{"exec", "--json", "--ephemeral", "--skip-git-repo-check"}
	switch {
	case s.Isolate == isolateNone:
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	case s.Tools == "none":
		args = append(args, "--ignore-user-config", "--ignore-rules", "-s", "read-only")
	default:
		args = append(args, "--ignore-user-config", "--ignore-rules", "-s", "workspace-write")
	}
	if s.Model != "" {
		args = append(args, "-m", s.Model)
	}
	if s.Effort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", s.Effort))
	}
	if len(req.Schema) > 0 {
		f, err := os.CreateTemp("", "evalkit-schema-*.json")
		if err != nil {
			return invocation{}, err
		}
		if _, err := f.Write(req.Schema); err != nil {
			return invocation{}, err
		}
		f.Close()
		args = append(args, "--output-schema", f.Name())
	}
	prompt := req.Prompt
	if req.System != "" {
		prompt = req.System + "\n\n" + prompt
	}
	return invocation{name: "codex", args: append(append(args, s.Args...), prompt), parse: parseCodex}, nil
}

func parseCodex(stdout []byte, req Request) (Result, error) {
	res := Result{Model: req.Spec.Model}
	var failure error
	eachLine(stdout, func(line json.RawMessage) {
		var e struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Error   struct {
				Message string `json:"message"`
			} `json:"error"`
			Item struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Command  string `json:"command"`
				Output   string `json:"aggregated_output"`
				ExitCode *int   `json:"exit_code"`
				Changes  any    `json:"changes"`
				Message  string `json:"message"`
			} `json:"item"`
			Usage struct {
				In        int `json:"input_tokens"`
				Cached    int `json:"cached_input_tokens"`
				CacheSave int `json:"cache_write_input_tokens"`
				Out       int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(line, &e) != nil {
			return
		}
		switch e.Type {
		case "item.completed":
			switch e.Item.Type {
			case "agent_message":
				res.Output = e.Item.Text
				res.Transcript = append(res.Transcript, Turn{ID: turnID(res.Transcript), Role: "assistant", Text: e.Item.Text})
			case "command_execution":
				res.Transcript = append(res.Transcript, Turn{ID: turnID(res.Transcript), Role: "tool_call", Name: "shell", Input: e.Item.Command})
				res.Transcript = append(res.Transcript, Turn{ID: turnID(res.Transcript), Role: "tool_result", Output: e.Item.Output, Error: e.Item.ExitCode != nil && *e.Item.ExitCode != 0})
			case "file_change":
				res.Transcript = append(res.Transcript, Turn{ID: turnID(res.Transcript), Role: "tool_call", Name: "file_change", Input: e.Item.Changes})
			case "error":
				failure = fail(AgentError, "codex: %s", e.Item.Message)
			}
		case "turn.completed":
			res.Turns++
			res.Tokens.In += e.Usage.In - e.Usage.Cached
			res.Tokens.CacheRead += e.Usage.Cached
			res.Tokens.CacheWrite += e.Usage.CacheSave
			res.Tokens.Out += e.Usage.Out
		case "turn.failed":
			failure = fail(AgentError, "codex: %s", e.Error.Message)
		case "error":
			failure = fail(AgentError, "codex: %s", e.Message)
		}
	})
	if res.Turns == 0 && failure == nil {
		return res, fail(Harness, "codex finished no turn")
	}
	return res, failure
}

func piInvocation(req Request) invocation {
	s := req.Spec
	args := []string{"-p", "--mode", "json", "--no-session"}
	if s.Isolate != isolateNone {
		args = append(args, "--no-extensions", "--no-mcp")
	}
	if s.Model != "" {
		args = append(args, "--model", s.Model)
	}
	if s.Effort != "" {
		args = append(args, "--thinking", s.Effort)
	}
	if req.System != "" {
		args = append(args, "--append-system-prompt", req.System)
	}
	if s.Tools == "none" {
		args = append(args, "--no-tools")
	}
	// pi has no structured-output flag, so the schema goes in the prompt.
	return invocation{name: "pi", args: append(append(args, s.Args...), schemaPrompt(req.Prompt, req.Schema)), parse: parsePi}
}

func parsePi(stdout []byte, _ Request) (Result, error) {
	var res Result
	var failure error
	settled := false
	eachLine(stdout, func(line json.RawMessage) {
		var e struct {
			Type    string `json:"type"`
			Aborted bool   `json:"aborted"`
			Message struct {
				Role    string          `json:"role"`
				Model   string          `json:"model"`
				Content json.RawMessage `json:"content"`
				IsError bool            `json:"isError"`
				Stop    string          `json:"stopReason"`
				ErrText string          `json:"errorMessage"`
				Usage   struct {
					In         int `json:"input"`
					Out        int `json:"output"`
					CacheRead  int `json:"cacheRead"`
					CacheWrite int `json:"cacheWrite"`
					Cost       struct {
						Total float64 `json:"total"`
					} `json:"cost"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &e) != nil {
			return
		}
		switch e.Type {
		case "agent_settled":
			settled = true
			if e.Aborted {
				failure = fail(AgentError, "pi aborted")
			}
		case "message_end":
			m := e.Message
			switch m.Role {
			case "assistant":
				res.Turns++
				res.Model = m.Model
				res.CostUSD += m.Usage.Cost.Total
				res.Tokens.In += m.Usage.In
				res.Tokens.Out += m.Usage.Out
				res.Tokens.CacheRead += m.Usage.CacheRead
				res.Tokens.CacheWrite += m.Usage.CacheWrite
				var blocks []struct {
					Type string `json:"type"`
					Text string `json:"text"`
					Name string `json:"name"`
					Args any    `json:"arguments"`
				}
				json.Unmarshal(m.Content, &blocks)
				for _, b := range blocks {
					switch b.Type {
					case "text":
						if strings.TrimSpace(b.Text) != "" {
							res.Output = b.Text
							res.Transcript = append(res.Transcript, Turn{ID: turnID(res.Transcript), Role: "assistant", Text: b.Text})
						}
					case "toolCall":
						res.Transcript = append(res.Transcript, Turn{ID: turnID(res.Transcript), Role: "tool_call", Name: b.Name, Input: b.Args})
					}
				}
				if m.Stop == "error" {
					failure = fail(AgentError, "pi: %s", m.ErrText)
				}
			case "toolResult":
				res.Transcript = append(res.Transcript, Turn{ID: turnID(res.Transcript), Role: "tool_result", Output: blockText(m.Content), Error: m.IsError})
			}
		}
	})
	if !settled && failure == nil {
		return res, fail(Harness, "pi did not finish")
	}
	return res, failure
}
