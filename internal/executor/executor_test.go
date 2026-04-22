// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package executor

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jc-lab/test-foundry/internal/action"
	"github.com/jc-lab/test-foundry/internal/config"
	"github.com/jc-lab/test-foundry/internal/qemu"
)

// --- mockAction implements action.Action ---

type mockAction struct {
	name   string
	execFn func(ctx context.Context, actx *action.ActionContext, params map[string]any) error
}

func (m *mockAction) Name() string { return m.name }

func (m *mockAction) Execute(ctx context.Context, actx *action.ActionContext, params map[string]any) error {
	if m.execFn != nil {
		return m.execFn(ctx, actx, params)
	}
	return nil
}

// newMockRegistry creates a Registry with the given mock actions registered.
func newMockRegistry(actions ...action.Action) *action.Registry {
	r := &action.Registry{}
	// We need to use NewRegistry but replace with our mocks, or build manually.
	// Since Registry.actions is unexported, we use Register on a fresh registry.
	// However, NewRegistry() registers built-in actions. We need a clean one.
	// We can work around this by creating a NewRegistry and registering our mocks
	// (which will override any built-in with the same name).
	r = action.NewRegistry()
	for _, a := range actions {
		r.Register(a)
	}
	return r
}

// makeStep creates a config.Step with the given action name and timeout duration.
func makeStep(actionName string, timeout time.Duration) config.Step {
	return config.Step{
		Action:  actionName,
		Timeout: config.Duration{Duration: timeout},
		Params:  map[string]any{},
	}
}

// nilPanicCh returns a nil channel (no panic will be signaled).
func nilPanicCh() <-chan struct{} {
	return nil
}

// --- TestRunSteps_AllPass ---

func TestRunSteps_AllPass(t *testing.T) {
	callCount := 0
	mock := &mockAction{
		name: "mock-action",
		execFn: func(ctx context.Context, actx *action.ActionContext, params map[string]any) error {
			callCount++
			return nil
		},
	}

	registry := newMockRegistry(mock)
	runner := NewRunner(registry, &action.ActionContext{})

	steps := []config.Step{
		makeStep("mock-action", 5*time.Second),
		makeStep("mock-action", 5*time.Second),
		makeStep("mock-action", 5*time.Second),
	}

	result, err := runner.RunSteps(context.Background(), steps, nilPanicCh())
	if err != nil {
		t.Fatalf("RunSteps returned error: %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}

	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(result.Steps))
	}

	for i, sr := range result.Steps {
		if sr.Status != "passed" {
			t.Errorf("step[%d].Status = %q, want %q", i, sr.Status, "passed")
		}
		if sr.DurationMs < 0 {
			t.Errorf("step[%d].DurationMs = %d, expected >= 0", i, sr.DurationMs)
		}
	}
}

func TestRunSteps_ResolvesExpressionsAtRuntime(t *testing.T) {
	dir := t.TempDir()

	var gotParams map[string]any
	mock := &mockAction{
		name: "mock-action",
		execFn: func(ctx context.Context, actx *action.ActionContext, params map[string]any) error {
			gotParams = params
			return nil
		},
	}

	registry := newMockRegistry(mock)
	runner := NewRunner(registry, &action.ActionContext{
		Machine: &qemu.Machine{
			Config: &qemu.MachineConfig{
				MachineName: "test-vm",
				SSHHostPort: 2222,
			},
		},
		WorkDir: dir,
		TestDir: filepath.Join(dir, "tests"),
	})

	steps := []config.Step{
		{
			Action:  "mock-action",
			Timeout: config.Duration{Duration: 5 * time.Second},
			Params: map[string]any{
				"path":     "${{ test.dir }}/artifact.txt",
				"name":     "${{ vmconfig.machine_name }}",
				"ssh_port": "${{ vmconfig.ssh_host_port }}",
			},
		},
	}

	result, err := runner.RunSteps(context.Background(), steps, nilPanicCh())
	if err != nil {
		t.Fatalf("RunSteps returned error: %v", err)
	}
	if len(result.Steps) != 1 || result.Steps[0].Status != "passed" {
		t.Fatalf("unexpected result: %#v", result.Steps)
	}
	gotPath := filepath.Clean(fmt.Sprint(gotParams["path"]))
	wantPath := filepath.Join(dir, "tests", "artifact.txt")
	if gotPath != wantPath {
		t.Fatalf("path = %v, want %v", gotPath, wantPath)
	}
	if gotParams["name"] != "test-vm" {
		t.Fatalf("name = %v, want test-vm", gotParams["name"])
	}
	if gotParams["ssh_port"] != float64(2222) {
		t.Fatalf("ssh_port = %#v, want 2222", gotParams["ssh_port"])
	}
}

