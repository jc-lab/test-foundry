// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/jc-lab/test-foundry/internal/logging"

	"github.com/jc-lab/test-foundry/internal/action"
	"github.com/jc-lab/test-foundry/internal/config"
)

// StepResult holds the result of a single step execution.
type StepResult struct {
	Action     string         `json:"action"`
	Status     string         `json:"status"` // "passed", "failed", "skipped"
	DurationMs int64          `json:"duration_ms"`
	Error      string         `json:"error,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
}

// RunResult holds the aggregate result of all step executions.
type RunResult struct {
	Steps         []StepResult `json:"steps"`
	PanicSteps    []StepResult `json:"panic_steps,omitempty"`
	PanicDetected bool         `json:"panic_detected,omitempty"`
}

// Runner executes a sequence of steps with timeout management.
type Runner struct {
	registry *action.Registry
	actx     *action.ActionContext
}

// NewRunner creates a new Runner.
func NewRunner(registry *action.Registry, actx *action.ActionContext) *Runner {
	return &Runner{
		registry: registry,
		actx:     actx,
	}
}

// RunSteps executes a slice of steps sequentially.
// If a step fails, remaining steps are marked as "skipped".
// If a panic is detected via panicCh, the current step is cancelled,
// marked as failed with "panic detected", and remaining steps are skipped.
func (r *Runner) RunSteps(ctx context.Context, steps []config.Step, panicCh <-chan struct{}) (*RunResult, error) {
	result := &RunResult{}

	for i, step := range steps {
		stepResult := StepResult{
			Action: step.Action,
		}

		resolvedParams, err := action.ResolveParams(step.Params, r.actx)
		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = fmt.Sprintf("failed to resolve params: %v", err)
			result.Steps = append(result.Steps, stepResult)
			for _, remaining := range steps[i+1:] {
				result.Steps = append(result.Steps, StepResult{
					Action: remaining.Action,
					Status: "skipped",
					Params: remaining.Params,
				})
			}
			return result, nil
		}
		stepResult.Params = resolvedParams

		// Look up the action from the registry
		act, err := r.registry.Get(step.Action)
		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = fmt.Sprintf("unknown action: %s", step.Action)
			result.Steps = append(result.Steps, stepResult)
			// Skip remaining steps
			for _, remaining := range steps[i+1:] {
				result.Steps = append(result.Steps, StepResult{
					Action: remaining.Action,
					Status: "skipped",
					Params: remaining.Params,
				})
			}
			return result, nil
		}

		// Create a timeout context for this step
		stepCtx, stepCancel := context.WithTimeout(ctx, step.Timeout.Duration)

		logging.Info("Executing step", "index", i, "action", step.Action, "timeout", step.Timeout.Duration)

		startTime := time.Now()

		// Execute the action in a goroutine so we can also listen for panic
		doneCh := make(chan error, 1)
		go func() {
			doneCh <- act.Execute(stepCtx, r.actx, resolvedParams)
		}()

		var stepErr error
		panicDetected := false

		select {
		case stepErr = <-doneCh:
			// Step completed (success or failure)
		case _ = <-panicCh:
			// Panic detected — cancel the current step
			stepCancel()

			// FIXME: winrm cannot handle cancellation
			// <-doneCh // Wait for the goroutine to finish

			panicDetected = true
			stepErr = fmt.Errorf("panic detected")
		}

		duration := time.Since(startTime)
		stepResult.DurationMs = duration.Milliseconds()

		stepCancel()

		if stepErr != nil {
			stepResult.Status = "failed"
			stepResult.Error = stepErr.Error()
			result.Steps = append(result.Steps, stepResult)

			if panicDetected {
				result.PanicDetected = true
			}

			logging.Error("Step failed", "index", i, "action", step.Action, "duration", duration, "error", stepErr)

			// Skip remaining steps
			for _, remaining := range steps[i+1:] {
				result.Steps = append(result.Steps, StepResult{
					Action: remaining.Action,
					Status: "skipped",
					Params: remaining.Params,
				})
			}
			return result, nil
		}

		stepResult.Status = "passed"
		result.Steps = append(result.Steps, stepResult)
		logging.Info("Step passed", "index", i, "action", step.Action, "duration", duration)
	}

	return result, nil
}

// RunPanicSteps executes the panic steps (for diagnostics after BSOD).
// Steps are executed best-effort: failures are recorded but do not stop execution.
func (r *Runner) RunPanicSteps(ctx context.Context, steps []config.Step) ([]StepResult, error) {
	var results []StepResult

	for i, step := range steps {
		stepResult := StepResult{
			Action: step.Action,
		}

		resolvedParams, err := action.ResolveParams(step.Params, r.actx)
		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = fmt.Sprintf("failed to resolve params: %v", err)
			results = append(results, stepResult)
			continue
		}
		stepResult.Params = resolvedParams

		act, err := r.registry.Get(step.Action)
		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = fmt.Sprintf("unknown action: %s", step.Action)
			results = append(results, stepResult)
			continue
		}

		stepCtx, stepCancel := context.WithTimeout(ctx, step.Timeout.Duration)

		logging.Info("Executing panic step", "index", i, "action", step.Action, "timeout", step.Timeout.Duration)

		startTime := time.Now()
		err = act.Execute(stepCtx, r.actx, resolvedParams)
		duration := time.Since(startTime)
		stepCancel()

		stepResult.DurationMs = duration.Milliseconds()

		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = err.Error()
			logging.Error("Panic step failed", "index", i, "action", step.Action, "duration", duration, "error", err)
		} else {
			stepResult.Status = "passed"
			logging.Info("Panic step passed", "index", i, "action", step.Action, "duration", duration)
		}

		results = append(results, stepResult)
	}

	return results, nil
}
