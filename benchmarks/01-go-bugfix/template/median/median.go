package median

import "sort"

// Median returns the middle value of xs.
func Median(xs []float64) float64 {
	sort.Float64s(xs)
	return xs[len(xs)/2]
}
