// Package mysqldrv provides MySQL database driver functionality.
// It handles driver registration and connection management for MySQL databases.
package mysqldrv

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql" // Blank import for driver registration
)

// Open creates and validates a connection to a MySQL database.
// It registers the MySQL driver (if not already registered) and establishes
// a connection to the database specified by the DSN (Data Source Name).
//
// The DSN format: "username:password@protocol(address)/dbname?param=value"
// Example: "user:password@tcp(localhost:3306)/mydb?parseTime=true"
//
// The function:
// 1. Opens a database connection using the mysql driver
// 2. Validates the connection with a ping
// 3. Returns the *sql.DB for caller use
//
// Returns an error if the connection cannot be established or validated.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	// Open database connection
	// The mysql driver auto-registers on import, so we can use "mysql" directly
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Validate the connection with a ping
	if err := db.PingContext(ctx); err != nil {
		db.Close() // Close the connection if ping fails
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}