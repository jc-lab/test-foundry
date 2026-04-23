// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package preboot

import (
	"fmt"
	"regexp"
	"strings"
)

var expressionPattern = regexp.MustCompile(`\$\{\{\s*([^}]+?)\s*\}\}`)

func ResolveParams(params map[string]any, actx *ActionContext) (map[string]any, error) {
	if params == nil {
		return nil, nil
	}

	resolved, err := resolveExpressionValue(params, actx)
	if err != nil {
		return nil, err
	}

	out, ok := resolved.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("params must resolve to an object")
	}
	return out, nil
}

func resolveExpressionValue(value any, actx *ActionContext) (any, error) {
	switch v := value.(type) {
	case string:
		return resolveExpressionString(v, actx)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			resolved, err := resolveExpressionValue(v[i], actx)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			resolved, err := resolveExpressionValue(item, actx)
			if err != nil {
				return nil, fmt.Errorf("param %q: %w", key, err)
			}
			out[key] = resolved
		}
		return out, nil
	default:
		return value, nil
	}
}

func resolveExpressionString(input string, actx *ActionContext) (any, error) {
	matches := expressionPattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}

	if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(input) {
		expr := strings.TrimSpace(input[matches[0][2]:matches[0][3]])
		return evaluateExpression(expr, actx)
	}

	var builder strings.Builder
	last := 0
	for _, match := range matches {
		builder.WriteString(input[last:match[0]])

		expr := strings.TrimSpace(input[match[2]:match[3]])
		value, err := evaluateExpression(expr, actx)
		if err != nil {
			return nil, err
		}
		builder.WriteString(fmt.Sprint(value))
		last = match[1]
	}
	builder.WriteString(input[last:])

	return builder.String(), nil
}

func evaluateExpression(expr string, actx *ActionContext) (any, error) {
	switch expr {
	case "test.dir":
		if actx == nil || actx.TestDir == "" {
			return nil, fmt.Errorf("test.dir is not available in this context")
		}
		return actx.TestDir, nil
	default:
		return nil, fmt.Errorf("unknown expression %q", expr)
	}
}
