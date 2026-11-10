package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelp_ListsEveryCommand(t *testing.T) {
	var out bytes.Buffer
	if err := run(nil, &out); err != nil {
		t.Fatal(err)
	}
	for _, c := range commands {
		if !strings.Contains(out.String(), "  "+c.name+" ") {
			t.Errorf("help does not list %q:\n%s", c.name, out.String())
		}
	}
}

func TestHelp_PerCommand(t *testing.T) {
	for _, args := range [][]string{{"help", "serve"}, {"serve", "-h"}} {
		var out bytes.Buffer
		if err := run(args, &out); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		for _, want := range []string{"Usage: evalkit serve [SUITE] [flags]", "-port", "evalkit serve evals/canary/suite.yaml"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%v output lacks %q:\n%s", args, want, out.String())
			}
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	err := run([]string{"srve"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `unknown command "srve"`) {
		t.Errorf("err = %v, want unknown command", err)
	}
}

func TestInit_WritesValidSuites(t *testing.T) {
	for _, args := range [][]string{{"init"}, {"init", "--example"}} {
		dir := t.TempDir()
		if err := run(append(args, dir), &bytes.Buffer{}); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if err := run([]string{"validate", filepath.Join(dir, "suite.yaml")}, &bytes.Buffer{}); err != nil {
			t.Errorf("%v wrote an invalid suite: %v", args, err)
		}
		if err := run(append(args, dir), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Errorf("%v twice: err = %v, want already exists", args, err)
		}
	}
}

func TestSuiteArg_DefaultsToSuiteYAML(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, _, err := suiteArg(nil); err == nil || !strings.Contains(err.Error(), `run "evalkit init"`) {
		t.Errorf("err = %v, want a hint to run evalkit init", err)
	}
	if err := os.WriteFile("suite.yaml", []byte("title: x\nitems: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, rest, err := suiteArg([]string{"pr-1", "post"})
	if err != nil || path != "suite.yaml" || len(rest) != 2 {
		t.Errorf("suiteArg = %q, %v, %v; want suite.yaml and both args kept", path, rest, err)
	}
}
