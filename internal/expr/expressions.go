// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package expr

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var expressionPattern = regexp.MustCompile(`\$\{\{\s*([^}]+?)\s*\}\}`)

// Context provides the runtime values available to expression evaluation.
type Context struct {
	TestDir  string
	OutDir   string
	VMConfig func() (map[string]any, error)
}

// Resolve recursively resolves expression strings inside the provided value.
func Resolve(value any, ctx *Context) (any, error) {
	switch v := value.(type) {
	case string:
		return resolveString(v, ctx)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			resolved, err := Resolve(v[i], ctx)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			resolved, err := Resolve(item, ctx)
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

// ResolveMap resolves expressions inside a map value and returns a map.
func ResolveMap(params map[string]any, ctx *Context) (map[string]any, error) {
	if params == nil {
		return nil, nil
	}

	resolved, err := Resolve(params, ctx)
	if err != nil {
		return nil, err
	}

	out, ok := resolved.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("value must resolve to an object")
	}
	return out, nil
}

func resolveString(input string, ctx *Context) (any, error) {
	matches := expressionPattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}

	if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(input) {
		expr := strings.TrimSpace(input[matches[0][2]:matches[0][3]])
		return evaluate(expr, ctx)
	}

	var builder strings.Builder
	last := 0
	for _, match := range matches {
		builder.WriteString(input[last:match[0]])

		expr := strings.TrimSpace(input[match[2]:match[3]])
		value, err := evaluate(expr, ctx)
		if err != nil {
			return nil, err
		}
		builder.WriteString(fmt.Sprint(value))
		last = match[1]
	}
	builder.WriteString(input[last:])

	return builder.String(), nil
}

func evaluate(expr string, ctx *Context) (any, error) {
	switch {
	case expr == "test.dir":
		if ctx == nil || ctx.TestDir == "" {
			return nil, fmt.Errorf("test.dir is not available in this context")
		}
		return ctx.TestDir, nil
	case expr == "output.dir":
		if ctx == nil || ctx.OutDir == "" {
			return nil, fmt.Errorf("output.dir is not available in this context")
		}
		return ctx.OutDir, nil
	case strings.HasPrefix(expr, "env."):
		key := strings.TrimPrefix(expr, "env.")
		if key == "" {
			return nil, fmt.Errorf("env expression requires a variable name")
		}
		return os.Getenv(key), nil
	case strings.HasPrefix(expr, "vmconfig."):
		if ctx == nil || ctx.VMConfig == nil {
			return nil, fmt.Errorf("vmconfig is not available in this context")
		}
		vmcfg, err := ctx.VMConfig()
		if err != nil {
			return nil, err
		}
		return lookupPath(vmcfg, strings.TrimPrefix(expr, "vmconfig."))
	default:
		return nil, fmt.Errorf("unknown expression %q", expr)
	}
}

func lookupPath(root map[string]any, path string) (any, error) {
	current := any(root)
	for _, segment := range strings.Split(path, ".") {
		nextMap, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expression path %q is not an object", path)
		}
		value, ok := nextMap[segment]
		if !ok {
			return nil, fmt.Errorf("unknown vmconfig field %q", path)
		}
		current = value
	}
	return current, nil
}

// StructToMap converts any struct-like value into a map using JSON tags.
func StructToMap(value any) (map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal value: %w", err)
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("failed to decode value: %w", err)
	}
	return out, nil
}