func TestRunSteps_UnknownExpressionFailsAtRuntime(t *testing.T) {
	mock := &mockAction{name: "mock-action"}
	registry := newMockRegistry(mock)
	runner := NewRunner(registry, &action.ActionContext{
		WorkDir: t.TempDir(),
		TestDir: t.TempDir(),
	})

	steps := []config.Step{
		{
			Action:  "mock-action",
			Timeout: config.Duration{Duration: 5 * time.Second},
			Params: map[string]any{
				"value": "${{ test.unknown }}",
			},
		},
	}

	result, err := runner.RunSteps(context.Background(), steps, nilPanicCh())
	if err != nil {
		t.Fatalf("RunSteps returned error: %v", err)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step result, got %d", len(result.Steps))
	}
	if result.Steps[0].Status != "failed" {
		t.Fatalf("step status = %q, want failed", result.Steps[0].Status)
	}
}

// --- TestRunSteps_FailureSkipsRemaining ---

func TestRunSteps_FailureSkipsRemaining(t *testing.T) {
	callCount := 0
	mock := &mockAction{
		name: "mock-action",
		execFn: func(ctx context.Context, actx *action.ActionContext, params map[string]any) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("intentional failure on step 2")
			}
			return nil
		},
	}

	registry := newMockRegistry(mock)
	runner := NewRunner(registry, &action.ActionContext{})

	steps := []config.Step{
		makeStep("mock-action", 5*time.Second),
		makeStep("mock-action", 5*time.Second),
		makeStep("mock-action", 5*time.Second),
	}

	result, err := runner.RunSteps(context.Background(), steps, nilPanicCh())
	if err != nil {
		t.Fatalf("RunSteps returned error: %v", err)
	}

	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(result.Steps))
	}

	if result.Steps[0].Status != "passed" {
		t.Errorf("step[0].Status = %q, want %q", result.Steps[0].Status, "passed")
	}
	if result.Steps[1].Status != "failed" {
		t.Errorf("step[1].Status = %q, want %q", result.Steps[1].Status, "failed")
	}
	if result.Steps[1].Error == "" {
		t.Error("step[1].Error should not be empty")
	}
	if result.Steps[2].Status != "skipped" {
		t.Errorf("step[2].Status = %q, want %q", result.Steps[2].Status, "skipped")
	}

	// Only 2 calls should have been made (step 3 was skipped)
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

// --- TestRunSteps_Timeout ---

