package wordcount

import "strings"

// Count returns the number of words in s.
func Count(s string) int {
	return len(strings.Split(s, " "))
}
