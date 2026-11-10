package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/jryio/evalkit"
	"github.com/jryio/evalkit/internal/feedback"
	"github.com/jryio/evalkit/internal/server"
	"github.com/jryio/evalkit/internal/suite"
	"github.com/jryio/evalkit/internal/summary"
	"github.com/jryio/evalkit/web"
)

func runInit(f *flag.FlagSet, pos []string) error {
	if len(pos) > 1 {
		return want("init", pos, 1)
	}
	dir := "."
	if len(pos) == 1 {
		dir = pos[0]
	}
	src := "examples/starter"
	if boolean(f, "example") {
		src = "examples/kitchen-sink"
	}
	target := filepath.Join(dir, "suite.yaml")
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%s already exists", target)
	}
	sub, err := fs.Sub(evalkit.Files, src)
	if err != nil {
		return err
	}
	if err := os.CopyFS(dir, sub); err != nil {
		return err
	}
	serve := "evalkit serve"
	if dir != "." {
		serve += " " + target
	}
	fmt.Printf("Wrote %s\n\nNext:\n  %-34s review it in the browser\n  %-34s the reference for every kind\n", target, serve, "evalkit docs")
	return nil
}

func runValidate(_ *flag.FlagSet, pos []string) error {
	path, pos, err := suiteArg(pos)
	if err != nil {
		return err
	}
	if err := want("validate", pos, 0); err != nil {
		return err
	}
	res, err := suite.Load(path)
	if err != nil {
		return err
	}
	errs := 0
	for _, p := range res.Problems {
		fmt.Println(p)
		if !p.Warning {
			errs++
		}
	}
	if errs > 0 {
		return fmt.Errorf("%d error(s) in %s", errs, path)
	}
	items, _ := res.Doc.Get("items")
	fmt.Printf("ok: %s has %d items\n", path, len(items.([]any)))
	return nil
}

func runServe(f *flag.FlagSet, pos []string) error {
	path, pos, err := suiteArg(pos)
	if err != nil {
		return err
	}
	if err := want("serve", pos, 0); err != nil {
		return err
	}
	fb, err := openFeedback(path, f)
	if err != nil {
		return err
	}
	var files fs.FS = web.FS
	if dir := str(f, "web"); dir != "" {
		files = os.DirFS(dir)
	}
	ln, err := listen(integer(f, "port"))
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String() + "/"
	if res, err := suite.Load(path); err == nil {
		for _, p := range res.Problems {
			fmt.Println(p)
		}
	}
	fmt.Printf("Review page: %s\nFeedback:    %s\nWaiting for sends: evalkit wait %s\n", url, fb.Path(), path)
	if !boolean(f, "no-open") {
		openBrowser(url)
	}

	srv := &http.Server{Handler: server.New(path, fb, files).Handler()}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// listen binds loopback only, trying port and the next 9.
func listen(port int) (net.Listener, error) {
	var lastErr error
	for p := port; p < port+10; p++ {
		ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
		if err == nil {
			return ln, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func openBrowser(url string) {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	exec.Command(name, url).Start()
}

func runWait(f *flag.FlagSet, pos []string) error {
	path, pos, err := suiteArg(pos)
	if err != nil {
		return err
	}
	if err := want("wait", pos, 0); err != nil {
		return err
	}
	fb, err := openFeedback(path, f)
	if err != nil {
		return err
	}
	seen := fb.Seen()
	latest, err := fb.Poll(context.Background(), seen, 500*time.Millisecond)
	if err != nil {
		return err
	}
	if err := printSummary(fb, latest, seen); err != nil {
		return err
	}
	return fb.SetSeen(latest)
}

func runSummarize(f *flag.FlagSet, pos []string) error {
	path, pos, err := suiteArg(pos)
	if err != nil {
		return err
	}
	if err := want("summarize", pos, 0); err != nil {
		return err
	}
	fb, err := openFeedback(path, f)
	if err != nil {
		return err
	}
	n := integer(f, "seq")
	if n == 0 {
		n = fb.Latest()
	}
	if n == 0 {
		return errors.New("nothing has been sent yet")
	}
	return printSummary(fb, n, n-1)
}

// printSummary prints snapshot seq diffed against snapshot base (0: none).
func printSummary(fb *feedback.Dir, seq, base int) error {
	cur, err := fb.ReadSnapshot(seq)
	if err != nil {
		return err
	}
	var prev map[string]any
	if base > 0 {
		if prev, err = fb.ReadSnapshot(base); err != nil {
			return err
		}
	}
	summary.Write(os.Stdout, cur, prev, fb.SnapshotPath(seq))
	return nil
}

func runDone(f *flag.FlagSet, pos []string) error {
	path, pos, err := suiteArg(pos)
	if err != nil {
		return err
	}
	if err := want("done", pos, 2); err != nil {
		return err
	}
	fb, err := openFeedback(path, f)
	if err != nil {
		return err
	}
	if err := fb.RecordDone(pos[0], pos[1], feedback.Done{URL: str(f, "url"), Note: str(f, "note")}); err != nil {
		return err
	}
	fmt.Printf("recorded %s on %s\n", pos[1], pos[0])
	return nil
}

func runDocs(*flag.FlagSet, []string) error { return printEmbedded("docs/schema.md") }

func runSchema(*flag.FlagSet, []string) error {
	_, err := os.Stdout.Write(suite.SchemaJSON)
	return err
}

func runSkill(f *flag.FlagSet, _ []string) error {
	if !boolean(f, "install") {
		return printEmbedded("skill/SKILL.md")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	b, err := evalkit.Files.ReadFile("skill/SKILL.md")
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".claude", "skills", "evalkit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(target, b, 0o644); err != nil {
		return err
	}
	fmt.Println("installed", target)
	return nil
}

func runVersion(*flag.FlagSet, []string) error {
	v := "(devel)"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		v = info.Main.Version
	}
	fmt.Println("evalkit", v)
	return nil
}

func printEmbedded(name string) error {
	b, err := evalkit.Files.ReadFile(name)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(b)
	return err
}

func openFeedback(suitePath string, f *flag.FlagSet) (*feedback.Dir, error) {
	dir := str(f, "feedback")
	if dir == "" {
		dir = feedback.ForSuite(suitePath)
	}
	return feedback.Open(dir)
}
