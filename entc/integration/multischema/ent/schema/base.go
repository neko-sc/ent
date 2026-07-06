package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/schema"
)

// base holds the default configuration for most schemas in this package.
type base struct {
	ent.Schema
}

// Annotations of the base schema.
func (base) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Schema("db1"),
	}
}
