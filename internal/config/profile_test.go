package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleConfig = `
dsns:
  dev:     mysql://root:pass@127.0.0.1:3306/devdb
  staging: mysql://readonly:pw@db.staging.example.com:3306/?parseTime=true
`

func TestLoadProfileMap_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".sql-cli.yaml")
	if err := os.WriteFile(path, []byte(sampleConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	pm, err := LoadProfileMap(path)
	if err != nil {
		t.Fatalf("LoadProfileMap: %v", err)
	}
	if len(pm) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(pm))
	}
	if pm["dev"] != "mysql://root:pass@127.0.0.1:3306/devdb" {
		t.Errorf("dev profile mismatch: %q", pm["dev"])
	}
	if !strings.Contains(pm["staging"], "parseTime=true") {
		t.Errorf("staging profile missing query params: %q", pm["staging"])
	}
}

func TestLoadProfileMap_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	// Unclosed quote — definitely not valid YAML.
	if err := os.WriteFile(path, []byte("dsns:\n  dev: 'unterminated\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadProfileMap(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error should mention the file path %q, got: %v", path, err)
	}
}

func TestLoadProfileMap_InvalidDSN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-dsn.yaml")
	// Wrong scheme — ValidateMySQLURL should reject this at load time.
	yaml := "dsns:\n  dev: http://user:pass@host:3306/db\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadProfileMap(path)
	if err == nil {
		t.Fatal("expected error for invalid DSN, got nil")
	}
	if !strings.Contains(err.Error(), "dev") {
		t.Errorf("error should mention the offending profile name, got: %v", err)
	}
}

func TestLoadProfileMap_EmptyPath(t *testing.T) {
	_, err := LoadProfileMap("")
	if !errors.Is(err, ErrNoConfigFound) {
		t.Errorf("expected ErrNoConfigFound, got: %v", err)
	}
}

func TestLoadProfileMap_MissingFile(t *testing.T) {
	_, err := LoadProfileMap("/nonexistent/path/.sql-cli.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestMergeProfiles_OverrideByName(t *testing.T) {
	base := ProfileMap{
		"dev":     "mysql://base-dev",
		"staging": "mysql://base-staging",
	}
	override := ProfileMap{
		"dev":  "mysql://local-dev", // override same name
		"prod": "mysql://local-prod", // new name
	}
	merged := MergeProfiles(base, override)
	if len(merged) != 3 {
		t.Errorf("expected 3 profiles, got %d", len(merged))
	}
	if merged["dev"] != "mysql://local-dev" {
		t.Errorf("expected override to win for dev, got %q", merged["dev"])
	}
	if merged["staging"] != "mysql://base-staging" {
		t.Errorf("expected base staging to remain, got %q", merged["staging"])
	}
	if merged["prod"] != "mysql://local-prod" {
		t.Errorf("expected new prod to appear, got %q", merged["prod"])
	}
}

func TestMergeProfiles_NilSafe(t *testing.T) {
	if m := MergeProfiles(nil, nil); len(m) != 0 {
		t.Errorf("nil+nil should be empty, got %d entries", len(m))
	}
	base := ProfileMap{"a": "x"}
	if m := MergeProfiles(base, nil); m["a"] != "x" {
		t.Errorf("base+nil should preserve base, got %v", m)
	}
	if m := MergeProfiles(nil, base); m["a"] != "x" {
		t.Errorf("nil+base should produce base, got %v", m)
	}
}

func TestFindLocalConfigPath_WalksUpward(t *testing.T) {
	// Create a 3-deep directory tree with .sql-cli.yaml at the middle level.
	root := t.TempDir()
	middle := filepath.Join(root, "a", "b")
	deep := filepath.Join(middle, "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(middle, ".sql-cli.yaml")
	if err := os.WriteFile(configPath, []byte("dsns:\n  x: y\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := FindLocalConfigPath(deep)
	want, _ := filepath.Abs(configPath)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFindLocalConfigPath_StopsAtRoot(t *testing.T) {
	got := FindLocalConfigPath(t.TempDir())
	if got != "" {
		t.Errorf("expected empty string when no config exists, got %q", got)
	}
}

func TestResolveProfile_Found(t *testing.T) {
	pm := ProfileMap{
		"dev":  "mysql://dev",
		"prod": "mysql://prod",
	}
	dsn, err := ResolveProfile(pm, "prod")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if dsn != "mysql://prod" {
		t.Errorf("got %q, want mysql://prod", dsn)
	}
}

func TestResolveProfile_NotFound_ListsAvailable(t *testing.T) {
	pm := ProfileMap{
		"dev":  "mysql://dev",
		"prod": "mysql://prod",
	}
	_, err := ResolveProfile(pm, "staging")
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
	if !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("expected ErrProfileNotFound, got: %v", err)
	}
	if !strings.Contains(err.Error(), "staging") {
		t.Errorf("error should mention the requested name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "dev") || !strings.Contains(err.Error(), "prod") {
		t.Errorf("error should list available profiles, got: %v", err)
	}
}

func TestResolveProfile_EmptyName(t *testing.T) {
	_, err := ResolveProfile(ProfileMap{"dev": "x"}, "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestResolveProfile_EmptyMap(t *testing.T) {
	_, err := ResolveProfile(ProfileMap{}, "any")
	if err == nil {
		t.Fatal("expected error when map is empty")
	}
	if !strings.Contains(err.Error(), "(none)") {
		t.Errorf("expected '(none)' for empty available list, got: %v", err)
	}
}
