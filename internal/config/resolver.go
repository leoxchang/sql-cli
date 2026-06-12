package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/qiezi999/sql-cli/internal/output"
)

// ErrDSNNotConfigured is returned when no DSN is provided via flag, env, or profile.
var ErrDSNNotConfigured = errors.New("DSN not configured: --dsn flag, SQL_CLI_DSN env, or --profile <name> (with config file) required")

// ResolveDSN determines the DSN to use based on the precedence chain:
//  1. --dsn flag (if non-empty)
//  2. SQL_CLI_DSN environment variable (if set)
//  3. profile from merged config file (if profileName non-empty and lookup succeeds)
//  4. Error
//
// The profile lookup only runs when flag and env are absent. If profileName
// is non-empty but the merged config is empty or the profile is missing, the
// returned error includes the available profile list to help the user fix
// their invocation. profileName "" means "no profile requested" — a missing
// profile is treated identically to "no DSN at all".
func ResolveDSN(flagDSN, profileName string) (string, error) {
	// Priority 1: Flag takes precedence
	if flagDSN != "" {
		return flagDSN, nil
	}

	// Priority 2: Environment variable
	envDSN := os.Getenv("SQL_CLI_DSN")
	if envDSN != "" {
		return envDSN, nil
	}

	// Priority 3: Config file profile (only when profileName is provided)
	if profileName != "" {
		merged, err := LoadMergedConfig()
		if err != nil {
			// YAML parse / invalid DSN in the config — surface that as CONFIG_ERROR
			// with the file path attached.
			return "", fmt.Errorf("load config: %w", err)
		}
		dsn, err := ResolveProfile(merged, profileName)
		if err != nil {
			if errors.Is(err, ErrProfileNotFound) {
				// Wrap so the caller can present "profile X not found" as the
				// primary user-facing message while still chaining the original
				// error.
				return "", fmt.Errorf("profile %q: %w", profileName, err)
			}
			return "", err
		}
		return dsn, nil
	}

	// Priority 4: Error - nothing configured
	return "", ErrDSNNotConfigured
}

// ResolveDSNOrError resolves the DSN or returns a CONFIG_ERROR envelope.
// This is a convenience function for CLI handlers that need to emit JSON errors.
func ResolveDSNOrError(flagDSN, profileName string) (string, *output.Envelope) {
	dsn, err := ResolveDSN(flagDSN, profileName)
	if err != nil {
		return "", output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			err.Error(),
			nil,
		)
	}
	return dsn, nil
}