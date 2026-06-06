//go:build integration

package output_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/qiezi999/sql-cli/internal/output"
	"github.com/qiezi999/sql-cli/internal/testhelpers"
)

func TestConvertRows_MySQLTypes(t *testing.T) {
	// Start MySQL container
	mysqlC := testhelpers.SetupMySQLContainer(t)
	defer mysqlC.Terminate(t)

	// Connect to database
	db, err := sql.Open("mysql", mysqlC.ConnectionString(t))
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create test table with various MySQL types
	_, err = db.Exec(`
		CREATE TABLE type_test (
			id INT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(100),
			age INT,
			salary DECIMAL(10,2),
			active BOOLEAN,
			created_at TIMESTAMP,
			data BLOB,
			description TEXT,
			score FLOAT,
			big_num BIGINT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data
	testTime := time.Date(2024, 6, 15, 14, 30, 45, 0, time.UTC)
	_, err = db.Exec(`
		INSERT INTO type_test (name, age, salary, active, created_at, data, description, score, big_num)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "Alice", 30, 75000.50, true, testTime, []byte("binary data"), "A description", 95.5, int64(1234567890123))
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	// Query the data
	rows, err := db.Query("SELECT id, name, age, salary, active, created_at, data, description, score, big_num FROM type_test")
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	defer rows.Close()

	// Convert rows
	columns, resultRows, err := output.ConvertRows(rows)
	if err != nil {
		t.Fatalf("Failed to convert rows: %v", err)
	}

	// Verify column count
	if len(columns) != 10 {
		t.Errorf("Expected 10 columns, got %d", len(columns))
	}

	// Verify column names
	expectedNames := []string{"id", "name", "age", "salary", "active", "created_at", "data", "description", "score", "big_num"}
	for i, col := range columns {
		if col.Name != expectedNames[i] {
			t.Errorf("Column %d: expected name %s, got %s", i, expectedNames[i], col.Name)
		}
	}

	// Verify column type case preservation (D12)
	// MySQL returns uppercase for SELECT statements
	expectedTypes := []string{"INT", "VARCHAR", "INT", "DECIMAL", "TINY", "TIMESTAMP", "BLOB", "TEXT", "FLOAT", "BIGINT"}
	for i, col := range columns {
		if col.Type != expectedTypes[i] {
			t.Errorf("Column %d (%s): expected type %s, got %s", i, col.Name, expectedTypes[i], col.Type)
		}
	}

	// Verify row count
	if len(resultRows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(resultRows))
	}

	// Verify row data
	row := resultRows[0]
	if len(row) != 10 {
		t.Errorf("Expected 10 values in row, got %d", len(row))
	}

	// Verify specific values
	// id (INT)
	if id, ok := row[0].(int64); !ok || id != 1 {
		t.Errorf("Expected id=1 (int64), got %v (%T)", row[0], row[0])
	}

	// name (VARCHAR -> string)
	if name, ok := row[1].(string); !ok || name != "Alice" {
		t.Errorf("Expected name='Alice' (string), got %v (%T)", row[1], row[1])
	}

	// age (INT)
	if age, ok := row[2].(int64); !ok || age != 30 {
		t.Errorf("Expected age=30 (int64), got %v (%T)", row[2], row[2])
	}

	// salary (DECIMAL -> float64)
	if salary, ok := row[3].(float64); !ok || salary != 75000.50 {
		t.Errorf("Expected salary=75000.50 (float64), got %v (%T)", row[3], row[3])
	}

	// active (BOOLEAN)
	if active, ok := row[4].(bool); !ok || !active {
		t.Errorf("Expected active=true (bool), got %v (%T)", row[4], row[4])
	}

	// created_at (TIMESTAMP -> RFC3339 string)
	if createdAt, ok := row[5].(string); !ok {
		t.Errorf("Expected created_at to be string, got %v (%T)", row[5], row[5])
	} else {
		// Parse the time to verify format
		parsed, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			t.Errorf("Failed to parse created_at as RFC3339: %v", err)
		}
		if !parsed.Equal(testTime) {
			t.Errorf("Expected created_at=%v, got %v", testTime, parsed)
		}
	}

	// data (BLOB -> string)
	if data, ok := row[6].(string); !ok || data != "binary data" {
		t.Errorf("Expected data='binary data' (string), got %v (%T)", row[6], row[6])
	}

	// description (TEXT -> string)
	if desc, ok := row[7].(string); !ok || desc != "A description" {
		t.Errorf("Expected description='A description' (string), got %v (%T)", row[7], row[7])
	}

	// score (FLOAT)
	if score, ok := row[8].(float64); !ok || score != 95.5 {
		t.Errorf("Expected score=95.5 (float64), got %v (%T)", row[8], row[8])
	}

	// big_num (BIGINT)
	if bigNum, ok := row[9].(int64); !ok || bigNum != 1234567890123 {
		t.Errorf("Expected big_num=1234567890123 (int64), got %v (%T)", row[9], row[9])
	}
}

