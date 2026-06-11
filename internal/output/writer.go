package output

import (
	"encoding/json"
	"io"
	"os"
)

// WriteSuccess writes a success envelope to the provided writer.
// It creates a success envelope using the provided columns, rows, row count, and elapsed time,
// marshals it to JSON, and writes it to w.
//
// Per D11: elapsedMs is wall-clock milliseconds from handler entry to last row read
// (includes connection setup).
//
// Note: The rowCount parameter is currently unused as NewSuccessEnvelope calculates
// row count from len(rows). This parameter is retained for API consistency and future use.
func WriteSuccess(w io.Writer, columns []Column, rows []any, rowCount int, elapsedMs int64) error {
	envelope := NewSuccessEnvelope(columns, rows, elapsedMs)

	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

// WriteSuccessWithTableComment writes a success envelope with table comment.
// Used by describe command to include table-level metadata.
func WriteSuccessWithTableComment(w io.Writer, columns []Column, rows []any, rowCount int, elapsedMs int64, tableComment string) error {
	envelope := NewSuccessEnvelope(columns, rows, elapsedMs)
	envelope.TableComment = tableComment

	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

// WriteError writes a failure envelope to stdout.
// This is a convenience function for CLI handlers that need to emit JSON errors.
//
// Note: This is a "fire and forget" function in error paths. If stdout write fails
// (e.g., broken pipe), the error is not propagated up because the CLI is already
// in an error state and about to exit. The returned error is primarily useful
// for testing and non-error-path scenarios.
func WriteError(envelope *Envelope) error {
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	// Append newline to prevent next shell prompt from appearing on same line
	data = append(data, '\n')

	_, err = os.Stdout.Write(data)
	return err
}

// WriteEnvelope writes an envelope to stdout.
// This is a convenience function for CLI handlers that need to emit JSON output.
//
// Note: Appends a newline after JSON to prevent next shell prompt from appearing
// on the same line, improving terminal display UX.
func WriteEnvelope(envelope *Envelope) error {
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	// Append newline to prevent next shell prompt from appearing on same line
	data = append(data, '\n')

	_, err = os.Stdout.Write(data)
	return err
}

// WriteFailure writes a failure envelope to the provided writer.
// It creates a failure envelope using the provided error code, message, and details,
// marshals it to JSON, and writes it to w.
//
// Per D10: details is a map[string]any carrying mysql_error_code (uint16) and optionally sql (string).
// The output layer copies details into the failure envelope verbatim.
//
// Per D2: INTERNAL_ERROR path has nil/empty details per failure-envelope schema.
func WriteFailure(w io.Writer, code ErrorCode, message string, details map[string]any) error {
	envelope := NewErrorEnvelope(code, message, details)

	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}
