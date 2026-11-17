package bench

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"

	"github.com/jryio/evalkit/internal/stats"
)

// Scored is a rows file's score under the suite's metric.
type Scored struct {
	stats.Interval
	N int // rows that count
}

// Score scores the rows of one run file that the suite does not list, such as
// a stand-in agent's throwaway run.
func Score(l *Loaded, rowsPath string) (Scored, error) {
	b, err := os.ReadFile(rowsPath)
	if err != nil {
		return Scored{}, err
	}
	s := l.Stats
	s.Runs, s.Items = nil, nil
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(nil, 64<<20)
	for sc.Scan() {
		var it stats.Item
		if json.Unmarshal(sc.Bytes(), &it) != nil || it.ID == "" {
			continue
		}
		it.Run, it.Pair = "probe", it.ID
		s.Items = append(s.Items, it)
	}
	rep := stats.Summarize(s, nil, 1)
	if rep.Metric == nil {
		return Scored{}, errors.New("nothing scores the rows: mark a question or a column primary")
	}
	if len(rep.Runs) == 0 || rep.Runs[0].Score == nil {
		return Scored{}, errors.New("no row has a value for the score: the score stages must set the primary question or column")
	}
	return Scored{Interval: *rep.Runs[0].Score, N: rep.Runs[0].N}, nil
}