func TestConvertRows_NullValues(t *testing.T) {
	mysqlC := testhelpers.SetupMySQLContainer(t)
	defer mysqlC.Terminate(t)

	db, err := sql.Open("mysql", mysqlC.ConnectionString(t))
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create table with nullable columns
	_, err = db.Exec(`
		CREATE TABLE null_test (
			id INT PRIMARY KEY,
			name VARCHAR(100),
			age INT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert row with NULL values
	_, err = db.Exec(`INSERT INTO null_test (id, name, age) VALUES (1, NULL, NULL)`)
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	// Query
	rows, err := db.Query("SELECT id, name, age FROM null_test")
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	defer rows.Close()

	columns, resultRows, err := output.ConvertRows(rows)
	if err != nil {
		t.Fatalf("Failed to convert rows: %v", err)
	}

	// Verify
	if len(resultRows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(resultRows))
	}

	row := resultRows[0]

	// id should have value
	if row[0] == nil {
		t.Error("Expected id to have value, got nil")
	}

	// name should be nil (NULL)
	if row[1] != nil {
		t.Errorf("Expected name to be nil (NULL), got %v", row[1])
	}

	// age should be nil (NULL)
	if row[2] != nil {
		t.Errorf("Expected age to be nil (NULL), got %v", row[2])
	}

	// Verify column types preserve case
	if len(columns) != 3 {
		t.Errorf("Expected 3 columns, got %d", len(columns))
	}

	// MySQL returns INT in uppercase for SELECT
	if columns[0].Type != "INT" {
		t.Errorf("Expected 'INT', got %s", columns[0].Type)
	}
	if columns[1].Type != "VARCHAR" {
		t.Errorf("Expected 'VARCHAR', got %s", columns[1].Type)
	}
	if columns[2].Type != "INT" {
		t.Errorf("Expected 'INT', got %s", columns[2].Type)
	}
}

func TestConvertRows_EmptyResult(t *testing.T) {
	mysqlC := testhelpers.SetupMySQLContainer(t)
	defer mysqlC.Terminate(t)

	db, err := sql.Open("mysql", mysqlC.ConnectionString(t))
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create empty table
	_, err = db.Exec(`CREATE TABLE empty_test (id INT, name VARCHAR(100))`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Query empty table
	rows, err := db.Query("SELECT id, name FROM empty_test")
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	defer rows.Close()

	columns, resultRows, err := output.ConvertRows(rows)
	if err != nil {
		t.Fatalf("Failed to convert rows: %v", err)
	}

	// Should have columns but no rows
	if len(columns) != 2 {
		t.Errorf("Expected 2 columns, got %d", len(columns))
	}
	if len(resultRows) != 0 {
		t.Errorf("Expected 0 rows, got %d", len(resultRows))
	}
}

func TestConvertRows_MultipleRows(t *testing.T) {
	mysqlC := testhelpers.SetupMySQLContainer(t)
	defer mysqlC.Terminate(t)

	db, err := sql.Open("mysql", mysqlC.ConnectionString(t))
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create and populate table
	_, err = db.Exec(`
		CREATE TABLE multi_test (id INT, name VARCHAR(50));
		INSERT INTO multi_test VALUES (1, 'Alice'), (2, 'Bob'), (3, 'Charlie')
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	rows, err := db.Query("SELECT id, name FROM multi_test ORDER BY id")
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	defer rows.Close()

	columns, resultRows, err := output.ConvertRows(rows)
	if err != nil {
		t.Fatalf("Failed to convert rows: %v", err)
	}

	// Verify row count
	if len(resultRows) != 3 {
		t.Errorf("Expected 3 rows, got %d", len(resultRows))
	}

	// Verify first row
	if id, ok := resultRows[0][0].(int64); !ok || id != 1 {
		t.Errorf("Row 0: Expected id=1, got %v", resultRows[0][0])
	}
	if name, ok := resultRows[0][1].(string); !ok || name != "Alice" {
		t.Errorf("Row 0: Expected name='Alice', got %v", resultRows[0][1])
	}

	// Verify second row
	if id, ok := resultRows[1][0].(int64); !ok || id != 2 {
		t.Errorf("Row 1: Expected id=2, got %v", resultRows[1][0])
	}

	// Verify third row
	if id, ok := resultRows[2][0].(int64); !ok || id != 3 {
		t.Errorf("Row 2: Expected id=3, got %v", resultRows[2][0])
	}

	// Verify columns
	if len(columns) != 2 {
		t.Errorf("Expected 2 columns, got %d", len(columns))
	}
	if columns[0].Name != "id" || columns[1].Name != "name" {
		t.Errorf("Column names incorrect: %v", columns)
	}
}