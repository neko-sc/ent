package dialect

import (
	"database/sql"
	"fmt"
)

// ConstraintKind classifies a violated database constraint.
type ConstraintKind uint8

// Constraint kinds reported by drivers.
const (
	Unique ConstraintKind = iota
	ForeignKey
	Check
	NotNull
	Exclusion
)

// ConstraintError is returned by drivers when a statement violates a constraint.
type ConstraintError struct {
	Kind       ConstraintKind
	Constraint string
	Table      string
	Err        error
}

func (e *ConstraintError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("dialect: constraint %q violated on table %q", e.Constraint, e.Table)
}

func (e *ConstraintError) Unwrap() error { return e.Err }

// UnsupportedError is returned when a feature is used on a dialect that lacks it.
type UnsupportedError struct {
	Feature string
	Dialect Dialect
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("dialect: %s is not supported by %s", e.Feature, e.Dialect)
}

// ErrNoRows is returned when a statement that must produce a row produces none.
// It is the database/sql sentinel so that errors.Is(err, sql.ErrNoRows) keeps working.
var ErrNoRows = sql.ErrNoRows
