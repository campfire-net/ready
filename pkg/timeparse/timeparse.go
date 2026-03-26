// Package timeparse provides relative and absolute time parsing for rd CLI flags.
// Supported formats:
//   - "2h"         → now + 2 hours
//   - "3d"         → now + 3 days
//   - "tomorrow"   → next day 09:00 UTC
//   - "next week"  → next Monday 09:00 UTC
//   - RFC3339      → passthrough
//   - YYYY-MM-DD   → that date 09:00 UTC
package timeparse

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parse parses a time expression and returns an RFC3339 UTC string.
// now is the reference time (typically time.Now()).
func Parse(expr string, now time.Time) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("empty time expression")
	}

	// RFC3339 passthrough: try to parse directly.
	if t, err := time.Parse(time.RFC3339, expr); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}

	// Named expressions.
	switch strings.ToLower(expr) {
	case "tomorrow":
		t := now.UTC().Add(24 * time.Hour)
		t = time.Date(t.Year(), t.Month(), t.Day(), 9, 0, 0, 0, time.UTC)
		return t.Format(time.RFC3339), nil
	case "next week":
		// Next Monday 09:00 UTC.
		t := now.UTC()
		daysUntilMonday := int(time.Monday - t.Weekday())
		if daysUntilMonday <= 0 {
			daysUntilMonday += 7
		}
		t = t.Add(time.Duration(daysUntilMonday) * 24 * time.Hour)
		t = time.Date(t.Year(), t.Month(), t.Day(), 9, 0, 0, 0, time.UTC)
		return t.Format(time.RFC3339), nil
	}

	// YYYY-MM-DD.
	if t, err := time.Parse("2006-01-02", expr); err == nil {
		t = time.Date(t.Year(), t.Month(), t.Day(), 9, 0, 0, 0, time.UTC)
		return t.Format(time.RFC3339), nil
	}

	// Relative: <N>h or <N>d.
	lower := strings.ToLower(expr)
	if strings.HasSuffix(lower, "h") {
		numStr := expr[:len(expr)-1]
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return "", fmt.Errorf("invalid hours %q: %w", numStr, err)
		}
		if n < 0 {
			return "", fmt.Errorf("negative duration not allowed: %q", expr)
		}
		t := now.UTC().Add(time.Duration(n) * time.Hour)
		return t.Format(time.RFC3339), nil
	}
	if strings.HasSuffix(lower, "d") {
		numStr := expr[:len(expr)-1]
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return "", fmt.Errorf("invalid days %q: %w", numStr, err)
		}
		if n < 0 {
			return "", fmt.Errorf("negative duration not allowed: %q", expr)
		}
		t := now.UTC().Add(time.Duration(n) * 24 * time.Hour)
		return t.Format(time.RFC3339), nil
	}

	return "", fmt.Errorf("unrecognized time format %q: use 2h, 3d, tomorrow, next week, RFC3339, or YYYY-MM-DD", expr)
}
