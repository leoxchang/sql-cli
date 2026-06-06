package config

import (
	"strings"
	"testing"
)

func TestValidateMySQLURL_ValidCompleteURL(t *testing.T) {
	dsn := "mysql://user:pass@localhost:3306/mydb?charset=utf8mb4"
	err := ValidateMySQLURL(dsn)
	if err != nil {
		t.Errorf("Expected valid URL to pass, got error: %v", err)
	}
}

func TestValidateMySQLURL_ValidMinimalURL(t *testing.T) {
	dsn := "mysql://user@localhost:3306/mydb"
	err := ValidateMySQLURL(dsn)
	if err != nil {
		t.Errorf("Expected valid minimal URL to pass, got error: %v", err)
	}
}

func TestValidateMySQLURL_ValidWithQuery(t *testing.T) {
	dsn := "mysql://user:pass@db.example.com:3306/testdb?parseTime=true&loc=UTC"
	err := ValidateMySQLURL(dsn)
	if err != nil {
		t.Errorf("Expected valid URL with query params to pass, got error: %v", err)
	}
}

func TestValidateMySQLURL_InvalidScheme(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"http scheme", "http://user:pass@localhost:3306/mydb"},
		{"https scheme", "https://user:pass@localhost:3306/mydb"},
		{"mysqls scheme", "mysqls://user:pass@localhost:3306/mydb"},
		{"empty scheme", "//user:pass@localhost:3306/mydb"},
		{"no scheme", "user:pass@localhost:3306/mydb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMySQLURL(tt.dsn)
			if err == nil {
				t.Errorf("Expected error for %s scheme, got nil", tt.name)
			}
			if err != nil && !strings.Contains(err.Error(), "scheme") {
				t.Errorf("Expected scheme error, got: %v", err)
			}
		})
	}
}

func TestValidateMySQLURL_EmptyHost(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"no host", "mysql://user:pass@:3306/mydb"},
		{"empty host", "mysql://@:3306/mydb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMySQLURL(tt.dsn)
			if err == nil {
				t.Errorf("Expected error for empty host, got nil")
			}
			if err != nil && err != ErrEmptyHost {
				t.Errorf("Expected ErrEmptyHost, got: %v", err)
			}
		})
	}
}

func TestValidateMySQLURL_MissingPort(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"no port segment", "mysql://user:pass@localhost/mydb"},
		{"empty port", "mysql://user:pass@localhost:/mydb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMySQLURL(tt.dsn)
			if err == nil {
				t.Errorf("Expected error for missing port, got nil")
			}
			if err != nil && err != ErrMissingPort {
				t.Errorf("Expected ErrMissingPort, got: %v", err)
			}
		})
	}
}

func TestValidateMySQLURL_NewlineCharacter(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"newline at start", "\nmysql://user:pass@localhost:3306/mydb"},
		{"newline in middle", "mysql://user:pass@local\nhost:3306/mydb"},
		{"newline at end", "mysql://user:pass@localhost:3306/mydb\n"},
		{"newline in password", "mysql://user:pass\nword@localhost:3306/mydb"},
		{"newline in query", "mysql://user:pass@localhost:3306/mydb?param=val\nue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMySQLURL(tt.dsn)
			if err == nil {
				t.Errorf("Expected error for newline character, got nil")
			}
			if err != nil && err != ErrDSNContainsNewline {
				t.Errorf("Expected ErrDSNContainsNewline, got: %v", err)
			}
		})
	}
}

func TestValidateMySQLURL_CarriageReturnCharacter(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"CR at start", "\rmysql://user:pass@localhost:3306/mydb"},
		{"CR in middle", "mysql://user:pass@local\rost:3306/mydb"},
		{"CR at end", "mysql://user:pass@localhost:3306/mydb\r"},
		{"CR in password", "mysql://user:pass\rword@localhost:3306/mydb"},
		{"CRLF sequence", "mysql://user:pass@localhost:3306/mydb\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMySQLURL(tt.dsn)
			if err == nil {
				t.Errorf("Expected error for carriage return character, got nil")
			}
			if err != nil && err != ErrDSNContainsCarriage {
				t.Errorf("Expected ErrDSNContainsCarriage, got: %v", err)
			}
		})
	}
}

