package app

import "strings"

// containsSubstring is a tiny readability wrapper used across this
// package's tests to assert that a rendered view contains expected text.
func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}
