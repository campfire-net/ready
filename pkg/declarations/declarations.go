// Package declarations embeds the work management convention:operation JSON
// declarations and provides functions to post them to a campfire.
package declarations

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed ops/*.json
var opsFS embed.FS

// All returns all convention:operation declaration payloads as raw JSON.
func All() ([][]byte, error) {
	entries, err := fs.ReadDir(opsFS, "ops")
	if err != nil {
		return nil, fmt.Errorf("reading embedded ops: %w", err)
	}
	var payloads [][]byte
	for _, e := range entries {
		data, err := opsFS.ReadFile("ops/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		payloads = append(payloads, data)
	}
	return payloads, nil
}
