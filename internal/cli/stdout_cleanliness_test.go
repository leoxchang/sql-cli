package cli

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiezi999/sql-cli/internal/output"
)

// TestStdoutCleanliness_HelpVersion verifies help and version output goes to stdout only
func TestStdoutCleanliness_HelpVersion(t *testing.T) {
	binary := buildBinary(t)

	tests := []struct {
		name string
		args []string
	}{
		{"help flag", []string{"--help"}},
		{"h flag", []string{"-h"}},
		{"help subcommand", []string{"help"}},
		{"version flag", []string{"--version"}},
		{"version subcommand", []string{"version"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			cmd.Stdout = stdout
			cmd.Stderr = stderr

			err := cmd.Run()
			if err != nil {
				t.Errorf("command failed: %v", err)
			}

			stdoutStr := stdout.String()
			stderrStr := stderr.String()

			// Help/version should go to stdout
			if stdoutStr == "" {
				t.Errorf("expected output on stdout, got empty")
			}

			// Stderr should be empty for help/version
			if stderrStr != "" {
				t.Errorf("stderr should be empty for %s, got: %q", tt.name, stderrStr)
			}

			// Output should be plain text, not JSON
			if strings.HasPrefix(stdoutStr, "{") {
				t.Errorf("help/version should be plain text, not JSON, got: %q", stdoutStr[:min(50, len(stdoutStr))])
			}
		})
	}
}

// TestStdoutCleanliness_MissingDSN verifies error envelope goes to stdout on CONFIG_ERROR
func TestStdoutCleanliness_MissingDSN(t *testing.T) {
	binary := buildBinary(t)

	// Set up environment without DSN
	cmd := exec.Command(binary, "databases")
	cmd.Env = []string{} // Clear all env vars
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	exitCode := runCommand(cmd)
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	// Should exit with CONFIG_ERROR code (2)
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	// Stdout should contain JSON error envelope
	var envelope output.Envelope
	err := json.Unmarshal([]byte(stdoutStr), &envelope)
	if err != nil {
		t.Errorf("stdout should contain valid JSON envelope, got parse error: %v\nstdout: %q", err, stdoutStr)
	}

	// Verify error envelope structure
	if envelope.Ok != false {
		t.Error("error envelope should have ok=false")
	}
	if envelope.Error == nil {
		t.Error("error envelope should have error field")
	} else if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected error code CONFIG_ERROR, got %s", envelope.Error.Code)
	}

	// Stderr may contain diagnostics but should not contaminate stdout
	// We already verified stdout is clean JSON above
	t.Logf("stderr output (diagnostics): %q", stderrStr)
}

// TestStdoutCleanliness_UnknownSubcommand verifies error envelope on unknown subcommand
func TestStdoutCleanliness_UnknownSubcommand(t *testing.T) {
	binary := buildBinary(t)

	// Provide DSN to get past DSN resolution
	cmd := exec.Command(binary, "--dsn", "mysql://root:test@localhost:3306/", "unknown")
	cmd.Env = []string{} // Clear env to avoid interference
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	exitCode := runCommand(cmd)
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	// Should exit with QUERY_ERROR code (1)
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	// Stdout should contain JSON error envelope
	var envelope output.Envelope
	err := json.Unmarshal([]byte(stdoutStr), &envelope)
	if err != nil {
		t.Errorf("stdout should contain valid JSON envelope, got parse error: %v\nstdout: %q", err, stdoutStr)
	}

	// Verify error envelope structure
	if envelope.Ok != false {
		t.Error("error envelope should have ok=false")
	}
	if envelope.Error == nil {
		t.Error("error envelope should have error field")
	} else if envelope.Error.Code != output.ErrorCodeQueryError {
		t.Errorf("expected error code QUERY_ERROR, got %s", envelope.Error.Code)
	}

	t.Logf("stderr output (diagnostics): %q", stderrStr)
}

