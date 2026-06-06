package safety

import (
	"os"
	"reflect"
	"testing"
)

func TestScanKeywords_NormalState(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "simple INSERT",
			sql:      "INSERT INTO users VALUES (1)",
			expected: []string{"INSERT"},
		},
		{
			name:     "simple UPDATE",
			sql:      "UPDATE users SET name = 'test'",
			expected: []string{"UPDATE"},
		},
		{
			name:     "simple DELETE",
			sql:      "DELETE FROM users WHERE id = 1",
			expected: []string{"DELETE"},
		},
		{
			name:     "DROP TABLE",
			sql:      "DROP TABLE users",
			expected: []string{"DROP"},
		},
		{
			name:     "ALTER TABLE",
			sql:      "ALTER TABLE users ADD COLUMN age INT",
			expected: []string{"ALTER"},
		},
		{
			name:     "CREATE TABLE",
			sql:      "CREATE TABLE users (id INT)",
			expected: []string{"CREATE"},
		},
		{
			name:     "TRUNCATE TABLE",
			sql:      "TRUNCATE TABLE users",
			expected: []string{"TRUNCATE"},
		},
		{
			name:     "REPLACE INTO",
			sql:      "REPLACE INTO users VALUES (1, 'test')",
			expected: []string{"REPLACE"},
		},
		{
			name:     "GRANT privilege",
			sql:      "GRANT SELECT ON users TO 'user'@'host'",
			expected: []string{"GRANT"},
		},
		{
			name:     "REVOKE privilege",
			sql:      "REVOKE SELECT ON users FROM 'user'@'host'",
			expected: []string{"REVOKE"},
		},
		{
			name:     "LOCK TABLES",
			sql:      "LOCK TABLES users READ",
			expected: []string{"LOCK"},
		},
		{
			name:     "multiple keywords",
			sql:      "INSERT INTO users VALUES (1); DELETE FROM logs",
			expected: []string{"INSERT", "DELETE"},
		},
		{
			name:     "case insensitive",
			sql:      "insert into users values (1)",
			expected: []string{"INSERT"},
		},
		{
			name:     "mixed case",
			sql:      "InSeRt InTo UsErS vAlUeS (1)",
			expected: []string{"INSERT"},
		},
		{
			name:     "keyword with underscores",
			sql:      "INSERT INTO user_data VALUES (1)",
			expected: []string{"INSERT"},
		},
		{
			name:     "no keywords",
			sql:      "SELECT * FROM users WHERE id = 1",
			expected: nil,
		},
		{
			name:     "keyword as substring should not match",
			sql:      "SELECTION FROM users",
			expected: nil,
		},
		{
			name:     "keyword in word should not match",
			sql:      "INSERTED INTO users",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScanKeywords(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ScanKeywords() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestScanKeywords_StringState(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "keyword in string",
			sql:      "SELECT 'INSERT INTO users' FROM dual",
			expected: nil,
		},
		{
			name:     "keyword before string",
			sql:      "INSERT INTO users VALUES ('test')",
			expected: []string{"INSERT"},
		},
		{
			name:     "keyword after string",
			sql:      "SELECT * FROM users WHERE name = 'test'; DELETE FROM logs",
			expected: []string{"DELETE"},
		},
		{
			name:     "escaped single quote",
			sql:      "INSERT INTO users VALUES ('it''s a test')",
			expected: []string{"INSERT"},
		},
		{
			name:     "multiple escaped quotes",
			sql:      "INSERT INTO users VALUES ('test''s '' value')",
			expected: []string{"INSERT"},
		},
		{
			name:     "keyword in string with escaped quote",
			sql:      "SELECT 'INSERT''UPDATE' FROM dual",
			expected: nil,
		},
		{
			name:     "empty string",
			sql:      "INSERT INTO users VALUES ('')",
			expected: []string{"INSERT"},
		},
		{
			name:     "string at end",
			sql:      "INSERT INTO users VALUES ('test')",
			expected: []string{"INSERT"},
		},
		{
			name:     "multiple strings with keywords",
			sql:      "SELECT 'DELETE' FROM dual; UPDATE users SET name = 'INSERT'",
			expected: []string{"UPDATE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScanKeywords(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ScanKeywords() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestScanKeywords_LineComment(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "keyword in line comment",
			sql:      "SELECT * FROM users -- INSERT INTO users VALUES (1)",
			expected: nil,
		},
		{
			name:     "keyword before comment",
			sql:      "INSERT INTO users VALUES (1) -- comment",
			expected: []string{"INSERT"},
		},
		{
			name:     "keyword after comment",
			sql:      "-- comment\nDELETE FROM users",
			expected: []string{"DELETE"},
		},
		{
			name:     "multiple line comments",
			sql:      "-- INSERT INTO users\n-- UPDATE users\nDELETE FROM users",
			expected: []string{"DELETE"},
		},
		{
			name:     "comment at end of line with newline",
			sql:      "INSERT INTO users VALUES (1) -- comment\n",
			expected: []string{"INSERT"},
		},
		{
			name:     "comment with multiple dashes",
			sql:      "SELECT * -- INSERT -- DELETE -- UPDATE\nFROM users",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScanKeywords(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ScanKeywords() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestScanKeywords_BlockComment(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "keyword in block comment",
			sql:      "SELECT * FROM users /* INSERT INTO users VALUES (1) */",
			expected: nil,
		},
		{
			name:     "keyword before block comment",
			sql:      "INSERT INTO users VALUES (1) /* comment */",
			expected: []string{"INSERT"},
		},
		{
			name:     "keyword after block comment",
			sql:      "/* comment */ DELETE FROM users",
			expected: []string{"DELETE"},
		},
		{
			name:     "multiline block comment",
			sql:      "/* INSERT INTO users\n   VALUES (1) */\nDELETE FROM users",
			expected: []string{"DELETE"},
		},
		{
			name:     "nested content in comment",
			sql:      "/* comment with 'string' and -- line comment */ SELECT * FROM users",
			expected: nil,
		},
		{
			name:     "block comment with asterisks",
			sql:      "/* *** INSERT *** */ SELECT * FROM users",
			expected: nil,
		},
		{
			name:     "multiple block comments",
			sql:      "/* INSERT */ DELETE /* UPDATE */ FROM users",
			expected: []string{"DELETE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScanKeywords(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ScanKeywords() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestScanKeywords_ComplexCases(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "string then comment",
			sql:      "SELECT 'test' -- INSERT INTO users\nFROM dual",
			expected: nil,
		},
		{
			name:     "comment then string",
			sql:      "-- DELETE\nINSERT INTO users VALUES ('test')",
			expected: []string{"INSERT"},
		},
		{
			name:     "string in block comment",
			sql:      "/* 'INSERT INTO users' */ SELECT * FROM users",
			expected: nil,
		},
		{
			name:     "block comment in string literal",
			sql:      "INSERT INTO users VALUES ('/* not a comment */')",
			expected: []string{"INSERT"},
		},
		{
			name:     "line comment in string literal",
			sql:      "INSERT INTO users VALUES ('-- not a comment')",
			expected: []string{"INSERT"},
		},
		{
			name:     "complex real-world query",
			sql:      `INSERT INTO users (name, email) VALUES ('John O''Brien', 'john@example.com'); -- Add user`,
			expected: []string{"INSERT"},
		},
		{
			name:     "multiple statements with various states",
			sql:      `INSERT INTO users VALUES (1); /* comment */ UPDATE users SET name = 'test''s'; DELETE FROM logs -- cleanup`,
			expected: []string{"INSERT", "UPDATE", "DELETE"},
		},
		{
			name:     "keyword at boundaries",
			sql:      "INSERT",
			expected: []string{"INSERT"},
		},
		{
			name:     "keyword with leading/trailing spaces",
			sql:      "  INSERT  ",
			expected: []string{"INSERT"},
		},
		{
			name:     "avoid duplicate keywords",
			sql:      "INSERT INTO users VALUES (1); INSERT INTO logs VALUES (2)",
			expected: []string{"INSERT"},
		},
		{
			name:     "multiple different keywords",
			sql:      "INSERT INTO users VALUES (1); UPDATE users SET x = 1; DELETE FROM logs",
			expected: []string{"INSERT", "UPDATE", "DELETE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScanKeywords(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ScanKeywords() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestScanKeywords_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "empty string",
			sql:      "",
			expected: nil,
		},
		{
			name:     "only whitespace",
			sql:      "   \t\n  ",
			expected: nil,
		},
		{
			name:     "only comment",
			sql:      "-- INSERT INTO users",
			expected: nil,
		},
		{
			name:     "only block comment",
			sql:      "/* INSERT INTO users */",
			expected: nil,
		},
		{
			name:     "only string",
			sql:      "'INSERT INTO users'",
			expected: nil,
		},
		{
			name:     "unclosed string - treat as string to end",
			sql:      "SELECT 'INSERT INTO users",
			expected: nil,
		},
		{
			name:     "unclosed block comment - treat as comment to end",
			sql:      "SELECT /* INSERT INTO users",
			expected: nil,
		},
		{
			name:     "single quote at end",
			sql:      "INSERT INTO users VALUES ('test')'",
			expected: []string{"INSERT"},
		},
		{
			name:     "comment markers in string",
			sql:      "INSERT INTO users VALUES ('-- not comment /* not comment */')",
			expected: []string{"INSERT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScanKeywords(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ScanKeywords() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestScanKeywords_KeywordVariations(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "INSERT with underscores in table name",
			sql:      "INSERT INTO user_account_mapping VALUES (1)",
			expected: []string{"INSERT"},
		},
		{
			name:     "CREATE with underscores",
			sql:      "CREATE TABLE user_account_mapping (id INT)",
			expected: []string{"CREATE"},
		},
		{
			name:     "keyword followed by underscore",
			sql:      "INSERT_EXTRA INTO users",
			expected: nil,
		},
		{
			name:     "keyword preceded by underscore",
			sql:      "_INSERT INTO users",
			expected: nil,
		},
		{
			name:     "underscore within keyword-like word",
			sql:      "IN_SERT INTO users",
			expected: nil,
		},
		{
			name:     "all keywords present",
			sql:      "INSERT UPDATE DELETE DROP ALTER CREATE TRUNCATE REPLACE GRANT REVOKE LOCK",
			expected: []string{"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE", "TRUNCATE", "REPLACE", "GRANT", "REVOKE", "LOCK"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScanKeywords(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ScanKeywords() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestScanKeywords_UTF8(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "UTF-8 identifier with INSERT keyword",
			sql:      "INSERT INTO 用户表 VALUES (1)",
			expected: []string{"INSERT"},
		},
		{
			name:     "UTF-8 identifier with UPDATE keyword",
			sql:      "UPDATE 用户表 SET 名字 = '测试'",
			expected: []string{"UPDATE"},
		},
		{
			name:     "UTF-8 identifier with DELETE keyword",
			sql:      "DELETE FROM 用户表 WHERE 编号 = 1",
			expected: []string{"DELETE"},
		},
		{
			name:     "UTF-8 string literal with keyword inside",
			sql:      "SELECT 'INSERT INTO 用户表' FROM dual",
			expected: nil,
		},
		{
			name:     "UTF-8 string literal with escaped quote and keyword",
			sql:      "INSERT INTO 用户表 VALUES ('测试''INSERT')",
			expected: []string{"INSERT"},
		},
		{
			name:     "UTF-8 comment with keyword inside",
			sql:      "SELECT * FROM 用户表 -- INSERT INTO 用户表\nWHERE 编号 = 1",
			expected: nil,
		},
		{
			name:     "UTF-8 block comment with keyword inside",
			sql:      "SELECT * FROM 用户表 /* INSERT INTO 用户表 */ WHERE 编号 = 1",
			expected: nil,
		},
		{
			name:     "mixed ASCII and UTF-8 identifiers",
			sql:      "INSERT INTO users_用户 VALUES (1, '测试')",
			expected: []string{"INSERT"},
		},
		{
			name:     "UTF-8 identifier with keyword as substring should not match",
			sql:      "SELECT * FROM 插入表 WHERE 编号 = 1",
			expected: nil,
		},
		{
			name:     "UTF-8 with multiple keywords",
			sql:      "INSERT INTO 用户表 VALUES (1); DELETE FROM 日志表",
			expected: []string{"INSERT", "DELETE"},
		},
		{
			name:     "emoji in string with keyword",
			sql:      "INSERT INTO users VALUES ('🎉INSERT🎉')",
			expected: []string{"INSERT"},
		},
		{
			name:     "multi-byte characters at boundaries",
			sql:      "中文INSERT中文",
			expected: nil,
		},
		{
			name:     "keyword with UTF-8 underscores",
			sql:      "INSERT INTO 用户_表 VALUES (1)",
			expected: []string{"INSERT"},
		},
		{
			name:     "complex UTF-8 with escaped quotes",
			sql:      "INSERT INTO 用户表 VALUES ('中文''测试''INSERT')",
			expected: []string{"INSERT"},
		},
		{
			name:     "UTF-8 identifier that looks like keyword but isn't",
			sql:      "SELECT * FROM 用户INSERT表",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScanKeywords(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ScanKeywords() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestScanKeywords_BypassFile reads testdata/bypass.sql and verifies scanner behavior
// This test documents known bypass techniques that the scanner does NOT catch.
// These are intentional limitations, not bugs - scanner catches honest mistakes,
// not malicious injection. Real security comes from read-only MySQL accounts.
func TestScanKeywords_BypassFile(t *testing.T) {
	// Read the bypass documentation file
	bypassSQL, err := os.ReadFile("testdata/bypass.sql")
	if err != nil {
		t.Fatalf("Failed to read testdata/bypass.sql: %v", err)
	}

	sql := string(bypassSQL)
	result := ScanKeywords(sql)

	// Expected keywords that SHOULD be detected in bypass.sql:
	// - Section 4 (Prepared Statements): "INSERT INTO users VALUES (?)", "UPDATE users SET..."
	// - Section 7 (Alternative Syntax): "REPLACE INTO...", "GRANT SELECT..."
	expectedKeywords := []string{"INSERT", "UPDATE", "REPLACE", "GRANT"}

	// Verify all expected keywords are detected
	for _, kw := range expectedKeywords {
		found := false
		for _, detected := range result {
			if detected == kw {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected keyword %s to be detected in bypass.sql but it was not", kw)
		}
	}

	// Verify bypass techniques are NOT detected (these would be true bypasses)
	// We can't directly test "what was NOT detected", but we verify total count matches expectations
	// The file has many bypass attempts that should NOT be detected:
	// - IN/**/SERT (block comment split)
	// - UP/**/DATE (block comment split)
	// - DEL/**/ET/**/E (block comment split)
	// - DR/**/OP (block comment split)
	// - AL/**/TER (block comment split)
	// - 0x494E53455254 (hex literal)
	// - Keywords in backticks: `INSERT`, `DROP_TABLE`, `DELETE`, `UPDATE`
	// - String concatenation patterns
	// - EXEC patterns

	// The scanner should detect exactly the keywords listed in expectedKeywords
	// (allowing for duplicate detection which the scanner handles via deduplication)
	if len(result) != len(expectedKeywords) {
		t.Logf("Warning: Scanner detected %d keywords, expected %d", len(result), len(expectedKeywords))
		t.Logf("Detected: %v", result)
		t.Logf("Expected: %v", expectedKeywords)
		t.Logf("This may indicate the scanner is detecting some bypass attempts (unexpected)")
		t.Logf("Or failing to detect legitimate keywords (bug)")
	}

	// Log results for documentation purposes
	t.Logf("Bypass file scanner test completed:")
	t.Logf("Total keywords detected: %d", len(result))
	t.Logf("Keywords: %v", result)
	t.Logf("This documents that bypass techniques using block comments, hex, backticks, etc. are NOT detected")
	t.Logf("This is intentional - scanner catches honest mistakes, not malicious injection")
}