func TestValidateMySQLURL_InvalidURLFormat(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"invalid characters", "mysql://user:pass@local\\host:3306/mydb"},
		{"malformed URL", "mysql://:pass@:3306"},
		{"double slashes", "mysql:///user:pass@localhost:3306/mydb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMySQLURL(tt.dsn)
			if err == nil {
				t.Errorf("Expected error for invalid URL format, got nil")
			}
		})
	}
}

func TestMySQLURLToDriverDSN_CompleteURL(t *testing.T) {
	dsn := "mysql://myuser:mypass@db.example.com:3307/mydb?charset=utf8mb4&parseTime=true"
	expected := "myuser:mypass@tcp(db.example.com:3307)/mydb?charset=utf8mb4&parseTime=true"

	result, err := MySQLURLToDriverDSN(dsn)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestMySQLURLToDriverDSN_NoPassword(t *testing.T) {
	dsn := "mysql://myuser@localhost:3306/mydb"
	expected := "myuser@tcp(localhost:3306)/mydb"

	result, err := MySQLURLToDriverDSN(dsn)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestMySQLURLToDriverDSN_NoQueryParams(t *testing.T) {
	dsn := "mysql://user:pass@localhost:3306/mydb"
	expected := "user:pass@tcp(localhost:3306)/mydb"

	result, err := MySQLURLToDriverDSN(dsn)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestMySQLURLToDriverDSN_NoDatabase(t *testing.T) {
	dsn := "mysql://user:pass@localhost:3306"
	expected := "user:pass@tcp(localhost:3306)"

	result, err := MySQLURLToDriverDSN(dsn)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestMySQLURLToDriverDSN_SpecialCharacters(t *testing.T) {
	// URL-encoded special characters in password
	dsn := "mysql://user:pass%40word@localhost:3306/mydb"
	// The driver DSN should preserve URL encoding
	result, err := MySQLURLToDriverDSN(dsn)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	// Verify it contains tcp format
	if !strings.Contains(result, "tcp(localhost:3306)") {
		t.Errorf("Expected result to contain tcp format, got: %q", result)
	}
}

func TestMySQLURLToDriverDSN_InvalidURL(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"invalid scheme", "http://user:pass@localhost:3306/mydb"},
		{"missing port", "mysql://user:pass@localhost/mydb"},
		{"empty host", "mysql://user:pass@:3306/mydb"},
		{"newline character", "mysql://user:pass@local\nhost:3306/mydb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MySQLURLToDriverDSN(tt.dsn)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestMySQLURLToDriverDSN_ComplexQueryParams(t *testing.T) {
	dsn := "mysql://user:pass@db.example.com:3306/testdb?charset=utf8mb4&parseTime=true&loc=UTC&timeout=5s"
	result, err := MySQLURLToDriverDSN(dsn)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify all query params are preserved
	if !strings.Contains(result, "charset=utf8mb4") {
		t.Errorf("Expected charset param to be preserved, got: %q", result)
	}
	if !strings.Contains(result, "parseTime=true") {
		t.Errorf("Expected parseTime param to be preserved, got: %q", result)
	}
	if !strings.Contains(result, "loc=UTC") {
		t.Errorf("Expected loc param to be preserved, got: %q", result)
	}
	if !strings.Contains(result, "timeout=5s") {
		t.Errorf("Expected timeout param to be preserved, got: %q", result)
	}
}

func TestValidateMySQLURLOrError_Success(t *testing.T) {
	dsn := "mysql://user:pass@localhost:3306/mydb"
	envelope := ValidateMySQLURLOrError(dsn)
	if envelope != nil {
		t.Errorf("Expected nil envelope for valid URL, got: %v", envelope)
	}
}

func TestValidateMySQLURLOrError_InvalidURL(t *testing.T) {
	dsn := "http://user:pass@localhost:3306/mydb"
	envelope := ValidateMySQLURLOrError(dsn)
	if envelope == nil {
		t.Error("Expected error envelope for invalid scheme, got nil")
	}
	if envelope != nil {
		if envelope.Ok {
			t.Error("Expected Ok=false in error envelope")
		}
		if envelope.Error == nil {
			t.Error("Expected Error detail in error envelope")
		} else {
			if envelope.Error.Code != "CONFIG_ERROR" {
				t.Errorf("Expected error code CONFIG_ERROR, got: %s", envelope.Error.Code)
			}
			if envelope.Error.Message == "" {
				t.Error("Expected non-empty error message")
			}
		}
	}
}

func TestMySQLURLToDriverDSNOrError_Success(t *testing.T) {
	dsn := "mysql://user:pass@localhost:3306/mydb"
	expected := "user:pass@tcp(localhost:3306)/mydb"

	result, envelope := MySQLURLToDriverDSNOrError(dsn)
	if envelope != nil {
		t.Errorf("Expected nil envelope on success, got: %v", envelope)
	}
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestMySQLURLToDriverDSNOrError_Error(t *testing.T) {
	dsn := "http://user:pass@localhost:3306/mydb"

	result, envelope := MySQLURLToDriverDSNOrError(dsn)
	if result != "" {
		t.Errorf("Expected empty result on error, got: %q", result)
	}
	if envelope == nil {
		t.Error("Expected error envelope, got nil")
	}
	if envelope != nil {
		if envelope.Ok {
			t.Error("Expected Ok=false in error envelope")
		}
		if envelope.Error == nil {
			t.Error("Expected Error detail in error envelope")
		} else {
			if envelope.Error.Code != "CONFIG_ERROR" {
				t.Errorf("Expected error code CONFIG_ERROR, got: %s", envelope.Error.Code)
			}
			if envelope.Error.Message == "" {
				t.Error("Expected non-empty error message")
			}
		}
	}
}

// Edge case tests
func TestValidateMySQLURL_IPAddress(t *testing.T) {
	dsn := "mysql://user:pass@127.0.0.1:3306/mydb"
	err := ValidateMySQLURL(dsn)
	if err != nil {
		t.Errorf("Expected valid URL with IP address to pass, got error: %v", err)
	}
}

func TestMySQLURLToDriverDSN_IPAddress(t *testing.T) {
	dsn := "mysql://user:pass@127.0.0.1:3306/mydb"
	expected := "user:pass@tcp(127.0.0.1:3306)/mydb"

	result, err := MySQLURLToDriverDSN(dsn)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestValidateMySQLURL_EmptyPassword(t *testing.T) {
	// Empty password is allowed: mysql://user:@host:port/db
	dsn := "mysql://user:@localhost:3306/mydb"
	err := ValidateMySQLURL(dsn)
	if err != nil {
		t.Errorf("Expected valid URL with empty password to pass, got error: %v", err)
	}
}

func TestMySQLURLToDriverDSN_EmptyPassword(t *testing.T) {
	dsn := "mysql://user:@localhost:3306/mydb"
	// Empty password should still have the colon: user:@tcp(...)
	result, err := MySQLURLToDriverDSN(dsn)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if !strings.HasPrefix(result, "user:@tcp(") {
		t.Errorf("Expected result to start with 'user:@tcp(', got: %q", result)
	}
}

func TestValidateMySQLURL_PortZero(t *testing.T) {
	// Port 0 is technically valid syntax (though not useful)
	dsn := "mysql://user:pass@localhost:0/mydb"
	err := ValidateMySQLURL(dsn)
	if err != nil {
		t.Errorf("Expected valid URL with port 0 to pass, got error: %v", err)
	}
}

func TestMySQLURLToDriverDSN_PortZero(t *testing.T) {
	dsn := "mysql://user:pass@localhost:0/mydb"
	expected := "user:pass@tcp(localhost:0)/mydb"

	result, err := MySQLURLToDriverDSN(dsn)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestValidateMySQLURL_EmptyUser(t *testing.T) {
	// Empty user is technically valid URL syntax
	dsn := "mysql://:pass@localhost:3306/mydb"
	err := ValidateMySQLURL(dsn)
	if err != nil {
		t.Errorf("Expected valid URL with empty user to pass, got error: %v", err)
	}
}

func TestMySQLURLToDriverDSN_EmptyUser(t *testing.T) {
	dsn := "mysql://:pass@localhost:3306/mydb"
	result, err := MySQLURLToDriverDSN(dsn)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	// Empty user with password: :pass@tcp(...)
	if !strings.HasPrefix(result, ":pass@tcp(") {
		t.Errorf("Expected result to start with ':pass@tcp(', got: %q", result)
	}
}

func TestValidateMySQLURL_EdgeCasePorts(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"high port", "mysql://user:pass@localhost:65535/mydb"},
		{"standard MySQL port", "mysql://user:pass@localhost:3306/mydb"},
		{"custom port", "mysql://user:pass@localhost:3307/mydb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMySQLURL(tt.dsn)
			if err != nil {
				t.Errorf("Expected valid URL to pass, got error: %v", err)
			}
		})
	}
}