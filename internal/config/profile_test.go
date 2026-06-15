package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

// ─── WriteProfileMap tests ────────────────────────────────────────────────

func TestWriteProfileMap_NewFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission modes behave differently on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	pm := ProfileMap{"dev": "mysql://root:pass@127.0.0.1:3306/devdb"}
	if err := WriteProfileMap(path, pm); err != nil {
		t.Fatalf("WriteProfileMap: %v", err)
	}

	// Check file was created and has 0600 permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestWriteProfileMap_ExistingFilePreservesPerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission modes behave differently on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Create existing file with 0644
	if err := os.WriteFile(path, []byte("dsns:\n  x: y\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	pm := ProfileMap{"dev": "mysql://root:pass@127.0.0.1:3306/devdb"}
	if err := WriteProfileMap(path, pm); err != nil {
		t.Fatalf("WriteProfileMap: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("expected 0644 permissions to be preserved, got %o", info.Mode().Perm())
	}
}

func TestWriteProfileMap_PreservesMapOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Insert keys in non-alphabetical order
	pm := ProfileMap{
		"zebra":   "mysql://z",
		"alpha":   "mysql://a",
		"middle":  "mysql://m",
		"beta":    "mysql://b",
		"qwerty":  "mysql://q",
	}
	if err := WriteProfileMap(path, pm); err != nil {
		t.Fatalf("WriteProfileMap: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(data)

	// Verify alphabetical order: alpha < beta < middle < qwerty < zebra
	alphaIdx := strings.Index(content, "alpha:")
	betaIdx := strings.Index(content, "beta:")
	middleIdx := strings.Index(content, "middle:")
	qwertyIdx := strings.Index(content, "qwerty:")
	zebraIdx := strings.Index(content, "zebra:")

	if alphaIdx == -1 || betaIdx == -1 || middleIdx == -1 || qwertyIdx == -1 || zebraIdx == -1 {
		t.Fatalf("missing profile keys in output:\n%s", content)
	}
	if !(alphaIdx < betaIdx && betaIdx < middleIdx && middleIdx < qwertyIdx && qwertyIdx < zebraIdx) {
		t.Errorf("keys are not in alphabetical order:\n%s", content)
	}
}

func TestWriteProfileMap_EmptyMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := WriteProfileMap(path, ProfileMap{}); err != nil {
		t.Fatalf("WriteProfileMap: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Should produce valid YAML (empty dsns block)
	var cfg ProfileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("output is not valid YAML:\n%s", string(data))
	}
	if len(cfg.DSNs) != 0 {
		t.Errorf("expected empty map, got %d entries", len(cfg.DSNs))
	}
}

func TestWriteProfileMap_NilMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write nil map — should normalize to empty
	if err := WriteProfileMap(path, nil); err != nil {
		t.Fatalf("WriteProfileMap(nil): %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var cfg ProfileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("output is not valid YAML:\n%s", string(data))
	}
	if len(cfg.DSNs) != 0 {
		t.Errorf("expected empty map for nil input, got %d entries", len(cfg.DSNs))
	}
}

func TestWriteProfileMap_DirNotExist(t *testing.T) {
	dir := t.TempDir()
	// Use a path where the last segment exists as a regular file — os.MkdirAll
	// cannot create a directory at that path, so it will fail.
	filePath := filepath.Join(dir, "afile")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	target := filepath.Join(filePath, "config.yaml")

	err := WriteProfileMap(target, ProfileMap{"dev": "mysql://x"})
	if err == nil {
		t.Fatal("expected error when parent is a file (cannot create directory)")
	}
	if !strings.Contains(err.Error(), "create parent directory") && !strings.Contains(err.Error(), "afile") {
		t.Errorf("error should mention the parent directory issue, got: %v", err)
	}
}

func TestWriteProfileMap_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// First write
	pm1 := ProfileMap{"first": "mysql://first"}
	if err := WriteProfileMap(path, pm1); err != nil {
		t.Fatalf("WriteProfileMap (first): %v", err)
	}

	// Overwrite with new content
	pm2 := ProfileMap{"second": "mysql://second"}
	if err := WriteProfileMap(path, pm2); err != nil {
		t.Fatalf("WriteProfileMap (overwrite): %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "first") {
		t.Errorf("overwritten file should not contain 'first' key:\n%s", content)
	}
	if !strings.Contains(content, "second") {
		t.Errorf("overwritten file should contain 'second' key:\n%s", content)
	}
}
