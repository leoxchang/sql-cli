package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProfileMap maps profile name (e.g. "dev", "staging") to its DSN value.
type ProfileMap map[string]string

// ProfileSource identifies which file a profile came from. Used in error
// messages so the user can locate the offending entry.
type ProfileSource struct {
	Name string // profile name
	Path string // absolute path of the file it was loaded from
}

// ProfileConfig is the on-disk shape we accept. Unknown top-level keys are
// preserved as raw nodes (not parsed) so a future version can add fields
// without breaking older binaries; we simply ignore them today.
//
//	dsns:
//	  dev:     mysql://u:p@host:3306/dev
//	  staging: mysql://u:p@host:3306/staging
type ProfileConfig struct {
	DSNs ProfileMap `yaml:"dsns"`
}

// configFileNames lists the filenames we look for, in priority order, when
// scanning a directory for a local config. Lower index wins on conflict.
var configFileNames = []string{".sql-cli.yaml"}

// Errors returned by this package. All are returned with %w wrapping so the
// caller can errors.Is() against these sentinels.
var (
	ErrProfileNotFound = errors.New("profile not found")
	ErrNoConfigFound   = errors.New("no config file found")
)

// FindGlobalConfigPath returns the first existing global config file path in
// the precedence order documented in design D5 (revised):
//
//  1. $SQL_CLI_CONFIG_DIR/config.yaml
//  2. $XDG_CONFIG_HOME/sql-cli/config.yaml
//  3. $HOME/.sql-cli/config.yaml
//
// Returns the empty string (no error) when no file exists at any of those
// paths; the CLI treats that as "no global config" rather than a failure.
func FindGlobalConfigPath() string {
	candidates := globalConfigCandidates()
	for _, p := range candidates {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func globalConfigCandidates() []string {
	var out []string
	if dir := os.Getenv("SQL_CLI_CONFIG_DIR"); dir != "" {
		out = append(out, filepath.Join(dir, "config.yaml"))
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		out = append(out, filepath.Join(dir, "sql-cli", "config.yaml"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, filepath.Join(home, ".sql-cli", "config.yaml"))
	}
	return out
}

// FindLocalConfigPath walks upward from startDir looking for a local config
// file (.sql-cli.yaml). It stops at the first match or at the filesystem root.
// Returns the empty string when no local file exists.
func FindLocalConfigPath(startDir string) string {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	for {
		for _, name := range configFileNames {
			candidate := filepath.Join(dir, name)
			if fileExists(candidate) {
				return candidate
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // reached filesystem root
		}
		dir = parent
	}
}

// LoadProfileMap reads a config file and returns the parsed profile map plus
// the path it was loaded from (so error messages can point at the source).
// Returns ErrNoConfigFound when path is empty.
func LoadProfileMap(path string) (ProfileMap, error) {
	if path == "" {
		return nil, ErrNoConfigFound
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}
	return parseProfileMap(data, path)
}

func parseProfileMap(data []byte, source string) (ProfileMap, error) {
	var cfg ProfileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", source, err)
	}
	// Validate every DSN value parses as a mysql:// URL. This catches typos at
	// load time rather than at first use. We reuse the same validation the CLI
	// already runs; if the file was wrong, we want the error to mention the
	// file path, which the outer caller has.
	for name, dsn := range cfg.DSNs {
		if err := ValidateMySQLURL(dsn); err != nil {
			return nil, fmt.Errorf("config file %q, profile %q: %w", source, name, err)
		}
	}
	return cfg.DSNs, nil
}

// MergeProfiles returns a new ProfileMap where entries in `override` replace
// entries with the same name in `base`. Either argument may be nil. Order of
// keys is not preserved; this is a shallow map merge by name.
func MergeProfiles(base, override ProfileMap) ProfileMap {
	out := make(ProfileMap)
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

// LoadMergedConfig loads the global + local configs and merges them, with the
// local config taking precedence. Returns the merged map. Returns an empty map
// (not an error) when no config files exist; the caller decides whether that
// is acceptable. Errors from malformed YAML or invalid DSNs are returned
// with the offending file path attached.
func LoadMergedConfig() (ProfileMap, error) {
	merged := ProfileMap{}
	if path := FindGlobalConfigPath(); path != "" {
		pm, err := LoadProfileMap(path)
		if err != nil {
			return nil, err
		}
		merged = MergeProfiles(merged, pm)
	}
	if path := FindLocalConfigPath("."); path != "" {
		pm, err := LoadProfileMap(path)
		if err != nil {
			return nil, err
		}
		merged = MergeProfiles(merged, pm)
	}
	return merged, nil
}

// ResolveProfile looks up a named profile in the merged map. Returns
// ErrProfileNotFound wrapped with the available profile list when missing,
// so the caller can surface a helpful error without re-listing.
func ResolveProfile(merged ProfileMap, name string) (string, error) {
	if name == "" {
		return "", errors.New("profile name is empty")
	}
	dsn, ok := merged[name]
	if !ok {
		return "", fmt.Errorf("%w: %q (available: %s)",
			ErrProfileNotFound, name, sortedKeys(merged))
	}
	return dsn, nil
}

func sortedKeys(m ProfileMap) string {
	if len(m) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// stable sort for deterministic error messages
	sortStrings(keys)
	return strings.Join(keys, ", ")
}

// minimal local string sort to avoid pulling in sort package for one call
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// WriteProfileMap atomically writes the profile map to path as YAML.
//
// It creates parent directories as needed, sorts keys alphabetically for
// deterministic output, and uses an atomic write pattern (temp file + rename)
// to avoid partial writes. Permissions: 0600 for new files, preserve existing
// file's permissions for overwrites. Nil maps are normalized to empty.
func WriteProfileMap(path string, pm ProfileMap) error {
	// Normalize nil to empty map
	if pm == nil {
		pm = ProfileMap{}
	}

	// Ensure parent directory exists
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create parent directory for %q: %w", path, err)
	}

	// Determine target permissions: preserve existing file's mode, else 0600
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	// Atomic write: create temp file in same directory, rename on success
	tmp, err := os.CreateTemp(parent, ".sql-cli.yaml.")
	if err != nil {
		return fmt.Errorf("create temp file in %q: %w", parent, err)
	}
	tmpPath := tmp.Name()

	cleanup := func() {
		os.Remove(tmpPath) // best-effort
	}

	if err := func() error {
		defer tmp.Close()
		// Build ordered YAML: dsns -> mapping of key: value
		content := buildYAMLContent(pm)
		if _, err := tmp.WriteString(content); err != nil {
			return fmt.Errorf("write temp file: %w", err)
		}
		// Set permissions before rename so the final file has the right mode
		if err := os.Chmod(tmpPath, mode); err != nil {
			return fmt.Errorf("chmod temp file: %w", err)
		}
		return nil
	}(); err != nil {
		cleanup()
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename temp file to %q: %w", path, err)
	}

	return nil
}

// buildYAMLContent builds a YAML string with keys sorted alphabetically,
// using yaml.Node to guarantee ordered mapping output.
func buildYAMLContent(pm ProfileMap) string {
	// Collect and sort keys
	keys := make([]string, 0, len(pm))
	for k := range pm {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build yaml.Node mapping: dsns -> [key: value, ...]
	dsnsContent := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range keys {
		dsnsContent.Content = append(dsnsContent.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: k},
			&yaml.Node{Kind: yaml.ScalarNode, Value: pm[k]},
		)
	}

	doc := &yaml.Node{Kind: yaml.MappingNode}
	doc.Content = append(doc.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "dsns"},
		dsnsContent,
	)

	data, _ := yaml.Marshal(doc)
	return string(data)
}
