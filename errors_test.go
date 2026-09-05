package ent_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/stretchr/testify/require"
)

func TestErrorClassification(t *testing.T) {
	for _, test := range []struct {
		name     string
		err      error
		classify func(error) bool
		message  string
	}{
		{"not found", &ent.NotFoundError{Label: "user"}, ent.IsNotFound, "ent: user not found"},
		{"not singular", &ent.NotSingularError{Label: "user"}, ent.IsNotSingular, "ent: user not singular"},
		{"validation", &ent.ValidationError{Name: "age", Err: errors.New("negative")}, ent.IsValidationError, `ent: validator failed for field "age": negative`},
		{"constraint", &dialect.ConstraintError{Err: errors.New("duplicate")}, ent.IsConstraintError, "duplicate"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.EqualError(t, test.err, test.message)
			require.True(t, test.classify(test.err))
			require.True(t, test.classify(fmt.Errorf("wrapped: %w", test.err)))
			require.True(t, test.classify(errors.Join(errors.New("other"), test.err)))
			require.False(t, test.classify(nil))
			require.False(t, test.classify(errors.New(test.message)))
		})
	}
	cause := errors.New("invalid")
	require.ErrorIs(t, &ent.ValidationError{Name: "name", Err: cause}, cause)
	require.ErrorIs(t, fmt.Errorf("insert: %w", ent.ErrConflict), ent.ErrConflict)
}
