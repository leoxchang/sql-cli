package config

import (
	"errors"
	"os"

	"github.com/qiezi999/sql-cli/internal/output"
)

// ErrDSNNotConfigured is returned when no DSN is provided via flag or environment.
var ErrDSNNotConfigured = errors.New("DSN not configured: --dsn flag or SQL_CLI_DSN environment variable required")

// ResolveDSN determines the DSN to use based on the precedence chain:
// 1. --dsn flag (if non-empty)
// 2. SQL_CLI_DSN environment variable (if set)
// 3. Error if both absent
//
// This follows D5 specification: flag is explicit user input, env var is shell export.
// No fallback to ~/.my.cnf or auto-discovery to avoid surprises in agent contexts.
func ResolveDSN(flagDSN string) (string, error) {
	// Priority 1: Flag takes precedence
	if flagDSN != "" {
		return flagDSN, nil
	}

	// Priority 2: Environment variable
	envDSN := os.Getenv("SQL_CLI_DSN")
	if envDSN != "" {
		return envDSN, nil
	}

	// Priority 3: Error - nothing configured
	return "", ErrDSNNotConfigured
}

// ResolveDSNOrError resolves the DSN or returns a CONFIG_ERROR envelope.
// This is a convenience function for CLI handlers that need to emit JSON errors.
func ResolveDSNOrError(flagDSN string) (string, *output.Envelope) {
	dsn, err := ResolveDSN(flagDSN)
	if err != nil {
		return "", output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			err.Error(),
			nil,
		)
	}
	return dsn, nil
}