package command

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiezi999/sql-cli/internal/config"
	"github.com/qiezi999/sql-cli/internal/output"
)

// ============================================================
// maskDSN tests
// ============================================================

func TestMaskDSN(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "with password",
			input:    "mysql://user:secret@localhost:3306/dev",
			expected: "mysql://user:****@localhost:3306/dev",
		},
		{
			name:     "without password",
			input:    "mysql://user@localhost:3306/dev",
			expected: "mysql://user@localhost:3306/dev",
		},
		{
			name:     "empty password",
			input:    "mysql://user:@localhost:3306/dev",
			expected: "mysql://user:@localhost:3306/dev",
		},
		{
			name:     "with query string",
			input:    "mysql://user:pass@localhost:3306/dev?timeout=5s",
			expected: "mysql://user:****@localhost:3306/dev?timeout=5s",
		},
		{
			name:     "complex password",
			input:    "mysql://user:P%40ss!w0rd&123@localhost:3306/dev",
			expected: "mysql://user:****@localhost:3306/dev",
		},
		{
			name:     "no password segment",
			input:    "mysql://localhost:3306/dev",
			expected: "mysql://localhost:3306/dev",
		},
		{
			name:     "non-mysql scheme",
			input:    "postgres://user:pass@localhost:5432/dev",
			expected: "postgres://user:pass@localhost:5432/dev",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := maskDSN(tc.input)
			if result != tc.expected {
				t.Errorf("maskDSN(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// ============================================================
// HandleConfig dispatcher tests
// ============================================================

func TestHandleConfig_MissingSubcommand(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := HandleConfig([]string{})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

func TestHandleConfig_UnknownSubcommand(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := HandleConfig([]string{"foo"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

// ============================================================
// handleConfigAdd argument validation tests
// ============================================================

func TestHandleConfigAdd_MissingName(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigAdd([]string{})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

func TestHandleConfigAdd_MissingDSN(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigAdd([]string{"dev"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

func TestHandleConfigAdd_InvalidName_Space(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigAdd([]string{"bad name", "mysql://u:p@h/db"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
	if !strings.Contains(envelope.Error.Message, "invalid profile name") {
		t.Errorf("expected message about invalid profile name, got %s", envelope.Error.Message)
	}
}

func TestHandleConfigAdd_InvalidName_StartsWithDash(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigAdd([]string{"-dev", "mysql://u:p@h/db"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
	if !strings.Contains(envelope.Error.Message, "invalid profile name") {
		t.Errorf("expected message about invalid profile name, got %s", envelope.Error.Message)
	}
}

func TestHandleConfigAdd_InvalidName_StartsWithDot(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigAdd([]string{".dev", "mysql://u:p@h/db"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
	if !strings.Contains(envelope.Error.Message, "invalid profile name") {
		t.Errorf("expected message about invalid profile name, got %s", envelope.Error.Message)
	}
}

func TestHandleConfigAdd_InvalidDSN(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigAdd([]string{"dev", "not-a-url"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
	if !strings.Contains(envelope.Error.Message, "invalid DSN") {
		t.Errorf("expected message about invalid DSN, got %s", envelope.Error.Message)
	}
}

// ============================================================
// handleConfigAdd file creation tests
// ============================================================

func TestHandleConfigAdd_LocalCreatesFile(t *testing.T) {
	// Create a temp directory to act as cwd
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	defer os.Chdir(oldCwd)

	// Remove .sql-cli.yaml if it exists
	localPath := filepath.Join(tmpDir, ".sql-cli.yaml")
	os.Remove(localPath)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigAdd([]string{"dev", "mysql://user:pass@localhost:3306/dev"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// Verify file was created with correct permissions
	info, err := os.Stat(localPath)
	if err != nil {
		t.Fatalf("expected .sql-cli.yaml to be created: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected file permissions 0600, got %o", info.Mode().Perm())
	}

	// Verify YAML content
	pm, err := config.LoadProfileMap(localPath)
	if err != nil {
		t.Fatalf("failed to load profile map: %v", err)
	}
	if pm["dev"] != "mysql://user:pass@localhost:3306/dev" {
		t.Errorf("expected dev profile, got %v", pm)
	}

	// Verify JSON output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if !envelope.Ok {
		t.Errorf("expected success envelope, got error: %+v", envelope.Error)
	}
}

func TestHandleConfigAdd_EmptyFileDoesntPanic(t *testing.T) {
	// Create a temp directory to act as cwd
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	defer os.Chdir(oldCwd)

	// Pre-create an empty file
	localPath := filepath.Join(tmpDir, ".sql-cli.yaml")
	if err := os.WriteFile(localPath, []byte{}, 0o600); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	// Should not panic
	exitCode := handleConfigAdd([]string{"dev", "mysql://user:pass@localhost:3306/dev"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// Verify YAML content
	pm, err := config.LoadProfileMap(localPath)
	if err != nil {
		t.Fatalf("failed to load profile map: %v", err)
	}
	if pm["dev"] != "mysql://user:pass@localhost:3306/dev" {
		t.Errorf("expected dev profile, got %v", pm)
	}
}

func TestHandleConfigAdd_OverwriteAnnouncesOnStderr(t *testing.T) {
	// Create a temp directory to act as cwd
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	defer os.Chdir(oldCwd)

	localPath := filepath.Join(tmpDir, ".sql-cli.yaml")
	os.Remove(localPath)

	// First add
	handleConfigAdd([]string{"dev", "mysql://user:pass@localhost:3306/dev"})

	// Capture stderr for second add
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	handleConfigAdd([]string{"dev", "mysql://admin:newpass@localhost:3306/dev"})

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	stderrStr := buf.String()

	if !strings.Contains(stderrStr, "overwritten") {
		t.Errorf("expected stderr to announce overwrite, got: %s", stderrStr)
	}
}

func TestHandleConfigAdd_SameValueNoStderr(t *testing.T) {
	// Create a temp directory to act as cwd
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	defer os.Chdir(oldCwd)

	localPath := filepath.Join(tmpDir, ".sql-cli.yaml")
	os.Remove(localPath)

	// First add
	handleConfigAdd([]string{"dev", "mysql://user:pass@localhost:3306/dev"})

	// Capture stderr for second add with same value
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	handleConfigAdd([]string{"dev", "mysql://user:pass@localhost:3306/dev"})

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	stderrStr := buf.String()

	if stderrStr != "" {
		t.Errorf("expected no stderr output for same value, got: %s", stderrStr)
	}
}

func TestHandleConfigAdd_WriteFailure(t *testing.T) {
	// Create a temp directory
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	defer os.Chdir(oldCwd)

	// Create a subdirectory and set it read-only
	readonlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readonlyDir, 0o555); err != nil {
		t.Fatalf("failed to create readonly dir: %v", err)
	}

	// Change to that directory
	if err := os.Chdir(readonlyDir); err != nil {
		t.Fatalf("failed to chdir to readonly dir: %v", err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigAdd([]string{"dev", "mysql://user:pass@localhost:3306/dev"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 99 {
		t.Errorf("expected exit code 99, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeInternalError {
		t.Errorf("expected INTERNAL_ERROR, got %s", envelope.Error.Code)
	}
}

// ============================================================
// handleConfigList tests
// ============================================================

func TestHandleConfigList_Empty(t *testing.T) {
	// Create a temp directory to act as cwd
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	defer os.Chdir(oldCwd)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigList([]string{})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if !envelope.Ok {
		t.Errorf("expected success envelope, got error: %+v", envelope.Error)
	}
	if envelope.RowCount != 0 {
		t.Errorf("expected 0 rows, got %d", envelope.RowCount)
	}
}

func TestHandleConfigList_PositionalArgsRejected(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigList([]string{"extra"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

func TestHandleConfigList_MutuallyExclusiveFlags(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigList([]string{"--local", "--global"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

func TestHandleConfigList_LocalOnly(t *testing.T) {
	// Create a temp directory to act as cwd
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	defer os.Chdir(oldCwd)

	// Pre-create local config with a profile
	localPath := filepath.Join(tmpDir, ".sql-cli.yaml")
	pm := config.ProfileMap{"localonly": "mysql://user:pass@localhost:3306/db"}
	if err := config.WriteProfileMap(localPath, pm); err != nil {
		t.Fatalf("failed to write profile map: %v", err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigList([]string{"--local"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var result struct {
		Ok       bool `json:"ok"`
		Profiles []struct {
			Name   string `json:"name"`
			DSN    string `json:"dsn"`
			Source string `json:"source"`
		} `json:"profiles"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if !result.Ok {
		t.Errorf("expected success envelope, got ok=false")
	}
	if result.Count != 1 {
		t.Errorf("expected count=1, got %d", result.Count)
	}
	if len(result.Profiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(result.Profiles))
	}

	// Verify DSN is masked
	if len(result.Profiles) > 0 {
		if result.Profiles[0].DSN != "mysql://user:****@localhost:3306/db" {
			t.Errorf("expected masked DSN, got %s", result.Profiles[0].DSN)
		}
		// Normalize path for macOS /var -> /private/var symlink
		normalizedSource, _ := filepath.EvalSymlinks(result.Profiles[0].Source)
		normalizedLocalPath, _ := filepath.EvalSymlinks(localPath)
		if normalizedSource != normalizedLocalPath {
			t.Errorf("expected source=%s, got %s", localPath, result.Profiles[0].Source)
		}
	}
}

func TestHandleConfigList_GlobalOnly(t *testing.T) {
	// Create a temp directory and set HOME to it
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Pre-create global config
	globalPath := filepath.Join(tmpDir, ".sql-cli", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("failed to create global dir: %v", err)
	}
	pm := config.ProfileMap{"globalonly": "mysql://user:pass@localhost:3306/db"}
	if err := config.WriteProfileMap(globalPath, pm); err != nil {
		t.Fatalf("failed to write profile map: %v", err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := handleConfigList([]string{"--global"})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var result struct {
		Ok       bool `json:"ok"`
		Profiles []struct {
			Name   string `json:"name"`
			DSN    string `json:"dsn"`
			Source string `json:"source"`
		} `json:"profiles"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if !result.Ok {
		t.Errorf("expected success envelope, got ok=false")
	}
	if result.Count != 1 {
		t.Errorf("expected count=1, got %d", result.Count)
	}
	if len(result.Profiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(result.Profiles))
	}
	if len(result.Profiles) > 0 {
		if result.Profiles[0].Name != "globalonly" {
			t.Errorf("expected profile name 'globalonly', got %s", result.Profiles[0].Name)
		}
		if result.Profiles[0].DSN != "mysql://user:****@localhost:3306/db" {
			t.Errorf("expected masked DSN, got %s", result.Profiles[0].DSN)
		}
		if result.Profiles[0].Source != globalPath {
			t.Errorf("expected source=%s, got %s", globalPath, result.Profiles[0].Source)
		}
	}
}

func TestHandleConfigList_MergedView_LocalOverrides(t *testing.T) {
	// Create a temp directory and set HOME to it
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	defer os.Chdir(oldCwd)

	// Pre-create global config
	globalPath := filepath.Join(tmpDir, ".sql-cli", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatalf("failed to create global dir: %v", err)
	}
	pmGlobal := config.ProfileMap{
		"global":    "mysql://user:pass@global:3306/db",
		"sharedkey": "mysql://user:pass@global:3306/db",
	}
	if err := config.WriteProfileMap(globalPath, pmGlobal); err != nil {
		t.Fatalf("failed to write global profile map: %v", err)
	}

	// Pre-create local config that overrides sharedkey
	localPath := filepath.Join(tmpDir, ".sql-cli.yaml")
	pmLocal := config.ProfileMap{
		"local":     "mysql://user:pass@local:3306/db",
		"sharedkey": "mysql://user:pass@local:3306/db",
	}
	if err := config.WriteProfileMap(localPath, pmLocal); err != nil {
		t.Fatalf("failed to write local profile map: %v", err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Merged view (no flags)
	exitCode := handleConfigList([]string{})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var result struct {
		Ok       bool `json:"ok"`
		Profiles []struct {
			Name   string `json:"name"`
			DSN    string `json:"dsn"`
			Source string `json:"source"`
		} `json:"profiles"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if !result.Ok {
		t.Errorf("expected success envelope, got ok=false")
	}

	// Should have 3 profiles: global, local, sharedkey (local overrides)
	if result.Count != 3 {
		t.Errorf("expected count=3, got %d", result.Count)
	}
	if len(result.Profiles) != 3 {
		t.Errorf("expected 3 profiles, got %d", len(result.Profiles))
	}

	// Verify alphabetical order
	names := make([]string, 0, len(result.Profiles))
	for _, p := range result.Profiles {
		names = append(names, p.Name)
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("expected alphabetical order, got %v", names)
			break
		}
	}

	// Verify sharedkey was overridden (local value)
	foundSharedkey := false
	for _, p := range result.Profiles {
		if p.Name == "sharedkey" {
			foundSharedkey = true
			if p.DSN != "mysql://user:****@local:3306/db" {
				t.Errorf("expected sharedkey to be overridden by local, got %s", p.DSN)
			}
			// Normalize path for macOS /var -> /private/var symlink
			normalizedSource, _ := filepath.EvalSymlinks(p.Source)
			normalizedLocalPath, _ := filepath.EvalSymlinks(localPath)
			if normalizedSource != normalizedLocalPath {
				t.Errorf("expected sharedkey source=%s, got %s", localPath, p.Source)
			}
		}
	}
	if !foundSharedkey {
		t.Error("expected to find sharedkey profile")
	}
}