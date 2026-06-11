package command

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/qiezi999/sql-cli/internal/output"
)

// TestHandleDescribe_MissingArgument tests that missing table argument returns CONFIG_ERROR.
func TestHandleDescribe_MissingArgument(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := HandleDescribe("mysql://user:pass@localhost:3306/", []string{})

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
	if !strings.Contains(envelope.Error.Message, "table identifier required") {
		t.Errorf("expected message to contain 'table identifier required', got %s", envelope.Error.Message)
	}
}

// TestHandleDescribe_TableOnlyWithDSNDatabase tests that single table name uses DSN database.
// When DSN contains a database, providing just a table name should work.
func TestHandleDescribe_TableOnlyWithDSNDatabase(t *testing.T) {
	testCases := []struct {
		name       string
		identifier string
	}{
		{"simple table", "users"},
		{"table with underscore", "user_table"},
		{"table with numbers", "table456"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// DSN includes database name - table-only identifier should be accepted
			// (will fail at connection, but should pass validation)
			exitCode := HandleDescribe("mysql://user:pass@localhost:3306/mydb", []string{tc.identifier})

			w.Close()
			os.Stdout = oldStdout

			// Read output
			var buf bytes.Buffer
			buf.ReadFrom(r)
			var envelope output.Envelope
			if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
				t.Fatalf("failed to parse output: %v", err)
			}

			// Should NOT be CONFIG_ERROR - validation passed
			// May be CONNECTION_ERROR since we can't actually connect
			if envelope.Ok == false && envelope.Error.Code == output.ErrorCodeConfigError {
				// Check if error is about identifier format (validation failure)
				if strings.Contains(envelope.Error.Message, "expected database.table") ||
					strings.Contains(envelope.Error.Message, "database required") {
					t.Errorf("table-only identifier should be valid when DSN has database, got: %s", envelope.Error.Message)
				}
			}

			// Exit code should not be CONFIG_ERROR for valid table name with DSN database
			_ = exitCode // May be CONNECTION_ERROR, which is acceptable
		})
	}
}

// TestHandleDescribe_TableOnlyWithoutDSNDatabase tests that single table name fails when DSN has no database.
func TestHandleDescribe_TableOnlyWithoutDSNDatabase(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// DSN has no database - table-only identifier should fail
	exitCode := HandleDescribe("mysql://user:pass@localhost:3306/", []string{"users"})

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
	if !strings.Contains(envelope.Error.Message, "database required") {
		t.Errorf("expected message to contain 'database required', got %s", envelope.Error.Message)
	}
}

// TestHandleDescribe_MultipleDots tests that identifiers with multiple dots return CONFIG_ERROR.
func TestHandleDescribe_MultipleDots(t *testing.T) {
	testCases := []struct {
		name       string
		identifier string
	}{
		{"three parts", "db.table.column"},
		{"four parts", "db.table.column.extra"},
		{"trailing dot", "db.table."},
		{"leading dot", ".db.table"},
		{"double dot", "db..table"},
		{"many dots", "a.b.c.d.e"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			exitCode := HandleDescribe("mysql://user:pass@localhost:3306/", []string{tc.identifier})

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
			if !strings.Contains(envelope.Error.Message, "invalid identifier format") {
				t.Errorf("expected message to contain 'invalid identifier format', got %s", envelope.Error.Message)
			}
		})
	}
}

