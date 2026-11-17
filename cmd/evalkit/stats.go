package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/jryio/evalkit/internal/stats"
	"github.com/jryio/evalkit/internal/suite"
)

var statsCommand = &command{name: "stats", args: "[SUITE]", summary: "print each run's score, interval and change", run: runStats,
	flags: func(fs *flag.FlagSet) {
		fs.Bool("json", false, "print JSON, for a script or an agent")
		fs.String("group", "", "count only these groups (comma-separated)")
	},
	help: `Scores every run the way the Runs view does, from the proposals and measured
values in the rows: the score with its 95% interval, the mean spend, and each
run paired case by case with the baseline run (else the run before it).

  evalkit stats
  evalkit stats --group test --json`}

// loadReport loads a suite and scores its runs over groups (nil: every item).
func loadReport(path string, groups []string) (stats.Report, error) {
	res, err := suite.Load(path)
	if err != nil {
		return stats.Report{}, err
	}
	if res.Fatal {
		return stats.Report{}, fmt.Errorf("%s has errors: run \"evalkit validate\"", path)
	}
	s, err := stats.Decode(res.Doc)
	if err != nil {
		return stats.Report{}, err
	}
	return stats.Summarize(s, groups, 1), nil
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func runStats(f *flag.FlagSet, pos []string) error {
	path, pos, err := suiteArg(pos)
	if err != nil {
		return err
	}
	if err := want("stats", pos, 0); err != nil {
		return err
	}
	rep, err := loadReport(path, splitList(str(f, "group")))
	if err != nil {
		return err
	}
	if boolean(f, "json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	if rep.Metric == nil {
		return errors.New("nothing scores the runs: mark a question or a column primary")
	}
	writeReport(os.Stdout, rep)
	return nil
}

func writeReport(out io.Writer, rep stats.Report) {
	fmt.Fprintf(out, "Score: %s (%s, %s is better)\n", rep.Metric.Label, rep.Metric.Kind, rep.Metric.Better)
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RUN\tN\tSCORE\t95% INTERVAL\tCHANGE\tUP\tDOWN\tSPEND")
	for _, r := range rep.Runs {
		score, interval, change, up, down, spend := "–", "–", "–", "–", "–", "–"
		if r.Score != nil {
			score = fmt.Sprintf("%.3f", r.Score.Value)
			interval = fmt.Sprintf("%.3f to %.3f", r.Score.Lo, r.Score.Hi)
		}
		if v := r.Versus; v != nil {
			change = fmt.Sprintf("%+.3f vs %s", v.Delta.Value, v.Run)
			if !v.Significant {
				change += " (within noise)"
			}
			up, down = fmt.Sprint(v.Up), fmt.Sprint(v.Down)
		}
		if r.Spend != nil {
			spend = fmt.Sprintf("%.4g", *r.Spend)
		}
		name := r.Run
		if r.Baseline {
			name += " (baseline)"
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n", name, r.N, score, interval, change, up, down, spend)
	}
	w.Flush()
}
