// Package rdconfig manages the rd configuration files:
//   - ~/.campfire/rd.json (global Config)
//   - <project>/.ready/config.json (project-local SyncConfig)
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

	// HomeCampfireID is the hex campfire ID of the operator root / home campfire.
	HomeCampfireID string `json:"home_campfire_id,omitempty"`

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

// DurabilityAssessment holds the result of a durability evaluation stored
// alongside sync config so callers can audit what was assessed at init time.
type DurabilityAssessment struct {
	// MeetsMinimum is true when the campfire meets the minimum durability
	// requirements (max-ttl:0 + lifecycle:persistent + provenance:basic or higher).
	MeetsMinimum bool `json:"meets_minimum"`

	// Weight is the qualitative trust weight: "high", "medium", "low",
	// "minimal", or "unknown".
	Weight string `json:"weight"`

	// MaxTTL is the parsed max-ttl value from the beacon tags ("0", "30d", etc.)
	// Empty if no max-ttl tag was present.
	MaxTTL string `json:"max_ttl,omitempty"`

	// LifecycleType is the parsed lifecycle type ("persistent", "ephemeral",
	// "bounded") or empty if no lifecycle tag was present.
	LifecycleType string `json:"lifecycle_type,omitempty"`

	// Warnings lists advisory messages from parse and trust evaluation.
	Warnings []string `json:"warnings,omitempty"`

	// ProvenanceLevel is the provenance level used for evaluation.
	ProvenanceLevel string `json:"provenance_level,omitempty"`
}

// SyncConfig holds project-local sync configuration stored in
// <project>/.ready/config.json.
type SyncConfig struct {
	// CampfireID is the hex campfire ID this project syncs to.
	CampfireID string `json:"campfire_id,omitempty"`

	// Durability is the durability assessment at configuration time.
	Durability *DurabilityAssessment `json:"durability,omitempty"`
}

// SyncConfigPath returns the path to the project-local sync config file.
func SyncConfigPath(projectDir string) string {
	return filepath.Join(projectDir, ".ready", "config.json")
}

// LoadSyncConfig reads the project-local sync config. Returns a zero SyncConfig
// if the file does not exist.
func LoadSyncConfig(projectDir string) (*SyncConfig, error) {
	data, err := os.ReadFile(SyncConfigPath(projectDir))
	if os.IsNotExist(err) {
		return &SyncConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading sync config: %w", err)
	}
	var c SyncConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing sync config: %w", err)
	}
	return &c, nil
}

// SaveSyncConfig writes the project-local sync config.
func SaveSyncConfig(projectDir string, c *SyncConfig) error {
	dir := filepath.Join(projectDir, ".ready")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating .ready dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding sync config: %w", err)
	}
	return os.WriteFile(SyncConfigPath(projectDir), data, 0644)
}
