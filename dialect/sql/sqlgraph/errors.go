package sqlgraph

import (
	"errors"

	"github.com/neko-sc/ent/dialect"
)

func IsConstraintError(err error) bool {
	if _, ok := errors.AsType[*ConstraintError](err); ok {
		return true
	}
	_, ok := errors.AsType[*dialect.ConstraintError](err)
	return ok
}

func IsUniqueConstraintError(err error) bool {
	constraint, ok := errors.AsType[*dialect.ConstraintError](err)
	return ok && constraint != nil && constraint.Kind == dialect.Unique
}

func IsForeignKeyConstraintError(err error) bool {
	constraint, ok := errors.AsType[*dialect.ConstraintError](err)
	return ok && constraint != nil && constraint.Kind == dialect.ForeignKey
}

func IsCheckConstraintError(err error) bool {
	constraint, ok := errors.AsType[*dialect.ConstraintError](err)
	return ok && constraint != nil && constraint.Kind == dialect.Check
}
