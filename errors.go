package ent

import (
	"errors"
	"fmt"

	"github.com/neko-sc/ent/dialect"
)

// NotFoundError indicates that a query requiring an entity found no rows.
type NotFoundError struct{ Label string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("ent: %s not found", e.Label) }

// NotSingularError indicates that a query requiring one entity found multiple rows.
type NotSingularError struct{ Label string }

func (e *NotSingularError) Error() string {
	return fmt.Sprintf("ent: %s not singular", e.Label)
}

// ValidationError associates a validation failure with a field name.
type ValidationError struct {
	Name string
	Err  error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("ent: validator failed for field %q: %v", e.Name, e.Err)
}

func (e *ValidationError) Unwrap() error { return e.Err }

// ErrConflict is returned when an upsert's DoNothing action encounters an existing row.
var ErrConflict = errors.New("ent: conflict")

// IsNotFound reports whether err wraps a NotFoundError.
func IsNotFound(err error) bool {
	_, matches := errors.AsType[*NotFoundError](err)
	return matches
}

// IsNotSingular reports whether err wraps a NotSingularError.
func IsNotSingular(err error) bool {
	_, matches := errors.AsType[*NotSingularError](err)
	return matches
}

// IsValidationError reports whether err wraps a ValidationError.
func IsValidationError(err error) bool {
	_, matches := errors.AsType[*ValidationError](err)
	return matches
}

// IsConstraintError reports whether err wraps a driver's constraint violation.
func IsConstraintError(err error) bool {
	_, matches := errors.AsType[*dialect.ConstraintError](err)
	return matches
}
