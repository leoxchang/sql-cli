package output

import (
	"database/sql"
	"fmt"
	"time"
)

// ConvertRows converts sql.Rows to a slice of Column structs and a 2D slice of JSON-ready values.
// It handles type conversion according to the specification:
//   - nil → JSON null
//   - []byte → JSON string (NOT base64)
//   - time.Time → JSON string in RFC3339 format
//   - Numeric types → JSON numbers
//   - Strings → JSON strings
//   - Booleans → JSON booleans
//
// Column types preserve the exact case returned by DatabaseTypeName() (verbatim, no transformation).
func ConvertRows(rows *sql.Rows) ([]Column, [][]any, error) {
	// Get column types for type information
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get column types: %w", err)
	}

	// Build Column slice with preserved type names
	columns := make([]Column, len(columnTypes))
	for i, ct := range columnTypes {
		columns[i] = Column{
			Name: ct.Name(),
			Type: ct.DatabaseTypeName(), // Preserve case verbatim (D12)
		}
	}

	// Prepare for scanning
	var allRows [][]any

	for rows.Next() {
		// Create scan destinations
		values := make([]any, len(columnTypes))
		scanDest := make([]any, len(columnTypes))
		for i := range values {
			scanDest[i] = &values[i]
		}

		// Scan row
		if err := rows.Scan(scanDest...); err != nil {
			return nil, nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert each value to JSON-ready format
		convertedRow := make([]any, len(values))
		for i, v := range values {
			convertedRow[i] = convertValue(v)
		}

		allRows = append(allRows, convertedRow)
	}

	// Check for errors during iteration
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return columns, allRows, nil
}

// convertValue converts a single value from sql.Rows to a JSON-ready format.
func convertValue(v any) any {
	// Handle NULL values
	if v == nil {
		return nil // Will become JSON null
	}

	// Type switch for all supported MySQL types
	switch val := v.(type) {
	// Strings
	case string:
		return val

	// Bytes - convert to string (NOT base64) per specification
	case []byte:
		return string(val)

	// Booleans
	case bool:
		return val

	// Integer types
	case int:
		return val
	case int8:
		return val
	case int16:
		return val
	case int32:
		return val
	case int64:
		return val
	case uint:
		return val
	case uint8:
		return val
	case uint16:
		return val
	case uint32:
		return val
	case uint64:
		return val

	// Floating point types
	case float32:
		return val
	case float64:
		return val

	// Time types - convert to RFC3339 string
	case time.Time:
		return val.Format(time.RFC3339)

	// Nil interface
	case nil:
		return nil

	// Unknown type - return as-is and let JSON marshaler handle it
	default:
		return val
	}
}