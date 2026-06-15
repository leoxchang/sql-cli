package command

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/qiezi999/sql-cli/internal/config"
	"github.com/qiezi999/sql-cli/internal/output"
)

// profileNameRegex validates profile names per spec:
// First char must be [A-Za-z0-9_], rest allows [A-Za-z0-9_.-]
var profileNameRegex = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// HandleConfig dispatches to config subcommands (add, list).
// Returns exit code 0 on success, 2 for CONFIG_ERROR, 99 for INTERNAL_ERROR.
func HandleConfig(args []string) int {
	if len(args) == 0 {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			"config subcommand required: use 'config add' or 'config list'",
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	subcommand := args[0]

	switch subcommand {
	case "add":
		return handleConfigAdd(args[1:])
	case "list":
		return handleConfigList(args[1:])
	default:
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			fmt.Sprintf("unknown config subcommand %q: use 'config add' or 'config list'", subcommand),
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}
}

// handleConfigAdd implements `config add <name> <dsn>`.
// Returns exit code 0 on success, 2 for CONFIG_ERROR, 99 for INTERNAL_ERROR.
func handleConfigAdd(args []string) int {
	startTime := time.Now()

	// Parse flags
	isGlobal := false
	positional := make([]string, 0, len(args))
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--global" {
			isGlobal = true
		} else if strings.HasPrefix(arg, "-") && len(arg) > 1 && arg[1] == '-' {
			// Long flag that is not --global → unknown
			errEnvelope := output.NewErrorEnvelope(
				output.ErrorCodeConfigError,
				fmt.Sprintf("unknown flag %q", arg),
				nil,
			)
			_ = output.WriteError(errEnvelope)
			return output.ErrorCodeConfigError.ExitCode()
		} else if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			// Short flag or flag-like arg (e.g. "-dev") — treat as positional
			// (profile names may start with a dash)
			positional = append(positional, arg)
		} else {
			positional = append(positional, arg)
		}
		i++
	}

	if len(positional) < 1 {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			"profile name required: config add <name> <dsn>",
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	if len(positional) < 2 {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			"DSN required: config add <name> <dsn>",
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	name := positional[0]
	dsn := positional[1]

	// Validate profile name: first char [A-Za-z0-9_], rest allows [A-Za-z0-9_.-]
	if !profileNameRegex.MatchString(name) {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			fmt.Sprintf("invalid profile name %q: must match ^[A-Za-z0-9_][A-Za-z0-9_.-]*$", name),
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	// Validate DSN
	if err := config.ValidateMySQLURL(dsn); err != nil {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			fmt.Sprintf("invalid DSN: %v", err),
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	// Determine target file path and load existing profiles
	var targetPath string
	var pm config.ProfileMap

	if isGlobal {
		targetPath = config.FindGlobalConfigPath()
		if targetPath == "" {
			home, _ := os.UserHomeDir()
			if home == "" {
				home = os.Getenv("HOME")
			}
			if home == "" {
				fmt.Fprintln(os.Stderr, "Error: cannot determine home directory for global config")
				return output.ErrorCodeConfigError.ExitCode()
			}
			targetPath = filepath.Join(home, ".sql-cli", "config.yaml")
		}
		// Load existing global profiles if file exists
		pm, _ = config.LoadProfileMap(targetPath)
		if pm == nil {
			pm = config.ProfileMap{}
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			errEnvelope := output.NewErrorEnvelope(
				output.ErrorCodeInternalError,
				fmt.Sprintf("failed to get current directory: %v", err),
				nil,
			)
			_ = output.WriteError(errEnvelope)
			return output.ErrorCodeInternalError.ExitCode()
		}
		targetPath = filepath.Join(cwd, ".sql-cli.yaml")
		// Load existing local profiles if file exists
		pm, _ = config.LoadProfileMap(targetPath)
		if pm == nil {
			pm = config.ProfileMap{}
		}
	}

	// Check for overwrite (different value)
	var action string
	if oldDSN, exists := pm[name]; exists && oldDSN != dsn {
		action = "overwritten"
		fmt.Fprintf(os.Stderr, "warning: profile %q overwritten: %s -> %s\n", name, maskDSN(oldDSN), maskDSN(dsn))
	} else if exists && oldDSN == dsn {
		action = "unchanged"
	} else {
		action = "created"
	}

	// Write updated profile map
	pm[name] = dsn
	if err := config.WriteProfileMap(targetPath, pm); err != nil {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeInternalError,
			fmt.Sprintf("failed to write config: %v", err),
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeInternalError.ExitCode()
	}

	elapsedMs := time.Since(startTime).Milliseconds()

	// Output spec-compliant envelope directly.
	// Only include "action" when the profile actually changed (created or overwritten).
	envelope := map[string]any{
		"ok":         true,
		"profile":    name,
		"dsn":       maskDSN(dsn),
		"path":      targetPath,
		"elapsed_ms": elapsedMs,
	}
	if action == "created" || action == "overwritten" {
		envelope["action"] = action
	}
	_ = json.NewEncoder(os.Stdout).Encode(envelope)

	return 0
}

// handleConfigList implements `config list`.
// Returns exit code 0 on success, 2 for CONFIG_ERROR.
func handleConfigList(args []string) int {
	startTime := time.Now()

	// Parse flags
	isLocal := false
	isGlobal := false
	for _, arg := range args {
		if arg == "--local" {
			isLocal = true
		} else if arg == "--global" {
			isGlobal = true
		} else {
			// Positional args are not allowed
			errEnvelope := output.NewErrorEnvelope(
				output.ErrorCodeConfigError,
				fmt.Sprintf("unknown argument %q: config list accepts only --local and --global flags", arg),
				nil,
			)
			_ = output.WriteError(errEnvelope)
			return output.ErrorCodeConfigError.ExitCode()
		}
	}

	if isLocal && isGlobal {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			"--local and --global flags are mutually exclusive",
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	switch {
	case isLocal:
		cwd, err := os.Getwd()
		if err != nil {
			errEnvelope := output.NewErrorEnvelope(
				output.ErrorCodeConfigError,
				fmt.Sprintf("failed to get current directory: %v", err),
				nil,
			)
			_ = output.WriteError(errEnvelope)
			return output.ErrorCodeConfigError.ExitCode()
		}
		localPath := filepath.Join(cwd, ".sql-cli.yaml")
		var pm config.ProfileMap
		if loaded, err := config.LoadProfileMap(localPath); err == nil {
			pm = loaded
		}
		sortedNames := make([]string, 0, len(pm))
		for name := range pm {
			sortedNames = append(sortedNames, name)
		}
		sort.Strings(sortedNames)

		elapsedMs := time.Since(startTime).Milliseconds()
		profiles := make([]map[string]string, 0, len(sortedNames))
		for _, name := range sortedNames {
			profiles = append(profiles, map[string]string{
				"name":   name,
				"dsn":    maskDSN(pm[name]),
				"source": localPath,
			})
		}
		envelope := map[string]any{
			"ok":        true,
			"profiles":  profiles,
			"count":     len(profiles),
			"elapsed_ms": elapsedMs,
		}
		_ = json.NewEncoder(os.Stdout).Encode(envelope)
		return 0
	case isGlobal:
		globalPath := config.FindGlobalConfigPath()
		var pm config.ProfileMap
		if globalPath != "" {
			if loaded, err := config.LoadProfileMap(globalPath); err == nil {
				pm = loaded
			}
		}
		sortedNames := make([]string, 0, len(pm))
		for name := range pm {
			sortedNames = append(sortedNames, name)
		}
		sort.Strings(sortedNames)

		elapsedMs := time.Since(startTime).Milliseconds()
		profiles := make([]map[string]string, 0, len(sortedNames))
		for _, name := range sortedNames {
			profiles = append(profiles, map[string]string{
				"name":   name,
				"dsn":    maskDSN(pm[name]),
				"source": globalPath,
			})
		}
		envelope := map[string]any{
			"ok":        true,
			"profiles":  profiles,
			"count":     len(profiles),
			"elapsed_ms": elapsedMs,
		}
		_ = json.NewEncoder(os.Stdout).Encode(envelope)
		return 0
	default:
		// Merged view: load global and local separately to track sources,
		// then merge with local taking precedence (same semantics as LoadMergedConfig).
		globalPath := config.FindGlobalConfigPath()
		globalPM := config.ProfileMap{}
		if globalPath != "" {
			if tmp, err := config.LoadProfileMap(globalPath); err == nil {
				globalPM = tmp
			}
		}

		cwd, _ := os.Getwd()
		localPath := filepath.Join(cwd, ".sql-cli.yaml")
		localPM := config.ProfileMap{}
		if tmp, err := config.LoadProfileMap(localPath); err == nil {
			localPM = tmp
		}

		merged := config.MergeProfiles(globalPM, localPM)

		// Build sources map: global first, then local overrides
		sources := make(map[string]string)
		for name := range globalPM {
			sources[name] = globalPath
		}
		for name := range localPM {
			sources[name] = localPath
		}

		// Output with source tracking
		sortedNames := make([]string, 0, len(merged))
		for name := range merged {
			sortedNames = append(sortedNames, name)
		}
		sort.Strings(sortedNames)

		elapsedMs := time.Since(startTime).Milliseconds()
		profiles := make([]map[string]string, 0, len(sortedNames))
		for _, name := range sortedNames {
			profiles = append(profiles, map[string]string{
				"name":   name,
				"dsn":    maskDSN(merged[name]),
				"source": sources[name],
			})
		}

		envelope := map[string]any{
			"ok":        true,
			"profiles":  profiles,
			"count":     len(profiles),
			"elapsed_ms": elapsedMs,
		}
		_ = json.NewEncoder(os.Stdout).Encode(envelope)
		return 0
	}
}

// maskDSN masks the password in a mysql:// URL.
// mysql://user:password@host:port/db -> mysql://user:****@host:port/db
// If no password is present or URL is invalid, returns the input unchanged.
func maskDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	if parsed.Scheme != "mysql" {
		return dsn
	}
	user := parsed.User
	if user == nil {
		return dsn
	}
	pass, hasPass := user.Password()
	// Only mask if a non-empty password is present
	if !hasPass || pass == "" {
		return dsn
	}
	// Rebuild the URL manually to avoid URL-encoding of the mask string.
	// Use parsed.Path (already decoded) and parsed.RawQuery (not decoded) so that
	// special characters in the path/database name are preserved literally.
	result := fmt.Sprintf("mysql://%s:****@%s%s", user.Username(), parsed.Host, parsed.Path)
	if parsed.RawQuery != "" {
		result += "?" + parsed.RawQuery
	}
	return result
}