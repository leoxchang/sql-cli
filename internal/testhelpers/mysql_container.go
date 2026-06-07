//go:build integration

package testhelpers

import (
	"context"
	"fmt"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

// MySQLTestContainer wraps the testcontainers MySQLContainer to provide
// a test-friendly ConnectionString method that doesn't require context.
type MySQLTestContainer struct {
	*mysql.MySQLContainer
	ctx context.Context
}

// ConnectionString returns the MySQL connection string for the container.
// This is a convenience method for tests that don't want to manage context.
// It calls the underlying ConnectionString method with the stored context.
//
// Returns a DSN in the format: test:test@localhost:3306/testdb
// Compatible with github.com/go-sql-driver/mysql
func (c *MySQLTestContainer) ConnectionString(t *testing.T) string {
	t.Helper()

	connStr, err := c.MySQLContainer.ConnectionString(c.ctx)
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	return connStr
}

// Terminate stops and removes the MySQL container.
// This is a convenience method that uses the stored context.
func (c *MySQLTestContainer) Terminate(t *testing.T) {
	t.Helper()

	if err := c.MySQLContainer.Terminate(c.ctx); err != nil {
		t.Fatalf("Failed to terminate container: %v", err)
	}
}

// SetupMySQLContainer creates a MySQL 8 test container for integration tests.
// It returns a MySQLTestContainer instance that provides convenient test methods.
// The container should be cleaned up with defer mysqlC.Terminate(t).
//
// Usage:
//
//	mysqlC := testhelpers.SetupMySQLContainer(t)
//	defer mysqlC.Terminate(t)
//
//	db, err := sql.Open("mysql", mysqlC.ConnectionString(t))
func SetupMySQLContainer(t *testing.T) *MySQLTestContainer {
	t.Helper()

	ctx := context.Background()

	// Create MySQL 8 container with test configuration
	mysqlC, err := mysql.Run(ctx,
		"mysql:8.0",
		mysql.WithUsername("test"),
		mysql.WithPassword("test"),
		mysql.WithDatabase("testdb"),
	)
	if err != nil {
		t.Fatalf("Failed to create MySQL container: %v", err)
	}

	return &MySQLTestContainer{
		MySQLContainer: mysqlC,
		ctx:            ctx,
	}
}

// SetupMySQLContainerWithDSN creates a MySQL 8 test container and returns both
// the container and a mysql:// DSN format compatible with our config package.
// This is useful when testing the full CLI stack with DSN resolution.
//
// Returns:
//   - container: the MySQLTestContainer instance for cleanup
//   - dsn: mysql:// URL format (e.g., mysql://test:test@localhost:3306/testdb)
//   - teardown: function to call for cleanup (equivalent to container.Terminate)
func SetupMySQLContainerWithDSN(ctx context.Context) (container *MySQLTestContainer, dsn string, teardown func(), err error) {
	// Create MySQL 8 container with test configuration
	mysqlC, err := mysql.Run(ctx,
		"mysql:8.0",
		mysql.WithUsername("test"),
		mysql.WithPassword("test"),
		mysql.WithDatabase("testdb"),
	)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to create MySQL container: %w", err)
	}

	// Get connection string from container
	connStr, err := mysqlC.ConnectionString(ctx)
	if err != nil {
		// Clean up container if we can't get connection string
		_ = mysqlC.Terminate(ctx)
		return nil, "", nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	// Convert DSN to mysql:// URL format
	// The testcontainers mysql module returns: test:test@localhost:3306/testdb
	// We need: mysql://test:test@localhost:3306/testdb
	dsn = "mysql://" + connStr

	// Wrap container for convenience
	wrapped := &MySQLTestContainer{
		MySQLContainer: mysqlC,
		ctx:            ctx,
	}

	// Create teardown function
	teardown = func() {
		if err := mysqlC.Terminate(ctx); err != nil {
			// Log error but don't panic in teardown
			fmt.Printf("Warning: failed to terminate MySQL container: %v\n", err)
		}
	}

	return wrapped, dsn, teardown, nil
}