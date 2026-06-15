package command

import (
	"strings"
	"testing"
)

func TestProbeSQL(t *testing.T) {
	if !strings.Contains(probeSQL, "INFORMATION_SCHEMA.COLUMNS") {
		t.Errorf("probeSQL missing INFORMATION_SCHEMA.COLUMNS: %s", probeSQL)
	}
	if !strings.Contains(probeSQL, "'is_visible'") || !strings.Contains(probeSQL, "'expression'") {
		t.Errorf("probeSQL missing column name literals: %s", probeSQL)
	}
}

func TestBuildIndexSQL_FullPath(t *testing.T) {
	main := buildIndexSQL(true, true)

	// Full path: 14 columns including IS_VISIBLE and EXPRESSION
	if !strings.Contains(main, "IS_VISIBLE") {
		t.Errorf("full path SQL missing IS_VISIBLE: %s", main)
	}
	if !strings.Contains(main, "EXPRESSION") {
		t.Errorf("full path SQL missing EXPRESSION: %s", main)
	}
	if strings.Contains(main, "'' AS") {
		t.Errorf("full path SQL should not have '' AS projection: %s", main)
	}
	assertMainQueryStructure(t, main)
}

func TestBuildIndexSQL_OnlyIsVisible(t *testing.T) {
	main := buildIndexSQL(true, false)

	// 8.0.0-8.0.12: IS_VISIBLE real, EXPRESSION faked
	if !strings.Contains(main, "IS_VISIBLE") {
		t.Errorf("should contain real IS_VISIBLE: %s", main)
	}
	if !strings.Contains(main, "'' AS EXPRESSION") {
		t.Errorf("should contain '' AS EXPRESSION: %s", main)
	}
	assertMainQueryStructure(t, main)
}

func TestBuildIndexSQL_NeitherColumn(t *testing.T) {
	main := buildIndexSQL(false, false)

	// 5.7: both faked
	if !strings.Contains(main, "'' AS IS_VISIBLE") {
		t.Errorf("should contain '' AS IS_VISIBLE: %s", main)
	}
	if !strings.Contains(main, "'' AS EXPRESSION") {
		t.Errorf("should contain '' AS EXPRESSION: %s", main)
	}
	assertMainQueryStructure(t, main)
}

// assertMainQueryStructure checks common structure shared by all 3 SQL variants.
func assertMainQueryStructure(t *testing.T, sql string) {
	t.Helper()
	if !strings.Contains(sql, "INFORMATION_SCHEMA.STATISTICS") {
		t.Errorf("missing INFORMATION_SCHEMA.STATISTICS: %s", sql)
	}
	if !strings.Contains(sql, "TABLE_SCHEMA = ?") {
		t.Errorf("missing TABLE_SCHEMA = ?: %s", sql)
	}
	if !strings.Contains(sql, "TABLE_NAME = ?") {
		t.Errorf("missing TABLE_NAME = ?: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY INDEX_NAME, SEQ_IN_INDEX") {
		t.Errorf("missing ORDER BY: %s", sql)
	}

	// Verify all 14 column names are present in the SELECT
	expectedColumns := []string{
		"INDEX_NAME", "NON_UNIQUE", "SEQ_IN_INDEX", "COLUMN_NAME",
		"COLLATION", "CARDINALITY", "SUB_PART", "PACKED",
		"NULLABLE", "INDEX_TYPE", "COMMENT", "INDEX_COMMENT",
		"IS_VISIBLE", "EXPRESSION",
	}
	for _, col := range expectedColumns {
		if !strings.Contains(sql, col) {
			t.Errorf("missing column %s in SQL: %s", col, sql)
		}
	}
}

func TestParseIndexesArgs_DotSeparated(t *testing.T) {
	db, table, err := parseIndexesArgs("mydb.users", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db != "mydb" {
		t.Errorf("expected database 'mydb', got %q", db)
	}
	if table != "users" {
		t.Errorf("expected table 'users', got %q", table)
	}
}

func TestParseIndexesArgs_TableOnlyWithDSN(t *testing.T) {
	db, table, err := parseIndexesArgs("users", "mysql://u:p@h:3306/mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db != "mydb" {
		t.Errorf("expected database 'mydb', got %q", db)
	}
	if table != "users" {
		t.Errorf("expected table 'users', got %q", table)
	}
}

func TestParseIndexesArgs_TableOnlyNoDSN(t *testing.T) {
	_, _, err := parseIndexesArgs("users", "mysql://u:p@h:3306/")
	if err == nil {
		t.Fatal("expected error for single-segment with no DSN database, got nil")
	}
}

func TestParseIndexesArgs_RejectedInputs(t *testing.T) {
	cases := []struct {
		name string
		arg  string
	}{
		{"empty string", ""},
		{"three segments", "a.b.c"},
		{"dot only", "."},
		{"trailing dot", "a."},
		{"leading dot", ".a"},
		{"double dot", "a..b"},
		{"injection attempt", "mydb.users; DROP TABLE x"},
		{"backtick injection", "`mydb`.`users`"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseIndexesArgs(tc.arg, "mysql://u:p@h:3306/mydb")
			if err == nil {
				t.Errorf("expected error for input %q, got nil", tc.arg)
			}
		})
	}
}