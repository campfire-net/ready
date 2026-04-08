package main

// truncateID safely truncates a string to maxLen characters, appending "..." if truncated.
// Unlike direct string slicing (s[:12]), this does not panic if the string is shorter than maxLen.
func truncateID(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
