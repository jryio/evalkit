package bench

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// sandboxWrap returns the command line that runs an agent under the macOS
// sandbox. The agent may write only in its own paths and the places every
// program needs (temp, caches, the agent CLIs' own state), and may neither
// read nor write the protected paths: the suite, the runs and the other slots.
// The profile is written under dir.
func sandboxWrap(dir string, own, protected []string) ([]string, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("bench.sandbox needs macOS; elsewhere, isolate with the agent's own sandbox or a container")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	general := []string{"/dev", "/private/tmp", "/private/var/folders", "/private/var/tmp",
		filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json"), filepath.Join(home, ".codex"), filepath.Join(home, ".pi"),
		filepath.Join(home, ".cache"), filepath.Join(home, ".config"), filepath.Join(home, ".local"), filepath.Join(home, ".npm"),
		filepath.Join(home, "Library", "Caches"), filepath.Join(home, "Library", "Application Support"), filepath.Join(home, "Library", "Keychains"),
		filepath.Join(home, "go")}
	// The user's keys and personal agent files: nothing an agent under test should read.
	for _, private := range []string{".ssh", ".aws", ".gnupg", ".netrc", ".config/gh", ".docker", ".kube", ".agents", ".claude/skills", ".claude/CLAUDE.md", ".codex/skills", ".pi/agent/skills"} {
		protected = append(protected, filepath.Join(home, private))
	}
	var b strings.Builder
	// The last matching rule wins: general rules first, then the protected
	// paths, then the attempt's own paths, which sit inside protected ones.
	b.WriteString("(version 1)\n(allow default)\n(deny file-write* (subpath \"/\"))\n")
	rule(&b, "allow file-write*", general)
	rule(&b, "deny file-read* file-write*", protected)
	rule(&b, "allow file-read* file-write*", own)
	profile := filepath.Join(dir, "sandbox.sb")
	if err := os.WriteFile(profile, []byte(b.String()), 0o644); err != nil {
		return nil, err
	}
	return []string{"sandbox-exec", "-f", profile}, nil
}

func rule(b *strings.Builder, verb string, paths []string) {
	fmt.Fprintf(b, "(%s", verb)
	for _, p := range paths {
		// The sandbox matches real paths: /var is /private/var on macOS.
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		fmt.Fprintf(b, " (subpath %q)", p)
	}
	b.WriteString(")\n")
}
