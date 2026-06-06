package output

import (
	"encoding/json"
	"io"
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