// TestStdoutCleanliness_DatabasesSubcommand verifies databases subcommand stdout cleanliness
func TestStdoutCleanliness_DatabasesSubcommand(t *testing.T) {
	binary := buildBinary(t)

	// Test with invalid DSN (will fail to connect, but that's OK)
	// We just need to verify stdout is clean JSON
	cmd := exec.Command(binary, "--dsn", "mysql://invalid:invalid@localhost:3306/", "databases")
	cmd.Env = []string{}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	exitCode := runCommand(cmd)
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	// Stdout should contain valid JSON
	var envelope output.Envelope
	err := json.Unmarshal([]byte(stdoutStr), &envelope)
	if err != nil {
		t.Errorf("stdout should contain valid JSON envelope, got parse error: %v\nstdout: %q", err, stdoutStr)
	}

	// Verify envelope structure (either success or failure)
	if exitCode == 0 {
		// Success case
		if envelope.Ok != true {
			t.Error("success envelope should have ok=true")
		}
		if envelope.RowCount < 0 {
			t.Error("row count should be >= 0 on success")
		}
	} else {
		// Error case
		if envelope.Ok != false {
			t.Error("error envelope should have ok=false")
		}
		if envelope.Error == nil {
			t.Error("error envelope should have error field")
		}
	}

	t.Logf("Exit code: %d, stderr: %q", exitCode, stderrStr)
}

// TestStdoutCleanliness_TablesSubcommand verifies tables subcommand stdout cleanliness
func TestStdoutCleanliness_TablesSubcommand(t *testing.T) {
	binary := buildBinary(t)

	cmd := exec.Command(binary, "--dsn", "mysql://invalid:invalid@localhost:3306/mydb", "tables", "mydb")
	cmd.Env = []string{}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	exitCode := runCommand(cmd)
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	// Stdout should contain valid JSON
	var envelope output.Envelope
	err := json.Unmarshal([]byte(stdoutStr), &envelope)
	if err != nil {
		t.Errorf("stdout should contain valid JSON envelope, got parse error: %v\nstdout: %q", err, stdoutStr)
	}

	// Verify envelope structure
	if envelope.Ok == false && envelope.Error == nil {
		t.Error("error envelope should have error field")
	}

	t.Logf("Exit code: %d, stderr: %q", exitCode, stderrStr)
}

// TestStdoutCleanliness_DescribeSubcommand verifies describe subcommand stdout cleanliness
func TestStdoutCleanliness_DescribeSubcommand(t *testing.T) {
	binary := buildBinary(t)

	cmd := exec.Command(binary, "--dsn", "mysql://invalid:invalid@localhost:3306/mydb", "describe", "mydb.users")
	cmd.Env = []string{}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	exitCode := runCommand(cmd)
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	// Stdout should contain valid JSON
	var envelope output.Envelope
	err := json.Unmarshal([]byte(stdoutStr), &envelope)
	if err != nil {
		t.Errorf("stdout should contain valid JSON envelope, got parse error: %v\nstdout: %q", err, stdoutStr)
	}

	// Verify envelope structure
	if envelope.Ok == false && envelope.Error == nil {
		t.Error("error envelope should have error field")
	}

	t.Logf("Exit code: %d, stderr: %q", exitCode, stderrStr)
}

