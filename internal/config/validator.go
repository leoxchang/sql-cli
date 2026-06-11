package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/qiezi999/sql-cli/internal/output"
)

// Validation errors for MySQL URL DSNs
var (
	ErrInvalidURLFormat    = errors.New("invalid URL format")
	ErrInvalidScheme       = errors.New("scheme must be 'mysql'")
	ErrEmptyHost           = errors.New("host cannot be empty")
	ErrMissingPort         = errors.New("port is required (host:port format)")
	ErrInvalidPortRange    = errors.New("port must be between 0 and 65535")
	ErrDSNContainsNewline  = errors.New("DSN contains forbidden newline character")
	ErrDSNContainsCarriage = errors.New("DSN contains forbidden carriage return character")
)

// ValidateMySQLURL validates a mysql:// URL DSN according to D4 specification.
// It checks:
//   - Parses as valid URL
//   - Scheme is exactly "mysql" (not mysqls, http, etc.)
//   - Host is non-empty
//   - Port segment is present (host:port format)
//   - Port is in valid range 0-65535
//   - No \n or \r characters anywhere in DSN (DSN smuggling prevention)
//
// The database path is optional per design.md D4. The format shown in D4
// (mysql://user:pass@host:port/db) is illustrative, not a strict requirement.
// This allows connections without a default database, which is useful for
// administrative queries like SHOW DATABASES.
//
// Returns CONFIG_ERROR for any validation failure.
func ValidateMySQLURL(dsn string) error {
	// Check for forbidden characters first (DSN smuggling prevention)
	// Check for \r first since it's less common in bypass attempts
	if strings.Contains(dsn, "\r") {
		return ErrDSNContainsCarriage
	}
	if strings.Contains(dsn, "\n") {
		return ErrDSNContainsNewline
	}

	// Parse as URL
	parsed, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURLFormat, err)
	}

	// Validate scheme is exactly "mysql"
	if parsed.Scheme != "mysql" {
		return fmt.Errorf("%w: got %q", ErrInvalidScheme, parsed.Scheme)
	}

	// Validate host is non-empty
	if parsed.Hostname() == "" {
		return ErrEmptyHost
	}

	// Validate port is present
	portStr := parsed.Port()
	if portStr == "" {
		return ErrMissingPort
	}

	// Validate port is in valid range 0-65535
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("%w: invalid port number %q", ErrInvalidPortRange, portStr)
	}
	if port < 0 || port > 65535 {
		return fmt.Errorf("%w: port %d out of range", ErrInvalidPortRange, port)
	}

	return nil
}

// MySQLURLToDriverDSN translates a mysql:// URL to go-sql-driver/mysql DSN format.
// Input: mysql://user:pass@host:port/db?param=value
// Output: user:pass@tcp(host:port)/db?param=value
//
// Returns CONFIG_ERROR for any validation or translation failure.
func MySQLURLToDriverDSN(dsn string) (string, error) {
	// Validate first
	if err := ValidateMySQLURL(dsn); err != nil {
		return "", err
	}

	// Parse (we know it's valid from validation above)
	parsed, _ := url.Parse(dsn)

	// Extract components
	user := parsed.User.Username()
	pass, hasPass := parsed.User.Password()
	host := parsed.Hostname()
	port := parsed.Port()
	path := parsed.Path
	query := parsed.RawQuery

	// Build driver DSN
	// Format: user:pass@tcp(host:port)/db?params
	var driverDSN strings.Builder

	// Add user:pass@
	if hasPass {
		driverDSN.WriteString(fmt.Sprintf("%s:%s@", user, pass))
	} else {
		driverDSN.WriteString(fmt.Sprintf("%s@", user))
	}

	// Add tcp(host:port)
	driverDSN.WriteString(fmt.Sprintf("tcp(%s:%s)", host, port))

	// Add path (database)
	driverDSN.WriteString(path)

	// Add query parameters if present
	if query != "" {
		driverDSN.WriteString("?")
		driverDSN.WriteString(query)
	}

	return driverDSN.String(), nil
}

// ValidateMySQLURLOrError validates a mysql:// URL or returns a CONFIG_ERROR envelope.
// This is a convenience function for CLI handlers that need to emit JSON errors.
func ValidateMySQLURLOrError(dsn string) *output.Envelope {
	if err := ValidateMySQLURL(dsn); err != nil {
		return output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			err.Error(),
			nil,
		)
	}
	return nil
}

// MySQLURLToDriverDSNOrError translates a mysql:// URL to driver DSN or returns a CONFIG_ERROR envelope.
// This is a convenience function for CLI handlers that need to emit JSON errors.
func MySQLURLToDriverDSNOrError(dsn string) (string, *output.Envelope) {
	driverDSN, err := MySQLURLToDriverDSN(dsn)
	if err != nil {
		return "", output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			err.Error(),
			nil,
		)
	}
	return driverDSN, nil
}

// ExtractDatabaseFromURL extracts the database name from a mysql:// URL.
// Returns empty string if no database is specified in the URL.
// Returns error if the URL is invalid.
func ExtractDatabaseFromURL(dsn string) (string, error) {
	// Validate first
	if err := ValidateMySQLURL(dsn); err != nil {
		return "", err
	}

	// Parse (we know it's valid from validation above)
	parsed, _ := url.Parse(dsn)

	// Extract database from path, stripping leading slash
	path := strings.TrimPrefix(parsed.Path, "/")
	return path, nil
}
