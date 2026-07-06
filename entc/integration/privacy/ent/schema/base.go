// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/entc/integration/privacy/ent/privacy"
	"github.com/neko-sc/ent/entc/integration/privacy/rule"
	"github.com/neko-sc/ent/schema/mixin"
)

// BaseMixin for all schemas.
type BaseMixin struct {
	mixin.Schema
}

// Policy defines shared policy for all schemas.
func (BaseMixin) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			rule.DenyIfNoViewer(),
			rule.AllowIfAdmin(),
		},
		Query: privacy.QueryPolicy{
			rule.DenyIfNoViewer(),
			rule.AllowIfAdmin(),
		},
	}
}