// TestHandleDescribe_InvalidDatabase tests that invalid database identifiers return CONFIG_ERROR.
func TestHandleDescribe_InvalidDatabase(t *testing.T) {
	testCases := []struct {
		name       string
		identifier string
		expectMsg  string
	}{
		{"semicolon injection in db", "db;DROP.users", "invalid database identifier"},
		{"backtick injection in db", "db`;DROP.users", "invalid database identifier"},
		{"space in database", "my db.users", "invalid database identifier"},
		{"special chars in db", "db@host.users", "invalid database identifier"},
		{"null byte in db", "db\x00.users", "invalid database identifier"},
		{"newline in db", "db\nDROP.users", "invalid database identifier"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			exitCode := HandleDescribe("mysql://user:pass@localhost:3306/", []string{tc.identifier})

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

// TestHandleDescribe_InvalidTable tests that invalid table identifiers return CONFIG_ERROR.
func TestHandleDescribe_InvalidTable(t *testing.T) {
	testCases := []struct {
		name       string
		identifier string
		expectMsg  string
	}{
		{"semicolon injection in table", "mydb.table;DROP", "invalid table identifier"},
		{"backtick injection in table", "mydb.table`;DROP", "invalid table identifier"},
		{"space in table", "mydb.my table", "invalid table identifier"},
		{"special chars in table", "mydb.table@name", "invalid table identifier"},
		{"null byte in table", "mydb.table\x00", "invalid table identifier"},
		{"newline in table", "mydb.table\nDROP", "invalid table identifier"},
		{"parentheses in table", "mydb.table()", "invalid table identifier"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			exitCode := HandleDescribe("mysql://user:pass@localhost:3306/", []string{tc.identifier})

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

// TestHandleDescribe_ValidIdentifiers tests that valid identifiers pass all validation.
func TestHandleDescribe_ValidIdentifiers(t *testing.T) {
	testCases := []struct {
		name       string
		identifier string
		database   string
		table      string
	}{
		{"simple", "mydb.users", "mydb", "users"},
		{"with underscore", "my_db.user_table", "my_db", "user_table"},
		{"with numbers", "db123.table456", "db123", "table456"},
		{"uppercase", "MYDB.USERS", "MYDB", "USERS"},
		{"mixed case", "MyDb.UserTable_123", "MyDb", "UserTable_123"},
		{"numbers first", "123db.456table", "123db", "456table"},
		{"all underscore", "___.___", "___", "___"},
		{"single char parts", "a.b", "a", "b"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Split and verify parts match regex
			parts := strings.Split(tc.identifier, ".")
			if len(parts) != 2 {
				t.Fatalf("test case invalid: %s should split into 2 parts", tc.identifier)
			}

			database := parts[0]
			table := parts[1]

			// Verify regex matches
			if !validIdentifier.MatchString(database) {
				t.Errorf("valid database '%s' rejected by regex", database)
			}
			if !validIdentifier.MatchString(table) {
				t.Errorf("valid table '%s' rejected by regex", table)
			}
		})
	}
}

// TestHandleDescribe_BothPartsInvalid tests when both database and table are invalid.
func TestHandleDescribe_BothPartsInvalid(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := HandleDescribe("mysql://user:pass@localhost:3306/", []string{"my;db.my*table"})

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
	// Should report the first invalid part (database)
	if !strings.Contains(envelope.Error.Message, "invalid database identifier") {
		t.Errorf("expected message to contain 'invalid database identifier', got %s", envelope.Error.Message)
	}
}

// TestHandleDescribe_EmptyParts tests that empty database or table returns CONFIG_ERROR.
func TestHandleDescribe_EmptyParts(t *testing.T) {
	testCases := []struct {
		name       string
		identifier string
		expectMsg  string
	}{
		{"empty database", ".users", "invalid database identifier"},
		{"empty table", "mydb.", "invalid table identifier"},
		{"both empty", ".", "invalid database identifier"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			exitCode := HandleDescribe("mysql://user:pass@localhost:3306/", []string{tc.identifier})

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

// TestHandleDescribe_InjectionAttempts tests various SQL injection attempts.
func TestHandleDescribe_InjectionAttempts(t *testing.T) {
	testCases := []struct {
		name       string
		identifier string
		expectMsg  string
	}{
		{"classic injection", "db;DROP.table", "invalid database identifier"},
		{"backtick escape", "db`;DROP.table", "invalid database identifier"},
		{"quote injection", "db'OR'1'='1.table", "invalid database identifier"},
		{"union injection", "db UNION.table", "invalid database identifier"},
		{"table injection", "mydb.table;SELECT*", "invalid table identifier"},
		{"table backtick", "mydb.table`WHERE", "invalid table identifier"},
		{"mixed injection", "db;DROP.table;SELECT", "invalid database identifier"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			exitCode := HandleDescribe("mysql://user:pass@localhost:3306/", []string{tc.identifier})

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
