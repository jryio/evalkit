// Package stats scores the runs of a suite the way the review page does: a
// rate or mean per run with a 95% interval, and a paired comparison between runs.
package stats

import (
	"math"
	"slices"
)

// Interval is a value with its 95% interval.
type Interval struct {
	Value float64 `json:"value"`
	Lo    float64 `json:"lo"`
	Hi    float64 `json:"hi"`
}

// Wilson is the Wilson score interval for k successes in n trials. ok is false for n = 0.
func Wilson(k float64, n int) (iv Interval, ok bool) {
	if n == 0 {
		return iv, false
	}
	const z = 1.96
	nf := float64(n)
	p, z2 := k/nf, z*z
	d := 1 + z2/nf
	mid := (p + z2/(2*nf)) / d
	half := (z / d) * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf))
	return Interval{Value: p, Lo: math.Max(0, mid-half), Hi: math.Min(1, mid+half)}, true
}

// rng is mulberry32, the page's generator: the same seed gives the page's intervals.
func rng(seed uint32) func() float64 {
	a := seed
	return func() float64 {
		a += 0x6d2b79f5
		t := (a ^ a>>15) * (a | 1)
		t ^= t + (t^t>>7)*(t|61)
		return float64(t^t>>14) / 4294967296
	}
}

// quantile interpolates linearly between order statistics (R's type 7).
func quantile(sorted []float64, q float64) float64 {
	h := float64(len(sorted)-1) * q
	lo := int(math.Floor(h))
	if lo+1 < len(sorted) {
		return sorted[lo] + (h-float64(lo))*(sorted[lo+1]-sorted[lo])
	}
	return sorted[lo]
}

// BootstrapMean is a percentile interval for the mean of xs. clusters gives a
// key per x; whole clusters are resampled, so the reps of one case move
// together. A nil clusters treats every x as its own cluster.
func BootstrapMean(xs []float64, clusters []string, seed uint32) (iv Interval, ok bool) {
	if len(xs) == 0 {
		return iv, false
	}
	var units [][]float64
	if clusters == nil {
		for _, x := range xs {
			units = append(units, []float64{x})
		}
	} else {
		index := map[string]int{}
		for i, x := range xs {
			j, seen := index[clusters[i]]
			if !seen {
				j = len(units)
				index[clusters[i]] = j
				units = append(units, nil)
			}
			units[j] = append(units[j], x)
		}
	}
	const rounds = 2000
	next := rng(seed)
	means := make([]float64, 0, rounds)
	for range rounds {
		var sum float64
		var count int
		for range units {
			u := units[int(next()*float64(len(units)))]
			for _, x := range u {
				sum += x
			}
			count += len(u)
		}
		means = append(means, sum/float64(count))
	}
	slices.Sort(means)
	var total float64
	for _, x := range xs {
		total += x
	}
	return Interval{Value: total / float64(len(xs)), Lo: quantile(means, 0.025), Hi: quantile(means, 0.975)}, true
}

// McNemar is the exact two-sided test on discordant pairs: Binomial(wins + losses, 0.5).
func McNemar(wins, losses int) float64 {
	n := wins + losses
	if n == 0 {
		return 1
	}
	logTerm := -float64(n) * math.Ln2
	var tail float64
	for i := 0; i <= min(wins, losses); i++ {
		tail += math.Exp(logTerm)
		logTerm += math.Log(float64(n-i) / float64(i+1))
	}
	return math.Min(1, 2*tail)
}
