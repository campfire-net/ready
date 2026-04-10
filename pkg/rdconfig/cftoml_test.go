package rdconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProjectBeacon_MissingFile_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got, err := LoadProjectBeacon(dir)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty beacon, got: %q", got)
	}
}

func TestLoadProjectBeacon_NoRdSection_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	cfDir := filepath.Join(dir, ".cf")
	if err := os.MkdirAll(cfDir, 0700); err != nil {
		t.Fatal(err)
	}
	contents := `[transport]
relay = "https://example.test/api"
`
	if err := os.WriteFile(filepath.Join(cfDir, "config.toml"), []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadProjectBeacon(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty beacon, got: %q", got)
	}
}

func TestLoadProjectBeacon_ReadsBeaconFromRdSection(t *testing.T) {
	dir := t.TempDir()
	cfDir := filepath.Join(dir, ".cf")
	if err := os.MkdirAll(cfDir, 0700); err != nil {
		t.Fatal(err)
	}
	contents := `[rd]
beacon = "beacon:abc123"
`
	if err := os.WriteFile(filepath.Join(cfDir, "config.toml"), []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadProjectBeacon(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "beacon:abc123" {
		t.Fatalf("expected beacon:abc123, got: %q", got)
	}
}

func TestLoadProjectBeacon_ParseError(t *testing.T) {
	dir := t.TempDir()
	cfDir := filepath.Join(dir, ".cf")
	if err := os.MkdirAll(cfDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfDir, "config.toml"), []byte("not = valid = toml ="), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadProjectBeacon(dir)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestSaveProjectBeacon_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	if err := SaveProjectBeacon(dir, "beacon:xyz"); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	got, err := LoadProjectBeacon(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if got != "beacon:xyz" {
		t.Fatalf("expected beacon:xyz, got: %q", got)
	}
}

func TestSaveProjectBeacon_PreservesOtherSections(t *testing.T) {
	dir := t.TempDir()
	cfDir := filepath.Join(dir, ".cf")
	if err := os.MkdirAll(cfDir, 0700); err != nil {
		t.Fatal(err)
	}
	original := `[transport]
relay = "https://example.test/api"

[behavior]
auto_join = ["beacon:other"]
`
	cfgPath := filepath.Join(cfDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	if err := SaveProjectBeacon(dir, "beacon:newone"); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	if !strings.Contains(out, "https://example.test/api") {
		t.Errorf("expected transport.relay preserved, got:\n%s", out)
	}
	if !strings.Contains(out, "beacon:other") {
		t.Errorf("expected behavior.auto_join preserved, got:\n%s", out)
	}
	if !strings.Contains(out, "beacon:newone") {
		t.Errorf("expected new beacon written, got:\n%s", out)
	}

	got, err := LoadProjectBeacon(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "beacon:newone" {
		t.Fatalf("expected beacon:newone via Load, got: %q", got)
	}
}

func TestSaveProjectBeacon_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	if err := SaveProjectBeacon(dir, "beacon:first"); err != nil {
		t.Fatal(err)
	}
	if err := SaveProjectBeacon(dir, "beacon:second"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadProjectBeacon(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "beacon:second" {
		t.Fatalf("expected beacon:second, got: %q", got)
	}
}

func TestSaveProjectBeacon_RestrictivePermissions(t *testing.T) {
	dir := t.TempDir()
	if err := SaveProjectBeacon(dir, "beacon:perm"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(CFConfigPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected 0600, got: %#o", perm)
	}
}
