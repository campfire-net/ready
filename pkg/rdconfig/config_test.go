package rdconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/campfire-net/ready/pkg/rdconfig"
)

func TestSave_RestrictivePermissions(t *testing.T) {
	cfHome := t.TempDir()
	c := &rdconfig.Config{Org: "testorg", HomeCampfireID: "abc123"}
	if err := rdconfig.Save(cfHome, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := rdconfig.Path(cfHome)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("rd.json permissions: got %04o, want 0600", got)
	}
}

func TestSaveSyncConfig_RestrictivePermissions(t *testing.T) {
	projectDir := t.TempDir()
	c := &rdconfig.SyncConfig{CampfireID: "deadbeef"}
	if err := rdconfig.SaveSyncConfig(projectDir, c); err != nil {
		t.Fatalf("SaveSyncConfig: %v", err)
	}

	readyDir := filepath.Join(projectDir, ".ready")

	// Directory must be owner-only (0700).
	dirInfo, err := os.Stat(readyDir)
	if err != nil {
		t.Fatalf("stat .ready: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Errorf(".ready dir permissions: got %04o, want 0700", got)
	}

	// config.json must be owner-only (0600).
	configFile := rdconfig.SyncConfigPath(projectDir)
	fileInfo, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("stat config.json: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0600 {
		t.Errorf("config.json permissions: got %04o, want 0600", got)
	}
}
