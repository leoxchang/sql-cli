package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDSN_FlagTakesPrecedence(t *testing.T) {
	// Setup: Set environment variable
	originalEnv := os.Getenv("SQL_CLI_DSN")
	defer os.Setenv("SQL_CLI_DSN", originalEnv)

	os.Setenv("SQL_CLI_DSN", "env_dsn_value")

	// Test: Flag should take precedence
	flagDSN := "flag_dsn_value"
	result, err := ResolveDSN(flagDSN, "")

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != flagDSN {
		t.Errorf("Expected flag DSN %q, got %q", flagDSN, result)
	}
}

func TestResolveDSN_EnvUsedWhenFlagAbsent(t *testing.T) {
	// Setup: Set environment variable
	originalEnv := os.Getenv("SQL_CLI_DSN")
	defer os.Setenv("SQL_CLI_DSN", originalEnv)

	envDSN := "env_dsn_value"
	os.Setenv("SQL_CLI_DSN", envDSN)

	// Test: Empty flag should fall back to env
	result, err := ResolveDSN("", "")

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != envDSN {
		t.Errorf("Expected env DSN %q, got %q", envDSN, result)
	}
}

func TestResolveDSN_ErrorWhenBothAbsent(t *testing.T) {
	// Setup: Unset environment variable
	originalEnv := os.Getenv("SQL_CLI_DSN")
	defer os.Setenv("SQL_CLI_DSN", originalEnv)

	os.Unsetenv("SQL_CLI_DSN")

	// Test: Should error when both absent
	result, err := ResolveDSN("", "")

	if err == nil {
		t.Errorf("Expected error when DSN not configured, got result: %q", result)
	}
	if !errors.Is(err, ErrDSNNotConfigured) {
		t.Errorf("Expected ErrDSNNotConfigured, got: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty result on error, got: %q", result)
	}
}

func TestResolveDSN_FlagNotEmptyEnvAbsent(t *testing.T) {
	// Setup: Unset environment variable
	originalEnv := os.Getenv("SQL_CLI_DSN")
	defer os.Setenv("SQL_CLI_DSN", originalEnv)

	os.Unsetenv("SQL_CLI_DSN")

	// Test: Flag should work when env is absent
	flagDSN := "flag_only_dsn"
	result, err := ResolveDSN(flagDSN, "")

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != flagDSN {
		t.Errorf("Expected flag DSN %q, got %q", flagDSN, result)
	}
}

func TestResolveDSN_FlagEmptyEnvAbsent(t *testing.T) {
	// Setup: Unset environment variable
	originalEnv := os.Getenv("SQL_CLI_DSN")
	defer os.Setenv("SQL_CLI_DSN", originalEnv)

	os.Unsetenv("SQL_CLI_DSN")

	// Test: Should error with empty flag and absent env
	result, err := ResolveDSN("", "")

	if err == nil {
		t.Errorf("Expected error, got result: %q", result)
	}
}

func TestResolveDSN_FlagOverridesEmptyEnv(t *testing.T) {
	// Setup: Set empty environment variable
	originalEnv := os.Getenv("SQL_CLI_DSN")
	defer os.Setenv("SQL_CLI_DSN", originalEnv)

	os.Setenv("SQL_CLI_DSN", "")

	// Test: Flag should work even if env is empty
	flagDSN := "flag_override_dsn"
	result, err := ResolveDSN(flagDSN, "")

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != flagDSN {
		t.Errorf("Expected flag DSN %q, got %q", flagDSN, result)
	}
}

func TestResolveDSN_EmptyEnvCausesError(t *testing.T) {
	// Setup: Set empty environment variable
	originalEnv := os.Getenv("SQL_CLI_DSN")
	defer os.Setenv("SQL_CLI_DSN", originalEnv)

	os.Setenv("SQL_CLI_DSN", "")

	// Test: Empty env with empty flag should error
	result, err := ResolveDSN("", "")

	if err == nil {
		t.Errorf("Expected error when both flag and env are empty, got result: %q", result)
	}
	if !errors.Is(err, ErrDSNNotConfigured) {
		t.Errorf("Expected ErrDSNNotConfigured, got: %v", err)
	}
}

func TestResolveDSNOrError_Success(t *testing.T) {
	// Setup: Set environment variable
	originalEnv := os.Getenv("SQL_CLI_DSN")
	defer os.Setenv("SQL_CLI_DSN", originalEnv)

	envDSN := "env_dsn_value"
	os.Setenv("SQL_CLI_DSN", envDSN)

	// Test: Should return DSN without error envelope
	result, envelope := ResolveDSNOrError("", "")

	if envelope != nil {
		t.Errorf("Expected nil envelope on success, got: %v", envelope)
	}
	if result != envDSN {
		t.Errorf("Expected DSN %q, got %q", envDSN, result)
	}
}

func TestResolveDSNOrError_Error(t *testing.T) {
	// Setup: Unset environment variable
	originalEnv := os.Getenv("SQL_CLI_DSN")
	defer os.Setenv("SQL_CLI_DSN", originalEnv)

	os.Unsetenv("SQL_CLI_DSN")

	// Test: Should return error envelope
	result, envelope := ResolveDSNOrError("", "")

	if result != "" {
		t.Errorf("Expected empty result on error, got: %q", result)
	}
	if envelope == nil {
		t.Error("Expected error envelope, got nil")
	}
	if envelope != nil {
		if envelope.Ok {
			t.Error("Expected Ok=false in error envelope")
		}
		if envelope.Error == nil {
			t.Error("Expected Error detail in error envelope")
		} else {
			if envelope.Error.Code != "CONFIG_ERROR" {
				t.Errorf("Expected error code CONFIG_ERROR, got: %s", envelope.Error.Code)
			}
			if envelope.Error.Message == "" {
				t.Error("Expected non-empty error message")
			}
		}
	}
}

func TestResolveDSN_ProfileResolvesFromConfig(t *testing.T) {
	// Build a temp config dir containing a single profile; clear env to
	// isolate the profile resolution path.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("dsns:\n  dev: mysql://u:p@h:3306/devdb\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	originalConfig := os.Getenv("SQL_CLI_CONFIG_DIR")
	originalEnv := os.Getenv("SQL_CLI_DSN")
	defer os.Setenv("SQL_CLI_CONFIG_DIR", originalConfig)
	defer os.Setenv("SQL_CLI_DSN", originalEnv)
	os.Setenv("SQL_CLI_CONFIG_DIR", dir)
	os.Unsetenv("SQL_CLI_DSN")

	got, err := ResolveDSN("", "dev")
	if err != nil {
		t.Fatalf("ResolveDSN: %v", err)
	}
	if got != "mysql://u:p@h:3306/devdb" {
		t.Errorf("expected profile DSN, got %q", got)
	}
}

func TestResolveDSN_ProfileUnknown_Errors(t *testing.T) {
	// Empty config dir → LoadMergedConfig returns empty map → profile lookup fails.
	originalConfig := os.Getenv("SQL_CLI_CONFIG_DIR")
	originalEnv := os.Getenv("SQL_CLI_DSN")
	defer os.Setenv("SQL_CLI_CONFIG_DIR", originalConfig)
	defer os.Setenv("SQL_CLI_DSN", originalEnv)
	os.Setenv("SQL_CLI_CONFIG_DIR", t.TempDir())
	os.Unsetenv("SQL_CLI_DSN")

	_, err := ResolveDSN("", "ghost")
	if !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("expected ErrProfileNotFound, got: %v", err)
	}
}