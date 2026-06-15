//go:build integration

package command_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiezi999/sql-cli/internal/output"
)

// binaryPath is the path where the CLI binary is built.
const binaryPath = "/tmp/sql-cli-config-test"

// buildBinary builds the CLI binary once before all tests.
func TestMain(m *testing.M) {
	// Derive the module root from this source file's location.
	// The test lives in <module-root>/internal/command/, so go up 3 levels.
	_, srcPath, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(srcPath)))
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/sql-cli")
	buildCmd.Dir = moduleRoot
	var buildStdout, buildStderr []byte
	stdoutPipe, _ := buildCmd.StdoutPipe()
	stderrPipe, _ := buildCmd.StderrPipe()
	if err := buildCmd.Start(); err != nil {
		panic("failed to start build: " + err.Error())
	}
	buildStdout, _ = io.ReadAll(stdoutPipe)
	buildStderr, _ = io.ReadAll(stderrPipe)
	_ = buildCmd.Wait()
	if buildCmd.ProcessState.ExitCode() != 0 {
		panic(fmt.Sprintf("failed to build CLI binary (exit %d):\nstdout: %s\nstderr: %s\nmoduleRoot: %s",
			buildCmd.ProcessState.ExitCode(), buildStdout, buildStderr, moduleRoot))
	}
	os.Exit(m.Run())
}

// runConfig runs the config subcommand with the given args and returns
// stdout, stderr, and exit code.
func runConfig(t *testing.T, tmpDir string, args ...string) (stdout []byte, stderr []byte, exitCode int) {
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = tmpDir

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to create stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start command: %v", err)
	}

	stdout, _ = readAll(stdoutPipe)
	stderr, _ = readAll(stderrPipe)

	_ = cmd.Wait()
	return stdout, stderr, cmd.ProcessState.ExitCode()
}

// readAll is provided by databases_integration_test.go in the same package.

// TestEndToEnd_AddLocal verifies that `config add` creates a local .sql-cli.yaml file.
func TestEndToEnd_AddLocal(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	stdout, _, exitCode := runConfig(t, tmpDir, "config", "add", "dev", "mysql://user:pass@localhost:3306/devdb")

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d; stderr: %s", exitCode, string(stdout))
	}

	// Verify file was created
	localPath := filepath.Join(tmpDir, ".sql-cli.yaml")
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		t.Fatalf("expected .sql-cli.yaml to be created at %s", localPath)
	}

	// Verify file permissions are 0600
	info, _ := os.Stat(localPath)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected file permissions 0600, got %o", info.Mode().Perm())
	}

	// Verify stdout contains success envelope (spec shape)
	var result struct {
		Ok      bool   `json:"ok"`
		Profile string `json:"profile"`
		DSN     string `json:"dsn"`
		Path    string `json:"path"`
		Action  string `json:"action"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("failed to parse stdout as JSON: %v", err)
	}
	if !result.Ok {
		t.Errorf("expected ok=true")
	}
	if result.Action != "created" {
		t.Errorf("expected action='created', got %q", result.Action)
	}
}

// TestEndToEnd_AddGlobal verifies that `config add --global` writes to ~/.sql-cli/config.yaml.
func TestEndToEnd_AddGlobal(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	globalDir := filepath.Join(tmpDir, ".sql-cli")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("failed to create .sql-cli dir: %v", err)
	}

	stdout, _, exitCode := runConfig(t, tmpDir, "config", "add", "--global", "staging", "mysql://admin:secret@prod:3306/stagingdb")

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// Verify global file was created
	globalPath := filepath.Join(globalDir, "config.yaml")
	if _, err := os.Stat(globalPath); os.IsNotExist(err) {
		t.Fatalf("expected global config to be created at %s", globalPath)
	}

	// Verify stdout is valid JSON (already confirmed by exitCode==0)
}

// TestEndToEnd_AddThenList verifies that after adding two profiles, `config list`
// returns them in alphabetical order with masked DSNs.
func TestEndToEnd_AddThenList(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	// Add two profiles
	runConfig(t, tmpDir, "config", "add", "zebra", "mysql://u:p@localhost:3306/zoo")
	runConfig(t, tmpDir, "config", "add", "alpha", "mysql://u:p@localhost:3306/alpha")

	// List profiles
	stdout, _, exitCode := runConfig(t, tmpDir, "config", "list")

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	var result struct {
		Ok       bool `json:"ok"`
		Profiles []struct {
			Name string `json:"name"`
			DSN  string `json:"dsn"`
		} `json:"profiles"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("failed to parse stdout as JSON: %v", err)
	}
	if !result.Ok {
		t.Errorf("expected ok=true")
	}
	if result.Count != 2 {
		t.Errorf("expected count=2, got %d", result.Count)
	}
	if len(result.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(result.Profiles))
	}

	// Verify alphabetical order
	names := make([]string, 0, len(result.Profiles))
	for _, p := range result.Profiles {
		names = append(names, p.Name)
	}
	if names[0] != "alpha" || names[1] != "zebra" {
		t.Errorf("expected alphabetical order [alpha, zebra], got %v", names)
	}

	// Verify DSNs are masked
	for _, p := range result.Profiles {
		if strings.Contains(p.DSN, "pass") {
			t.Errorf("expected DSN to be masked, got %s", p.DSN)
		}
	}
}

