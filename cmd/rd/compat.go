package main

// statusAliases maps bd-style status names to canonical rd status values.
// These aliases exist to ease migration from bd to rd.
// Shared by update.go (input validation) and list.go (filter resolution).
var statusAliases = map[string]string{
	"in_progress": "active",
	"in-progress": "active",
	"open":        "inbox",
	"closed":      "done",
	"completed":   "done",
}

// resolveStatus returns the canonical status for a given status string,
// resolving any bd-compat alias. If the string is already canonical or
// unrecognised, it is returned unchanged.
func resolveStatus(s string) string {
	if canonical, ok := statusAliases[s]; ok {
		return canonical
	}
	return s
}
