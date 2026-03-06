// Package repository provides common errors for repositories.
package repository

import "errors"

// Common repository errors.
var (
	// ErrNotFound is returned when a record is not found.
	ErrNotFound = errors.New("record not found")

	// ErrDuplicate is returned when a duplicate record exists.
	ErrDuplicate = errors.New("duplicate record")

	// ErrInvalidInput is returned for invalid input data.
	ErrInvalidInput = errors.New("invalid input")
)
