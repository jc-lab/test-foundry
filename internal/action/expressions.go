// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"github.com/jc-lab/test-foundry/internal/expr"
)

func ResolveParams(params map[string]any, actx *ActionContext) (map[string]any, error) {
	var ctx *expr.Context
	if actx != nil {
		ctx = actx.ExprContext()
	}

	resolved, err := expr.ResolveMap(params, ctx)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}
