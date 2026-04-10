// cftoml.go — read/write the [rd] section of a project's .cf/config.toml.
//
// The campfire SDK config cascade (protocol.LoadConfig) parses a fixed set of
// sections (identity, store, transport, naming, behavior, scope) and silently
// ignores any others. rd uses an [rd] section in the same file to store
// project-local rd state that needs to be portable across machines, primarily
// the project beacon for zero-ceremony multi-machine onboarding.
//
// We deliberately do NOT walk the cf cascade for the [rd] section: beacons
// are project-specific (each project has its own campfire), so a global or
// ancestor [rd].beacon would be meaningless. Only ./.cf/config.toml is read.
//
// Writes use parse → modify → re-encode (BurntSushi/toml). This loses comments
// and reorders sections, matching how `cf config set` writes its own config.

package rdconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// CFConfigPath returns the path to the project's .cf/config.toml.
func CFConfigPath(projectDir string) string {
	return filepath.Join(projectDir, ".cf", "config.toml")
}

// LoadProjectBeacon reads [rd].beacon from <projectDir>/.cf/config.toml.
// Returns "" with no error if the file does not exist or the section is missing.
// Returns an error only on TOML parse failures.
func LoadProjectBeacon(projectDir string) (string, error) {
	path := CFConfigPath(projectDir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	var raw map[string]interface{}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return "", fmt.Errorf("parsing %s: %w", path, err)
	}

	rdSection, ok := raw["rd"].(map[string]interface{})
	if !ok {
		return "", nil
	}
	beacon, ok := rdSection["beacon"].(string)
	if !ok {
		return "", nil
	}
	return beacon, nil
}

// SaveProjectBeacon writes [rd].beacon = "<beacon>" into <projectDir>/.cf/config.toml,
// preserving any other sections present in the file. Creates the file (and the
// .cf/ parent directory) if they do not exist. Existing comments and section
// ordering are NOT preserved — this matches `cf config set` behavior.
func SaveProjectBeacon(projectDir string, beaconStr string) error {
	path := CFConfigPath(projectDir)

	raw := make(map[string]interface{})
	if data, err := os.ReadFile(path); err == nil {
		if _, err := toml.Decode(string(data), &raw); err != nil {
			return fmt.Errorf("parsing existing %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	rdSection, _ := raw["rd"].(map[string]interface{})
	if rdSection == nil {
		rdSection = make(map[string]interface{})
	}
	rdSection["beacon"] = beaconStr
	raw["rd"] = rdSection

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating .cf dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	if err := enc.Encode(raw); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
