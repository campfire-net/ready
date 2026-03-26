// Package rdconfig manages the rd configuration file (~/.campfire/rd.json).
package rdconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds rd-specific configuration persisted across sessions.
type Config struct {
	// Org is the organization name used for namespace registration (e.g. "acme").
	// The ready namespace is cf://<org>.ready.
	Org string `json:"org,omitempty"`

	// ReadyCampfireID is the hex campfire ID of the cf://<org>.ready namespace campfire.
	ReadyCampfireID string `json:"ready_campfire_id,omitempty"`
}

// Path returns the config file path within the given campfire home directory.
func Path(cfHome string) string {
	return filepath.Join(cfHome, "rd.json")
}

// Load reads the config from disk. Returns a zero Config if the file doesn't exist.
func Load(cfHome string) (*Config, error) {
	data, err := os.ReadFile(Path(cfHome))
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &c, nil
}

// Save writes the config to disk.
func Save(cfHome string, c *Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return os.WriteFile(Path(cfHome), data, 0644)
}
