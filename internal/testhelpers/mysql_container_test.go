//go:build integration

package testhelpers_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/qiezi999/sql-cli/internal/testhelpers"
)

// TestSetupMySQLContainer verifies the helper creates a working MySQL container.
// This test requires Docker to be running.
func TestSetupMySQLContainer(t *testing.T) {
	if testing.Short() || os.Getenv("SKIP_DOCKER") != "" {
		t.Skip("Skipping integration test in short mode or when SKIP_DOCKER is set")
	}

	mysqlC := testhelpers.SetupMySQLContainer(t)
	defer mysqlC.Terminate(t)

	// Verify connection string is valid
	connStr := mysqlC.ConnectionString(t)
	if connStr == "" {
		t.Fatal("ConnectionString returned empty string")
	}

	// Verify we can connect to the database
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Verify connection is working
	err = db.Ping()
	if err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	// Verify we can execute a query
	var version string
	err = db.QueryRow("SELECT VERSION()").Scan(&version)
	if err != nil {
		t.Fatalf("Failed to query version: %v", err)
	}

	// Verify it's MySQL 8
	if !strings.Contains(version, "8.0") {
		t.Errorf("Expected MySQL 8.0, got version: %s", version)
	}

	t.Logf("Connected to MySQL version: %s", version)
}

// TestSetupMySQLContainerWithDSN verifies the DSN-returning helper.
func TestSetupMySQLContainerWithDSN(t *testing.T) {
	if testing.Short() || os.Getenv("SKIP_DOCKER") != "" {
		t.Skip("Skipping integration test in short mode or when SKIP_DOCKER is set")
	}

	ctx := context.Background()

	container, dsn, teardown, err := testhelpers.SetupMySQLContainerWithDSN(ctx)
	if err != nil {
		t.Fatalf("Failed to setup container: %v", err)
	}
	defer teardown()

	// Verify DSN format
	if dsn == "" {
		t.Fatal("DSN is empty")
	}

	if !strings.HasPrefix(dsn, "mysql://") {
		t.Errorf("DSN should start with 'mysql://', got: %s", dsn)
	}

	// Verify we can connect using the DSN
	db, err := sql.Open("mysql", dsn[8:]) // Remove mysql:// prefix for driver
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	// Verify container is returned
	if container == nil {
		t.Fatal("Container should not be nil")
	}

	t.Logf("Successfully created container with DSN: %s", dsn)
}

// TestSetupMySQLContainer_DatabaseCreation verifies the testdb database exists.
func TestSetupMySQLContainer_DatabaseCreation(t *testing.T) {
	if testing.Short() || os.Getenv("SKIP_DOCKER") != "" {
		t.Skip("Skipping integration test in short mode or when SKIP_DOCKER is set")
	}

	mysqlC := testhelpers.SetupMySQLContainer(t)
	defer mysqlC.Terminate(t)

	db, err := sql.Open("mysql", mysqlC.ConnectionString(t))
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Verify testdb database exists
	var dbName string
	err = db.QueryRow("SELECT DATABASE()").Scan(&dbName)
	if err != nil {
		t.Fatalf("Failed to query database name: %v", err)
	}

	if dbName != "testdb" {
		t.Errorf("Expected database 'testdb', got '%s'", dbName)
	}

	// Verify user is 'test'
	var user string
	err = db.QueryRow("SELECT CURRENT_USER()").Scan(&user)
	if err != nil {
		t.Fatalf("Failed to query user: %v", err)
	}

	if !strings.HasPrefix(user, "test@") {
		t.Errorf("Expected user 'test@...', got '%s'", user)
	}
}