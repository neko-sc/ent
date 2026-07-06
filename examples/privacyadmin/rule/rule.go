// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package rule

import (
	"context"

	"github.com/neko-sc/ent/examples/privacyadmin/ent/privacy"
	"github.com/neko-sc/ent/examples/privacyadmin/viewer"
)

// DenyIfNoViewer is a rule that returns deny decision if the viewer is missing in the context.
func DenyIfNoViewer() privacy.QueryMutationRule {
	return privacy.ContextQueryMutationRule(func(ctx context.Context) error {
		view := viewer.FromContext(ctx)
		if view == nil {
			return privacy.Denyf("viewer-context is missing")
		}
		// Skip to the next privacy rule (equivalent to return nil).
		return privacy.Skip
	})
}

// AllowIfAdmin is a rule that returns allow decision if the viewer is admin.
func AllowIfAdmin() privacy.QueryMutationRule {
	return privacy.ContextQueryMutationRule(func(ctx context.Context) error {
		view := viewer.FromContext(ctx)
		if view.Admin() {
			return privacy.Allow
		}
		// Skip to the next privacy rule (equivalent to return nil).
		return privacy.Skip
	})
}
