// Command evalkit serves a keyboard-first review page for agent output and hands
// the human's feedback back to the agent.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const intro = `evalkit: a keyboard-first review page for agent output.

Write a suite (YAML), serve it, and wait. The human reviews in the browser and
presses ⌘↵ whenever they like; the agent gets back only what changed.

Get started:
  evalkit init        write ./suite.yaml, a starter to edit
  evalkit serve       open the review page (keeps running across sends)
  evalkit wait        block until the human sends, then print what changed

Commands:
`

const outro = `
SUITE defaults to ./suite.yaml. Feedback goes to feedback/ beside the suite.
Run "evalkit help <command>" for details and examples.
`

type command struct {
	name    string
	args    string // positional arguments, for the usage line
	summary string
	help    string // details and examples
	run     func(fs *flag.FlagSet, args []string) error
	flags   func(fs *flag.FlagSet)
}

var commands = []*command{
	{name: "init", args: "[DIR]", summary: "write a starter suite.yaml (or the every-kind example)", run: runInit,
		flags: func(fs *flag.FlagSet) {
			fs.Bool("example", false, "write the kitchen sink: every artifact and question kind once")
		},
		help: `Writes DIR/suite.yaml (DIR defaults to the current directory). It refuses to
overwrite an existing file.

  evalkit init                 a two-item starter to edit
  evalkit init --example demo  every kind once, as a catalog to copy from`},
	{name: "validate", args: "[SUITE]", summary: "check a suite; exit 1 on errors", run: runValidate,
		help: `Prints every problem as file:line:col, the item and the key, then exits 1 if
any is an error. Items with errors are skipped by "serve" instead of breaking it.

  evalkit validate
  evalkit validate evals/canary/suite.yaml`},
	{name: "serve", args: "[SUITE]", summary: "serve the review page", run: runServe,
		flags: func(fs *flag.FlagSet) {
			fs.Int("port", 8790, "first port to try; the next 9 are tried if it is busy")
			fs.Bool("no-open", false, "do not open a browser")
			fs.String("web", "", "serve UI files from this directory instead of the embedded copy")
			feedbackFlag(fs)
		},
		help: `Serves the review page on 127.0.0.1 and keeps running: each send writes a new
snapshot and the human can keep reviewing. The suite is re-read on every load,
so edits show up without a restart. Agents run it in the background.

  evalkit serve
  evalkit serve evals/canary/suite.yaml --port 8800 --no-open`},
	{name: "wait", args: "[SUITE]", summary: "block until the human sends, then print what changed", run: runWait,
		flags: feedbackFlag,
		help: `Blocks until a snapshot newer than the last one it reported exists, then prints
the approved actions and every answer that changed since then. Agents run it in
the background, read its output when it exits, and start it again right away.
It reads the feedback directory, so it works across server restarts.

  evalkit wait`},
	{name: "summarize", args: "[SUITE]", summary: "print a send, diffed against the one before", run: runSummarize,
		flags: func(fs *flag.FlagSet) {
			fs.Int("seq", 0, "snapshot number (default: the newest)")
			feedbackFlag(fs)
		},
		help: `  evalkit summarize
  evalkit summarize --seq 2`},
	{name: "done", args: "[SUITE] ITEM ACTION", summary: "record that the agent performed an approved action", run: runDone,
		flags: func(fs *flag.FlagSet) {
			fs.String("url", "", "link to the result, shown on the item")
			fs.String("note", "", "short note shown beside the link")
			feedbackFlag(fs)
		},
		help: `After performing an action the human approved (post, merge, deploy …), record
it. The item then shows a done link and the action cannot be approved again.

  evalkit done pr-9875 post --url https://github.com/acme/app/pull/9875`},
	benchCommands[0], benchCommands[1], benchCommands[2],
	statsCommand,
	{name: "docs", summary: "print the suite reference (markdown)", run: runDocs},
	{name: "schema", summary: "print the suite JSON Schema, for editors", run: runSchema},
	{name: "skill", summary: "print the agent skill, or install it for Claude Code", run: runSkill,
		flags: func(fs *flag.FlagSet) {
			fs.Bool("install", false, "write it to ~/.claude/skills/evalkit/SKILL.md")
		},
		help: `The skill tells an agent the whole loop: write a suite, serve, wait, act, repeat.

  evalkit skill              print it
  evalkit skill --install    install it for Claude Code`},
	{name: "version", summary: "print the version", run: runVersion},
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "evalkit:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		if len(args) > 1 {
			if c := find(args[1]); c != nil {
				usage(c, out)
				return nil
			}
		}
		help(out)
		return nil
	}
	c := find(args[0])
	if c == nil {
		return fmt.Errorf("unknown command %q (run \"evalkit help\")", args[0])
	}
	fs := flag.NewFlagSet(c.name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if c.flags != nil {
		c.flags(fs)
	}
	pos, err := parse(fs, args[1:])
	if errors.Is(err, flag.ErrHelp) {
		usage(c, out)
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w (run \"evalkit help %s\")", c.name, err, c.name)
	}
	return c.run(fs, pos)
}

func find(name string) *command {
	for _, c := range commands {
		if c.name == name {
			return c
		}
	}
	return nil
}

func help(out io.Writer) {
	fmt.Fprint(out, intro)
	for _, c := range commands {
		fmt.Fprintf(out, "  %-10s %s\n", c.name, c.summary)
	}
	fmt.Fprint(out, outro)
}

func usage(c *command, out io.Writer) {
	fmt.Fprintf(out, "Usage: evalkit %s %s\n\n%s.\n", c.name, strings.TrimSpace(c.args+" [flags]"), capitalize(c.summary))
	if c.help != "" {
		fmt.Fprintf(out, "\n%s\n", c.help)
	}
	fs := flag.NewFlagSet(c.name, flag.ContinueOnError)
	if c.flags != nil {
		c.flags(fs)
		fmt.Fprintln(out, "\nFlags:")
		fs.SetOutput(out)
		fs.PrintDefaults()
	}
}

func capitalize(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

// parse reads flags that may come before or after the positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// suiteArg takes an optional leading suite path (a .yaml, .yml or .json file)
// and defaults to ./suite.yaml.
func suiteArg(pos []string) (string, []string, error) {
	path := "suite.yaml"
	if len(pos) > 0 && isSuiteFile(pos[0]) {
		path, pos = pos[0], pos[1:]
	}
	if _, err := os.Stat(path); err != nil {
		if path == "suite.yaml" {
			return "", nil, errors.New(`no suite.yaml here: run "evalkit init" or pass a suite path`)
		}
		return "", nil, err
	}
	return path, pos, nil
}

func isSuiteFile(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".yaml", ".yml", ".json":
		return true
	}
	return false
}

func feedbackFlag(fs *flag.FlagSet) {
	fs.String("feedback", "", "feedback directory (default: feedback/ beside the suite)")
}

func str(fs *flag.FlagSet, name string) string {
	return fs.Lookup(name).Value.(flag.Getter).Get().(string)
}
func boolean(fs *flag.FlagSet, name string) bool {
	return fs.Lookup(name).Value.(flag.Getter).Get().(bool)
}
func integer(fs *flag.FlagSet, name string) int {
	return fs.Lookup(name).Value.(flag.Getter).Get().(int)
}

// want checks the number of positional arguments left after the suite path.
func want(c string, pos []string, n int) error {
	if len(pos) != n {
		return fmt.Errorf("%s: want %d argument(s) after the suite, got %d (run \"evalkit help %s\")", c, n, len(pos), c)
	}
	return nil
}
