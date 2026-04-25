// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package preboot

import (
	"github.com/jc-lab/test-foundry/internal/expr"
)

func ResolveParams(params map[string]any, actx *ActionContext) (map[string]any, error) {
	if actx == nil {
		return expr.ResolveMap(params, nil)
	}

	resolved, err := expr.ResolveMap(params, &expr.Context{
		TestDir: actx.TestDir,
	})
	if err != nil {
		return nil, err
	}
	return resolved, nil
}
