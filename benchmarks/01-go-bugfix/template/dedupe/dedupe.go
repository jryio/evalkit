package dedupe

// Dedupe returns the distinct strings of s in first-seen order.
func Dedupe(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range s {
		if seen[v] {
			seen[v] = false
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
