package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// RegradeOptions says what to grade again.
type RegradeOptions struct {
	Run         string
	Score       bool     // run the score stages again
	Judge       bool     // run the judge again
	Only        []string // only these case ids
	RunsDir     string
	Concurrency int
	Log         io.Writer
}

// Regrade grades a finished run's stored attempts again without running the
// agent: after a rubric or a scorer changed. Each regraded row replaces the
// earlier one. It returns how many rows it wrote and what the judge cost.
func Regrade(ctx context.Context, l *Loaded, opts RegradeOptions) (rows int, cost float64, err error) {
	if opts.RunsDir == "" {
		opts.RunsDir = filepath.Join(l.Dir, "runs")
	}
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	r := &runner{l: l, cfg: l.Config, opts: Options{Run: opts.Run, RunsDir: opts.RunsDir}, runDir: filepath.Join(opts.RunsDir, opts.Run)}
	if opts.Judge && r.cfg.Judge == nil {
		return 0, 0, fmt.Errorf("the suite has no bench.judge")
	}
	files, err := filepath.Glob(filepath.Join(r.runDir, "attempts", "*.json"))
	if err != nil {
		return 0, 0, err
	}
	if len(files) == 0 {
		return 0, 0, fmt.Errorf("run %s has no stored attempts in %s", opts.Run, r.runDir)
	}
	limit := time.Duration(r.cfg.Timeout * float64(time.Second))
	conc := opts.Concurrency
	if conc <= 0 {
		conc = r.cfg.Concurrency
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return rows, cost, err
		}
		var p payload
		if err := json.Unmarshal(b, &p); err != nil {
			return rows, cost, fmt.Errorf("%s: %w", f, err)
		}
		if len(opts.Only) > 0 && !slices.Contains(opts.Only, p.Case.ID()) {
			continue
		}
		// The slot is long gone: stages run in a scratch directory with the stored payload.
		p.Out = strings.TrimSuffix(f, ".json")
		p.Workspace, p.Cwd = "", p.Out
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			spent, err := r.regradeOne(ctx, p, opts, limit)
			mu.Lock()
			defer mu.Unlock()
			cost += spent
			if err != nil {
				fmt.Fprintf(opts.Log, "  %-28s FAILED: %v\n", attemptKey(p.Case.ID(), p.Rep), err)
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			if err := appendLine(r.rowsPath(), p.Row); err != nil && firstErr == nil {
				firstErr = err
				return
			}
			rows++
			fmt.Fprintf(opts.Log, "  %-28s regraded\n", attemptKey(p.Case.ID(), p.Rep))
		})
	}
	wg.Wait()
	return rows, cost, firstErr
}

func (r *runner) regradeOne(ctx context.Context, p payload, opts RegradeOptions, limit time.Duration) (float64, error) {
	if err := os.MkdirAll(filepath.Join(p.Out, "tmp"), 0o755); err != nil {
		return 0, err
	}
	defer savePayload(p)
	env := r.env(p)
	if opts.Score {
		for i, command := range r.cfg.Score {
			frag, err := r.stage(ctx, fmt.Sprintf("score[%d]", i), command, p, env, limit)
			if err != nil {
				return 0, err
			}
			if err := merge(p.Row, frag); err != nil {
				return 0, err
			}
		}
	}
	if !opts.Judge {
		return 0, nil
	}
	if flag, _ := p.Row["flag"].(string); strings.HasPrefix(flag, judgeFailed) {
		delete(p.Row, "flag")
	}
	cost, err := r.judge(ctx, p, env, limit)
	if err != nil {
		p.Row["flag"] = judgeFailed + err.Error()
	}
	if values, ok := p.Row["values"].(map[string]any); ok && r.l.Columns["judge_cost_usd"] && cost > 0 {
		values["judge_cost_usd"] = round(cost, 4)
	}
	return cost, nil
}
