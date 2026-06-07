package command

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/qiezi999/sql-cli/internal/output"
)

// TestHandleTables_MissingArgument tests that missing database argument returns CONFIG_ERROR.
func TestHandleTables_MissingArgument(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := HandleTables("mysql://user:pass@localhost:3306/", []string{})

	w.Close()
	os.Stdout = oldStdout

	// Verify exit code
	if exitCode != 2 {
		t.Errorf("expected exit code 2 (CONFIG_ERROR), got %d", exitCode)
	}

	// Read and parse output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	// Verify error envelope
	if envelope.Ok {
		t.Error("expected error envelope (ok=false), got success envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected error code CONFIG_ERROR, got %s", envelope.Error.Code)
	}
	if !strings.Contains(envelope.Error.Message, "database name required") {
		t.Errorf("expected message to contain 'database name required', got %s", envelope.Error.Message)
	}
}

// TestHandleTables_InvalidIdentifier tests that invalid identifiers are rejected.
func TestHandleTables_InvalidIdentifier(t *testing.T) {
	testCases := []struct {
		name       string
		database   string
		expectMsg  string
	}{
		{
			name:      "semicolon injection",
			database:  "db; DROP TABLE users; --",
			expectMsg: "invalid database identifier",
		},
		{
			name:      "backtick injection",
			database:  "app`; DROP TABLE x; --`",
			expectMsg: "invalid database identifier",
		},
		{
			name:      "quote injection",
			database:  "db' OR '1'='1",
			expectMsg: "invalid database identifier",
		},
		{
			name:      "dash in name",
			database:  "my-database",
			expectMsg: "invalid database identifier",
		},
		{
			name:      "dot in name",
			database:  "my.database",
			expectMsg: "invalid database identifier",
		},
		{
			name:      "space in name",
			database:  "my database",
			expectMsg: "invalid database identifier",
		},
		{
			name:      "special characters",
			database:  "db@host",
			expectMsg: "invalid database identifier",
		},
		{
			name:      "parentheses injection",
			database:  "db()",
			expectMsg: "invalid database identifier",
		},
		{
			name:      "null byte attempt",
			database:  "db\x00",
			expectMsg: "invalid database identifier",
		},
		{
			name:      "newline injection",
			database:  "db\nDROP TABLE users",
			expectMsg: "invalid database identifier",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			exitCode := HandleTables("mysql://user:pass@localhost:3306/", []string{tc.database})

			w.Close()
			os.Stdout = oldStdout

			// Verify exit code
			if exitCode != 2 {
				t.Errorf("expected exit code 2 (CONFIG_ERROR), got %d", exitCode)
			}

			// Read and parse output
			var buf bytes.Buffer
			buf.ReadFrom(r)
			var envelope output.Envelope
			if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
				t.Fatalf("failed to parse output: %v", err)
			}

			// Verify error envelope
			if envelope.Ok {
				t.Error("expected error envelope (ok=false), got success envelope")
			}
			if envelope.Error.Code != output.ErrorCodeConfigError {
				t.Errorf("expected error code CONFIG_ERROR, got %s", envelope.Error.Code)
			}
			if !strings.Contains(envelope.Error.Message, tc.expectMsg) {
				t.Errorf("expected message to contain '%s', got %s", tc.expectMsg, envelope.Error.Message)
			}
		})
	}
}

// TestHandleTables_ValidIdentifiers tests that valid identifiers pass regex validation.
func TestHandleTables_ValidIdentifiers(t *testing.T) {
	testCases := []struct {
		name     string
		database string
	}{
		{"simple", "mydb"},
		{"with_underscore", "my_db"},
		{"with_numbers", "db123"},
		{"uppercase", "MYDB"},
		{"mixed_case", "MyDb_123"},
		{"numbers_first", "123db"},
		{"all_underscore", "___"},
		{"single_char", "a"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify the identifier matches the regex
			if !validIdentifier.MatchString(tc.database) {
				t.Errorf("valid identifier '%s' rejected by regex", tc.database)
			}
		})
	}
}

// TestHandleTables_RegexBoundaryCases tests edge cases for identifier validation.
func TestHandleTables_RegexBoundaryCases(t *testing.T) {
	testCases := []struct {
		name      string
		database  string
		shouldMinch bool
	}{
		// Valid cases
		{"empty string", "", false},
		{"single letter", "a", true},
		{"single number", "1", true},
		{"single underscore", "_", true},
		{"max length typical", strings.Repeat("a", 64), true},

		// Invalid cases - special chars
		{"contains dash", "my-db", false},
		{"contains dot", "my.db", false},
		{"contains space", "my db", false},
		{"contains at", "db@test", false},
		{"contains hash", "db#test", false},
		{"contains dollar", "db$test", false},
		{"contains percent", "db%test", false},
		{"contains ampersand", "db&test", false},
		{"contains asterisk", "db*test", false},
		{"contains plus", "db+test", false},
		{"contains equals", "db=test", false},
		{"contains question", "db?test", false},
		{"contains exclamation", "db!test", false},
		{"contains tilde", "db~test", false},
		{"contains backtick", "db`test", false},
		{"contains quote", "db'test", false},
		{"contains double_quote", `db"test`, false},
		{"contains backslash", `db\test`, false},
		{"contains slash", "db/test", false},
		{"contains pipe", "db|test", false},
		{"contains brackets", "db[test]", false},
		{"contains braces", "db{test}", false},
		{"contains parens", "db(test)", false},
		{"contains angle", "db<test>", false},
		{"contains comma", "db,test", false},
		{"contains colon", "db:test", false},
		{"contains semicolon", "db;test", false},
		{"contains newline", "db\ntest", false},
		{"contains tab", "db\ttest", false},
		{"contains null", "db\x00test", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matched := validIdentifier.MatchString(tc.database)
			if matched != tc.shouldMinch {
				t.Errorf("identifier '%s': expected match=%v, got match=%v", tc.database, tc.shouldMinch, matched)
			}
		})
	}
}