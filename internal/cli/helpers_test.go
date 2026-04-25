// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package cli

import (
	"testing"

	"github.com/jc-lab/test-foundry/internal/action"
	"github.com/jc-lab/test-foundry/internal/config"
	"github.com/jc-lab/test-foundry/internal/workspace"
)

func TestResolveTestSerialLog(t *testing.T) {
	layout := &workspace.Layout{Root: "/work/vm"}
	testCfg := &config.TestConfig{
		QEMU: config.TestQEMUConfig{
			Serial: "${{ output.dir }}/serial.log",
		},
	}
	actx := &action.ActionContext{
		TestDir: "/tests",
		OutDir:  "/output",
	}

	got, err := resolveTestSerialLog(testCfg, actx, layout)
	if err != nil {
		t.Fatalf("resolveTestSerialLog failed: %v", err)
	}
	if got != "/output/serial.log" {
		t.Fatalf("got %q, want %q", got, "/output/serial.log")
	}
}

func TestResolveTestSerialLog_Default(t *testing.T) {
	layout := &workspace.Layout{Root: "/work/vm"}
	got, err := resolveTestSerialLog(&config.TestConfig{}, &action.ActionContext{}, layout)
	if err != nil {
		t.Fatalf("resolveTestSerialLog failed: %v", err)
	}
	if got != layout.SerialLog() {
		t.Fatalf("got %q, want %q", got, layout.SerialLog())
	}
}