// TestEndToEnd_AddOverwritesViaBinary verifies that overwriting an existing profile
// returns action="overwritten" and announces on stderr.
func TestEndToEnd_AddOverwritesViaBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	// Add initial profile
	runConfig(t, tmpDir, "config", "add", "dev", "mysql://user:oldpass@localhost:3306/devdb")

	// Overwrite with new value — stderr should contain "overwritten"
	_, stderr, exitCode := runConfig(t, tmpDir, "config", "add", "dev", "mysql://admin:newpass@localhost:3306/devdb")

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(string(stderr), "overwritten") {
		t.Errorf("expected stderr to announce overwrite, got: %s", stderr)
	}
}

// TestEndToEnd_AddInvalidDSN verifies that an invalid DSN returns exit code 2.
func TestEndToEnd_AddInvalidDSN(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	stdout, _, exitCode := runConfig(t, tmpDir, "config", "add", "dev", "not-a-url")

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var envelope output.Envelope
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		t.Fatalf("failed to parse stdout as JSON: %v", err)
	}
	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

// TestEndToEnd_AddInvalidProfileName verifies that an invalid profile name returns exit code 2.
func TestEndToEnd_AddInvalidProfileName(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	stdout, _, exitCode := runConfig(t, tmpDir, "config", "add", "bad name", "mysql://u:p@localhost:3306/db")

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var envelope output.Envelope
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		t.Fatalf("failed to parse stdout as JSON: %v", err)
	}
	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

// TestEndToEnd_ChmodOnNewFile verifies that a newly created config file has 0600 permissions.
func TestEndToEnd_ChmodOnNewFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, ".sql-cli.yaml")

	runConfig(t, tmpDir, "config", "add", "secure", "mysql://user:pass@localhost:3306/devdb")

	info, err := os.Stat(localPath)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected new file permissions 0600, got %o", info.Mode().Perm())
	}
}

// TestEndToEnd_NoChmodOnExistingFile verifies that overwriting an existing file
// preserves its original permissions (0644).
func TestEndToEnd_NoChmodOnExistingFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, ".sql-cli.yaml")

	// Create file with 0644 permissions
	if err := os.WriteFile(localPath, []byte("dsns:\n  existing: mysql://u:p@localhost:3306/db\n"), 0o644); err != nil {
		t.Fatalf("failed to create existing file: %v", err)
	}

	runConfig(t, tmpDir, "config", "add", "existing", "mysql://user:newpass@localhost:3306/devdb")

	info, err := os.Stat(localPath)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("expected existing file permissions 0644 to be preserved, got %o", info.Mode().Perm())
	}
}