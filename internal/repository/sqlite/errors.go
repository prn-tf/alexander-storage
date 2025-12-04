package sqlite

import (
	"database/sql"
	"errors"
	"strings"
)

// Error handling utilities for SQLite.

// isUniqueViolation checks if an error is a unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// SQLite unique constraint error message
	errStr := err.Error()
	return strings.Contains(errStr, "UNIQUE constraint failed") ||
		strings.Contains(errStr, "constraint failed: UNIQUE")
}

// isForeignKeyViolation checks if an error is a foreign key constraint violation.
func isForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "FOREIGN KEY constraint failed")
}

// isNoRows checks if an error indicates no rows were found.
func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
