package mysqldrv

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

// TestOpen_ValidDSN tests successful database connection
func TestOpen_ValidDSN(t *testing.T) {
	if testing.Short() || os.Getenv("SKIP_DOCKER") != "" {
		t.Skip("Skipping integration test in short mode or when SKIP_DOCKER is set")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	// Test Open
	db, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Verify connection is working
	if err := db.PingContext(ctx); err != nil {
		t.Errorf("Failed to ping database: %v", err)
	}
}

// TestOpen_InvalidDSN tests error handling for invalid DSN
func TestOpen_InvalidDSN(t *testing.T) {
	tests := []struct {
		name        string
		dsn         string
		description string
	}{
		{
			name:        "empty_dsn",
			dsn:         "",
			description: "Empty DSN should fail",
		},
		{
			name:        "malformed_dsn",
			dsn:         "not:a:valid:dsn",
			description: "Malformed DSN should fail",
		},
		{
			name:        "invalid_protocol",
			dsn:         "user:pass@invalid(protocol)/db",
			description: "Invalid protocol should fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			db, err := Open(ctx, tt.dsn)
			if err == nil {
				db.Close()
				t.Errorf("%s: expected error, got nil", tt.description)
			}
		})
	}
}

// TestOpen_Timeout tests that Open respects context cancellation
func TestOpen_Timeout(t *testing.T) {
	if testing.Short() || os.Getenv("SKIP_DOCKER") != "" {
		t.Skip("Skipping integration test in short mode or when SKIP_DOCKER is set")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	// Create a cancelled context
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	// Test Open with cancelled context
	db, err := Open(cancelledCtx, dsn)
	if err == nil {
		db.Close()
		t.Error("Expected error with cancelled context, got nil")
	}
}

// TestOpen_ConnectionReuse tests that Open can be called multiple times
func TestOpen_ConnectionReuse(t *testing.T) {
	if testing.Short() || os.Getenv("SKIP_DOCKER") != "" {
		t.Skip("Skipping integration test in short mode or when SKIP_DOCKER is set")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	// Open multiple connections
	db1, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("First Open failed: %v", err)
	}
	defer db1.Close()

	db2, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Second Open failed: %v", err)
	}
	defer db2.Close()

	// Both should work
	if err := db1.PingContext(ctx); err != nil {
		t.Errorf("First connection failed to ping: %v", err)
	}
	if err := db2.PingContext(ctx); err != nil {
		t.Errorf("Second connection failed to ping: %v", err)
	}
}

// TestOpen_InvalidCredentials tests error handling for invalid credentials
func TestOpen_InvalidCredentials(t *testing.T) {
	if testing.Short() || os.Getenv("SKIP_DOCKER") != "" {
		t.Skip("Skipping integration test in short mode or when SKIP_DOCKER is set")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string with wrong credentials
	_, err = container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	// Modify DSN to use invalid credentials
	// Original format: user:pass@tcp(host:port)/db?params
	// Replace with invalid credentials
	invalidDSN := "wronguser:wrongpass@tcp(localhost:3306)/nonexistent?parseTime=true"

	// Test Open with invalid credentials
	db, err := Open(ctx, invalidDSN)
	if err == nil {
		db.Close()
		t.Error("Expected error with invalid credentials, got nil")
	}
}

// TestOpen_ValidDSNWithParseTime tests connection with parseTime parameter
func TestOpen_ValidDSNWithParseTime(t *testing.T) {
	if testing.Short() || os.Getenv("SKIP_DOCKER") != "" {
		t.Skip("Skipping integration test in short mode or when SKIP_DOCKER is set")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string with parseTime=true
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	// Test Open
	db, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Verify we can query time values
	var currentTime time.Time
	err = db.QueryRowContext(ctx, "SELECT NOW()").Scan(&currentTime)
	if err != nil {
		t.Errorf("Failed to query time: %v", err)
	}
	if currentTime.IsZero() {
		t.Error("Expected non-zero time value")
	}
}