package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/qiezi999/sql-cli/internal/output"
)

func TestRun_NoArgs(t *testing.T) {
	// When no args provided, should show help and return 0
	exitCode := Run([]string{"sql-cli"})
	if exitCode != 0 {
		t.Errorf("expected exit code 0 for no args, got %d", exitCode)
	}
}

func TestRun_HelpFlag(t *testing.T) {
	// Capture stdout to verify help is written there
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// --help should show help and return 0
	exitCode := Run([]string{"sql-cli", "--help"})

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for --help, got %d", exitCode)
	}
	if output == "" {
		t.Error("help text should be written to stdout")
	}

	// -h should also work
	r, w, _ = os.Pipe()
	os.Stdout = w
	exitCode = Run([]string{"sql-cli", "-h"})
	w.Close()
	os.Stdout = oldStdout

	buf.Reset()
	buf.ReadFrom(r)
	output = buf.String()

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for -h, got %d", exitCode)
	}
	if output == "" {
		t.Error("help text should be written to stdout for -h")
	}
}

func TestRun_VersionFlag(t *testing.T) {
	// Capture stdout to verify version is written there
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// --version should show version and return 0 (without DSN)
	exitCode := Run([]string{"sql-cli", "--version"})

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for --version, got %d", exitCode)
	}
	if output == "" {
		t.Error("version text should be written to stdout")
	}

	// version subcommand should also work
	r, w, _ = os.Pipe()
	os.Stdout = w
	exitCode = Run([]string{"sql-cli", "version"})
	w.Close()
	os.Stdout = oldStdout

	buf.Reset()
	buf.ReadFrom(r)
	output = buf.String()

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for version subcommand, got %d", exitCode)
	}
	if output == "" {
		t.Error("version text should be written to stdout for version subcommand")
	}
}

func TestRun_MissingDSN(t *testing.T) {
	// Clear environment variable
	os.Unsetenv("SQL_CLI_DSN")

	// Missing DSN should return CONFIG_ERROR exit code 2
	exitCode := Run([]string{"sql-cli", "databases"})
	if exitCode != 2 {
		t.Errorf("expected exit code 2 for missing DSN, got %d", exitCode)
	}
}

func TestRun_DSNFromFlag(t *testing.T) {
	// Clear environment variable
	os.Unsetenv("SQL_CLI_DSN")

	// DSN from flag should be used
	// Note: subcommand handlers are not yet implemented, so we expect internal error
	// But DSN resolution should succeed
	dsn := "mysql://root:test@localhost:3306/"
	exitCode := Run([]string{"sql-cli", "--dsn", dsn, "databases"})

	// Since handler is not implemented, we expect internal error (99)
	// If DSN resolution failed, we would get 2
	if exitCode == 2 {
		t.Error("DSN resolution should succeed with valid DSN from flag")
	}
}

func TestRun_DSNFromEnv(t *testing.T) {
	// Set environment variable
	dsn := "mysql://root:test@localhost:3306/"
	os.Setenv("SQL_CLI_DSN", dsn)
	defer os.Unsetenv("SQL_CLI_DSN")

	// DSN from environment should be used
	exitCode := Run([]string{"sql-cli", "databases"})

	// Since handler is not implemented, we expect internal error (99)
	// If DSN resolution failed, we would get 2
	if exitCode == 2 {
		t.Error("DSN resolution should succeed with valid DSN from environment")
	}
}

func TestRun_FlagOverridesEnv(t *testing.T) {
	// Set environment variable
	os.Setenv("SQL_CLI_DSN", "mysql://env:pass@localhost:3306/")
	defer os.Unsetenv("SQL_CLI_DSN")

	// Flag DSN should override environment DSN
	flagDSN := "mysql://flag:pass@localhost:3306/"
	exitCode := Run([]string{"sql-cli", "--dsn", flagDSN, "databases"})

	// Since handler is not implemented, we expect internal error (99)
	// If DSN resolution failed, we would get 2
	if exitCode == 2 {
		t.Error("DSN resolution should succeed with flag DSN overriding environment")
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	// Set valid DSN
	os.Setenv("SQL_CLI_DSN", "mysql://root:test@localhost:3306/")
	defer os.Unsetenv("SQL_CLI_DSN")

	// Unknown subcommand should return QUERY_ERROR exit code 1
	exitCode := Run([]string{"sql-cli", "unknown"})
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for unknown subcommand, got %d", exitCode)
	}
}

