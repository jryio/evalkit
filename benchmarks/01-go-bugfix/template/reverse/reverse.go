package reverse

// Reverse returns the elements of s in the opposite order.
func Reverse(s []int) []int {
	out := make([]int, len(s))
	for i := 1; i < len(s); i++ {
		out[len(s)-i] = s[i]
	}
	return out
}
