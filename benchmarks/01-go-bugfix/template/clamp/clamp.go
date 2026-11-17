package clamp

// Clamp limits v to the range [lo, hi].
func Clamp(v, lo, hi int) int {
	if v < lo {
		return hi
	}
	if v > hi {
		return lo
	}
	return v
}
