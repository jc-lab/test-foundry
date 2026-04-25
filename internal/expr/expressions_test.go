// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package expr

import "testing"

func TestResolve(t *testing.T) {
	t.Run("test_and_output_dirs", func(t *testing.T) {
		got, err := Resolve("serial:${{ output.dir }}", &Context{
			TestDir: "/tmp/test",
			OutDir:  "/tmp/output",
		})
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if got != "serial:/tmp/output" {
			t.Fatalf("got %v, want %q", got, "serial:/tmp/output")
		}
	})

	t.Run("vmconfig_path", func(t *testing.T) {
		got, err := Resolve("${{ vmconfig.nested.value }}", &Context{
			VMConfig: func() (map[string]any, error) {
				return map[string]any{
					"nested": map[string]any{
						"value": "ok",
					},
				}, nil
			},
		})
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if got != "ok" {
			t.Fatalf("got %v, want %q", got, "ok")
		}
	})

	t.Run("resolve_map", func(t *testing.T) {
		got, err := ResolveMap(map[string]any{
			"path": "${{ test.dir }}/file.txt",
			"args": []any{"--output", "${{ output.dir }}"},
		}, &Context{
			TestDir: "/tmp/test",
			OutDir:  "/tmp/output",
		})
		if err != nil {
			t.Fatalf("ResolveMap failed: %v", err)
		}
		if got["path"] != "/tmp/test/file.txt" {
			t.Fatalf("path = %v, want %q", got["path"], "/tmp/test/file.txt")
		}
		args, ok := got["args"].([]any)
		if !ok || len(args) != 2 {
			t.Fatalf("args = %#v, want 2 items", got["args"])
		}
		if args[1] != "/tmp/output" {
			t.Fatalf("args[1] = %v, want %q", args[1], "/tmp/output")
		}
	})
}