// TestStdoutCleanliness_QuerySubcommand verifies query subcommand stdout cleanliness
func TestStdoutCleanliness_QuerySubcommand(t *testing.T) {
	binary := buildBinary(t)

	tests := []struct {
		name     string
		args     []string
		envVars  []string
		stdin    string
	}{
		{
			name:    "missing SQL",
			args:    []string{"--dsn", "mysql://root:test@localhost:3306/", "query"},
			envVars: []string{},
		},
		{
			name:    "empty SQL",
			args:    []string{"--dsn", "mysql://root:test@localhost:3306/", "query", ""},
			envVars: []string{},
		},
		{
			name:    "safety blocked",
			args:    []string{"--dsn", "mysql://root:test@localhost:3306/", "query", "DROP TABLE users"},
			envVars: []string{},
		},
		{
			name:    "connection error",
			args:    []string{"--dsn", "mysql://invalid:invalid@localhost:3306/", "query", "SELECT 1"},
			envVars: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			cmd.Env = tt.envVars
			if tt.stdin != "" {
				cmd.Stdin = strings.NewReader(tt.stdin)
			}
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			cmd.Stdout = stdout
			cmd.Stderr = stderr

			exitCode := runCommand(cmd)
			stdoutStr := stdout.String()
			stderrStr := stderr.String()

			// Stdout should contain valid JSON
			var envelope output.Envelope
			err := json.Unmarshal([]byte(stdoutStr), &envelope)
			if err != nil {
				t.Errorf("stdout should contain valid JSON envelope, got parse error: %v\nstdout: %q", err, stdoutStr)
			}

			// All test cases are error scenarios, verify error envelope
			if envelope.Ok != false {
				t.Error("error envelope should have ok=false")
			}
			if envelope.Error == nil {
				t.Error("error envelope should have error field")
			}

			t.Logf("Exit code: %d, stderr: %q", exitCode, stderrStr)
		})
	}
}

// TestStdoutCleanliness_AllErrorCodes verifies all error codes produce clean JSON on stdout
func TestStdoutCleanliness_AllErrorCodes(t *testing.T) {
	binary := buildBinary(t)

	tests := []struct {
		name           string
		args           []string
		expectedCode   int
		expectedError  output.ErrorCode
	}{
		{
			name:          "CONFIG_ERROR - missing DSN",
			args:          []string{"databases"},
			expectedCode:  2,
			expectedError: output.ErrorCodeConfigError,
		},
		{
			name:          "QUERY_ERROR - unknown subcommand",
			args:          []string{"--dsn", "mysql://root:test@localhost:3306/", "unknown"},
			expectedCode:  1,
			expectedError: output.ErrorCodeQueryError,
		},
		{
			name:          "CONFIG_ERROR - empty query",
			args:          []string{"--dsn", "mysql://root:test@localhost:3306/", "query"},
			expectedCode:  2,
			expectedError: output.ErrorCodeConfigError,
		},
		{
			name:          "SAFETY_BLOCKED - DROP statement",
			args:          []string{"--dsn", "mysql://root:test@localhost:3306/", "query", "DROP TABLE users"},
			expectedCode:  5,
			expectedError: output.ErrorCodeSafetyBlocked,
		},
		{
			name:          "SAFETY_BLOCKED - DELETE statement",
			args:          []string{"--dsn", "mysql://root:test@localhost:3306/", "query", "DELETE FROM users"},
			expectedCode:  5,
			expectedError: output.ErrorCodeSafetyBlocked,
		},
		{
			name:          "SAFETY_BLOCKED - INSERT statement",
			args:          []string{"--dsn", "mysql://root:test@localhost:3306/", "query", "INSERT INTO users VALUES (1)"},
			expectedCode:  5,
			expectedError: output.ErrorCodeSafetyBlocked,
		},
		{
			name:          "SAFETY_BLOCKED - UPDATE statement",
			args:          []string{"--dsn", "mysql://root:test@localhost:3306/", "query", "UPDATE users SET x=1"},
			expectedCode:  5,
			expectedError: output.ErrorCodeSafetyBlocked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			cmd.Env = []string{} // Clear env
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			cmd.Stdout = stdout
			cmd.Stderr = stderr

			exitCode := runCommand(cmd)
			stdoutStr := stdout.String()
			stderrStr := stderr.String()

			// Verify exit code
			if exitCode != tt.expectedCode {
				t.Errorf("expected exit code %d, got %d", tt.expectedCode, exitCode)
			}

			// Stdout should contain valid JSON error envelope
			var envelope output.Envelope
			err := json.Unmarshal([]byte(stdoutStr), &envelope)
			if err != nil {
				t.Errorf("stdout should contain valid JSON envelope, got parse error: %v\nstdout: %q", err, stdoutStr)
			}

			// Verify error envelope structure
			if envelope.Ok != false {
				t.Error("error envelope should have ok=false")
			}
			if envelope.Error == nil {
				t.Error("error envelope should have error field")
			} else if envelope.Error.Code != tt.expectedError {
				t.Errorf("expected error code %s, got %s", tt.expectedError, envelope.Error.Code)
			}

			t.Logf("stderr: %q", stderrStr)
		})
	}
}