func TestRun_SubcommandsNotImplemented(t *testing.T) {
	// Set valid DSN
	os.Setenv("SQL_CLI_DSN", "mysql://root:test@localhost:3306/")
	defer os.Unsetenv("SQL_CLI_DSN")

	// Since handlers are now implemented, they will attempt to connect
	// We expect connection/auth error, not internal error (99)
	subcommands := []string{"databases", "tables", "describe", "query"}

	for _, subcmd := range subcommands {
		args := []string{"sql-cli", subcmd}
		if subcmd == "tables" {
			args = append(args, "mydb")
		} else if subcmd == "describe" {
			args = append(args, "mydb.users")
		} else if subcmd == "query" {
			args = append(args, "SELECT 1")
		}

		exitCode := Run(args)
		// Handlers are implemented, will fail with connection/auth error (not 99)
		// Expected codes: 3 (CONNECTION_ERROR), 4 (AUTH_ERROR), or similar
		if exitCode == 99 {
			t.Errorf("handlers should be implemented for %s, expected connection/auth error, got internal error 99", subcmd)
		}
	}
}

func TestDispatch_ValidSubcommands(t *testing.T) {
	dsn := "mysql://root:test@localhost:3306/"

	// Test that dispatch recognizes all valid subcommands
	subcommands := []struct {
		name string
		args []string
	}{
		{"databases", []string{}},
		{"tables", []string{"mydb"}},
		{"describe", []string{"mydb.users"}},
		{"desc", []string{"mydb.users"}},
		{"query", []string{"SELECT 1"}},
	}

	for _, tc := range subcommands {
		exitCode := dispatch(tc.name, dsn, tc.args)
		// Handlers are implemented, will fail with connection/auth error (not 99)
		// Expected codes: 3 (CONNECTION_ERROR), 4 (AUTH_ERROR), or similar
		if exitCode == 99 {
			t.Errorf("handlers should be implemented for %s, expected connection/auth error, got internal error 99", tc.name)
		}
	}
}

func TestDispatch_HelpVariants(t *testing.T) {
	dsn := "mysql://root:test@localhost:3306/"

	// Test that help variants return 0 (these are handled in dispatch for legacy compatibility)
	helpVariants := []string{"--help", "-h"}

	for _, variant := range helpVariants {
		exitCode := dispatch(variant, dsn, []string{})
		if exitCode != 0 {
			t.Errorf("expected exit code 0 for help variant %s, got %d", variant, exitCode)
		}
	}
}

func TestDispatch_VersionVariants(t *testing.T) {
	// Version is handled in Run() before dispatch, so dispatch will treat it as unknown
	dsn := "mysql://root:test@localhost:3306/"

	// "version" is not handled in dispatch, it's handled in Run()
	exitCode := dispatch("version", dsn, []string{})
	// Should return 1 (QUERY_ERROR) since dispatch doesn't know about version
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for version in dispatch (handled in Run), got %d", exitCode)
	}
}

func TestDispatch_UnknownSubcommand(t *testing.T) {
	dsn := "mysql://root:test@localhost:3306/"

	// Unknown subcommand should return 1 (QUERY_ERROR)
	exitCode := dispatch("unknown", dsn, []string{})
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for unknown subcommand, got %d", exitCode)
	}
}

func TestOutputFormat_OnError(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Clear environment variable to trigger CONFIG_ERROR
	os.Unsetenv("SQL_CLI_DSN")

	// Run with missing DSN
	exitCode := Run([]string{"sql-cli", "databases"})

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	outputStr := buf.String()

	// Parse JSON
	var envelope output.Envelope
	err := json.Unmarshal(buf.Bytes(), &envelope)
	if err != nil {
		t.Errorf("output should be valid JSON: %v", err)
	}

	// Verify envelope structure
	if envelope.Ok != false {
		t.Error("error envelope should have ok=false")
	}

	if envelope.Error == nil {
		t.Error("error envelope should have error field")
	} else {
		if envelope.Error.Code != output.ErrorCodeConfigError {
			t.Errorf("expected error code CONFIG_ERROR, got %s", envelope.Error.Code)
		}
	}

	// Verify exit code
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	// Output should not be empty
	if outputStr == "" {
		t.Error("output should not be empty on error")
	}

	// Verify newline is appended after JSON
	if outputStr[len(outputStr)-1] != '\n' {
		t.Error("output should end with newline for proper terminal display")
	}
}

func TestRun_DSNAndProfileMutuallyExclusive(t *testing.T) {
	// Set DSN via env to keep this test free of DSN-resolution side effects.
	os.Setenv("SQL_CLI_DSN", "mysql://env:pass@localhost:3306/")
	defer os.Unsetenv("SQL_CLI_DSN")

	// Capture stdout to verify the error envelope is emitted.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := Run([]string{"sql-cli", "--dsn", "mysql://flag:pass@localhost:3306/", "--profile", "dev", "databases"})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 (CONFIG_ERROR), got %d", exitCode)
	}
	output := buf.String()
	if !strings.Contains(output, "mutually exclusive") {
		t.Errorf("expected error to mention mutual exclusion, got: %s", output)
	}
}