func TestRunSteps_Timeout(t *testing.T) {
	mock := &mockAction{
		name: "slow-action",
		execFn: func(ctx context.Context, actx *action.ActionContext, params map[string]any) error {
			select {
			case <-time.After(10 * time.Second):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}

	registry := newMockRegistry(mock)
	runner := NewRunner(registry, &action.ActionContext{})

	steps := []config.Step{
		makeStep("slow-action", 100*time.Millisecond), // Very short timeout
	}

	result, err := runner.RunSteps(context.Background(), steps, nilPanicCh())
	if err != nil {
		t.Fatalf("RunSteps returned error: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step result, got %d", len(result.Steps))
	}

	if result.Steps[0].Status != "failed" {
		t.Errorf("step[0].Status = %q, want %q", result.Steps[0].Status, "failed")
	}

	if result.Steps[0].Error == "" {
		t.Error("step[0].Error should contain timeout/deadline error")
	}
}

// --- TestRunSteps_PanicDetected ---

func TestRunSteps_PanicDetected(t *testing.T) {
	panicCh := make(chan struct{}, 1)

	mock := &mockAction{
		name: "waiting-action",
		execFn: func(ctx context.Context, actx *action.ActionContext, params map[string]any) error {
			// Simulate work, but will be interrupted by panic
			select {
			case <-time.After(10 * time.Second):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}

	registry := newMockRegistry(mock)
	runner := NewRunner(registry, &action.ActionContext{})

	steps := []config.Step{
		makeStep("waiting-action", 5*time.Second),
		makeStep("waiting-action", 5*time.Second),
	}

	// Send panic signal after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		panicCh <- struct{}{}
	}()

	result, err := runner.RunSteps(context.Background(), steps, panicCh)
	if err != nil {
		t.Fatalf("RunSteps returned error: %v", err)
	}

	if !result.PanicDetected {
		t.Error("expected PanicDetected to be true")
	}

	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.Steps))
	}

	if result.Steps[0].Status != "failed" {
		t.Errorf("step[0].Status = %q, want %q", result.Steps[0].Status, "failed")
	}
	if result.Steps[1].Status != "skipped" {
		t.Errorf("step[1].Status = %q, want %q", result.Steps[1].Status, "skipped")
	}
}

// --- TestRunPanicSteps_BestEffort ---

func TestRunPanicSteps_BestEffort(t *testing.T) {
	callCount := 0
	mock := &mockAction{
		name: "panic-step-action",
		execFn: func(ctx context.Context, actx *action.ActionContext, params map[string]any) error {
			callCount++
			if callCount == 1 {
				return fmt.Errorf("first panic step failed")
			}
			return nil
		},
	}

	registry := newMockRegistry(mock)
	runner := NewRunner(registry, &action.ActionContext{})

	panicSteps := []config.Step{
		makeStep("panic-step-action", 5*time.Second),
		makeStep("panic-step-action", 5*time.Second),
		makeStep("panic-step-action", 5*time.Second),
	}

	results, err := runner.RunPanicSteps(context.Background(), panicSteps)
	if err != nil {
		t.Fatalf("RunPanicSteps returned error: %v", err)
	}

	// All 3 steps should have been executed (best-effort)
	if callCount != 3 {
		t.Errorf("expected 3 calls (best effort), got %d", callCount)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if results[0].Status != "failed" {
		t.Errorf("panic step[0].Status = %q, want %q", results[0].Status, "failed")
	}
	if results[0].Error == "" {
		t.Error("panic step[0].Error should not be empty")
	}

	if results[1].Status != "passed" {
		t.Errorf("panic step[1].Status = %q, want %q", results[1].Status, "passed")
	}
	if results[2].Status != "passed" {
		t.Errorf("panic step[2].Status = %q, want %q", results[2].Status, "passed")
	}
}

// --- TestRunSteps_UnknownAction ---

func TestRunSteps_UnknownAction(t *testing.T) {
	registry := action.NewRegistry()
	runner := NewRunner(registry, &action.ActionContext{})

	steps := []config.Step{
		makeStep("known-action-that-does-not-exist", 5*time.Second),
		makeStep("another-step", 5*time.Second),
	}

	// The first step has an unknown action, so it should fail and skip the rest
	// But actually we need to use a name that's NOT in the built-in registry
	result, err := runner.RunSteps(context.Background(), steps, nilPanicCh())
	if err != nil {
		t.Fatalf("RunSteps returned error: %v", err)
	}

	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.Steps))
	}

	if result.Steps[0].Status != "failed" {
		t.Errorf("step[0].Status = %q, want %q", result.Steps[0].Status, "failed")
	}
	if result.Steps[1].Status != "skipped" {
		t.Errorf("step[1].Status = %q, want %q", result.Steps[1].Status, "skipped")
	}
}