// TestStdoutCleanliness_NewlineTermination verifies all JSON output ends with newline
func TestStdoutCleanliness_NewlineTermination(t *testing.T) {
	binary := buildBinary(t)

	tests := []struct {
		name string
		args []string
	}{
		{"missing DSN", []string{"databases"}},
		{"unknown subcommand", []string{"--dsn", "mysql://root@localhost/", "unknown"}},
		{"empty query", []string{"--dsn", "mysql://root@localhost/", "query", ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			cmd.Env = []string{}
			stdout := &bytes.Buffer{}
			cmd.Stdout = stdout

			_ = runCommand(cmd)
			stdoutStr := stdout.String()

			// Output should end with newline
			if len(stdoutStr) == 0 || stdoutStr[len(stdoutStr)-1] != '\n' {
				t.Errorf("JSON output should end with newline, got: %q", stdoutStr)
			}
		})
	}
}

// TestStdoutCleanliness_NoProgressOnStdout verifies no progress text reaches stdout
func TestStdoutCleanliness_NoProgressOnStdout(t *testing.T) {
	binary := buildBinary(t)

	// Run all subcommands and verify no progress text in stdout
	subcommands := []struct {
		name string
		args []string
	}{
		{"databases", []string{"--dsn", "mysql://root@localhost/", "databases"}},
		{"tables", []string{"--dsn", "mysql://root@localhost/mydb", "tables", "mydb"}},
		{"describe", []string{"--dsn", "mysql://root@localhost/mydb", "describe", "mydb.users"}},
		{"query", []string{"--dsn", "mysql://root@localhost/", "query", "SELECT 1"}},
	}

	for _, tt := range subcommands {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			cmd.Env = []string{}
			stdout := &bytes.Buffer{}
			cmd.Stdout = stdout

			_ = runCommand(cmd)
			stdoutStr := stdout.String()

			// Check for progress indicators that should NOT be in stdout
			progressIndicators := []string{
				"connecting",
				"loading",
				"processing",
				"fetching",
				"executing",
				"running",
				"waiting",
				"...",
				"%",
				"[",
				"]",
				"INFO:",
				"WARN:",
				"DEBUG:",
				"ERROR:",
			}

			stdoutLower := strings.ToLower(stdoutStr)
			for _, indicator := range progressIndicators {
				// Skip if indicator is part of valid JSON (e.g., in a string value)
				// We just need to ensure stdout is valid JSON
				if strings.Contains(stdoutLower, strings.ToLower(indicator)) {
					// Verify it's actually part of valid JSON, not a progress message
					var envelope output.Envelope
					err := json.Unmarshal([]byte(stdoutStr), &envelope)
					if err != nil {
						t.Errorf("found progress indicator %q in stdout, and JSON is invalid: %v\nstdout: %q",
							indicator, err, stdoutStr)
					}
				}
			}

			// Most importantly, verify stdout is valid JSON
			var envelope output.Envelope
			err := json.Unmarshal([]byte(stdoutStr), &envelope)
			if err != nil {
				t.Errorf("stdout should be valid JSON, got parse error: %v\nstdout: %q", err, stdoutStr)
			}
		})
	}
}

// Helper: build the binary and return its path
func buildBinary(t *testing.T) string {
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	binaryPath := filepath.Join(projectRoot, "sql-cli-test")

	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	// Build the binary
	cmd := exec.Command("go", "build", "-o", binaryPath, filepath.Join(projectRoot, "cmd", "sql-cli"))
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build binary: %v\noutput: %s", err, output)
	}

	return binaryPath
}

// Helper: run command and return exit code
func runCommand(cmd *exec.Cmd) int {
	err := cmd.Run()
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

// Helper: min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}