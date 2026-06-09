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
	ID         string         `json:"id"`
	Name       string         `json:"name,omitempty"`
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

// ProgressCallback is called whenever the runner has updated result state.
type ProgressCallback func(*RunResult) error

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
func (r *Runner) RunSteps(ctx context.Context, steps []config.Step, panicCh <-chan struct{}, onProgress ProgressCallback) (*RunResult, error) {
	result := &RunResult{}
	notifyProgress := func() {
		if onProgress != nil {
			onProgress(result)
		}
	}

	for i, step := range steps {
		stepID := step.ID
		if stepID == "" {
			stepID = fmt.Sprintf("step_%d", i+1)
		}
		stepResult := StepResult{
			ID:     stepID,
			Name:   step.Name,
			Action: step.Action,
		}

		resolvedParams, err := action.ResolveParams(step.Params, r.actx)
		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = fmt.Sprintf("failed to resolve params: %v", err)
			result.Steps = append(result.Steps, stepResult)
			notifyProgress()
			for j, remaining := range steps[i+1:] {
				rid := remaining.ID
				if rid == "" {
					rid = fmt.Sprintf("step_%d", i+2+j)
				}
				result.Steps = append(result.Steps, StepResult{
					ID:     rid,
					Name:   remaining.Name,
					Action: remaining.Action,
					Status: "skipped",
					Params: remaining.Params,
				})
				notifyProgress()
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
			notifyProgress()
			// Skip remaining steps
			for j, remaining := range steps[i+1:] {
				rid := remaining.ID
				if rid == "" {
					rid = fmt.Sprintf("step_%d", i+2+j)
				}
				result.Steps = append(result.Steps, StepResult{
					ID:     rid,
					Name:   remaining.Name,
					Action: remaining.Action,
					Status: "skipped",
					Params: remaining.Params,
				})
				notifyProgress()
			}
			return result, nil
		}

		sctx := action.NewStepContext(r.actx)

		// Create a timeout context for this step
		stepCtx, stepCancel := context.WithTimeout(ctx, step.Timeout.Duration)

		logging.Info("Executing step", "index", i, "id", stepID, "name", step.Name, "action", step.Action, "timeout", step.Timeout.Duration)

		startTime := time.Now()

		// Execute the action in a goroutine so we can also listen for panic
		doneCh := make(chan error, 1)
		go func() {
			doneCh <- act.Execute(stepCtx, sctx, resolvedParams)
		}()

		var stepErr error
		panicDetected := false

		select {
		case stepErr = <-doneCh:
			// Step completed (success or failure).
			if stepErr != nil {
				if delay := panicActionDelay(r.actx); delay > 0 && panicCh != nil {
					select {
					case <-panicCh:
						panicDetected = true
						stepErr = fmt.Errorf("panic detected")
					case <-time.After(delay):
					}
				}
			}
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

		if r.actx.StepOutputs == nil {
			r.actx.StepOutputs = make(map[string]map[string]string)
		}
		r.actx.StepOutputs[stepID] = sctx.Outputs()

		if stepErr != nil {
			stepResult.Status = "failed"
			stepResult.Error = stepErr.Error()
			result.Steps = append(result.Steps, stepResult)

			if panicDetected {
				result.PanicDetected = true
			}

			logging.Error("Step failed", "index", i, "id", stepID, "name", step.Name, "action", step.Action, "duration", duration, "error", stepErr)
			notifyProgress()

			// Skip remaining steps
			for j, remaining := range steps[i+1:] {
				rid := remaining.ID
				if rid == "" {
					rid = fmt.Sprintf("step_%d", i+2+j)
				}
				result.Steps = append(result.Steps, StepResult{
					ID:     rid,
					Name:   remaining.Name,
					Action: remaining.Action,
					Status: "skipped",
					Params: remaining.Params,
				})
				notifyProgress()
			}
			return result, nil
		}

		stepResult.Status = "passed"
		result.Steps = append(result.Steps, stepResult)
		logging.Info("Step passed", "index", i, "id", stepID, "name", step.Name, "action", step.Action, "duration", duration)
		notifyProgress()
	}

	return result, nil
}

// RunPanicSteps executes the panic steps (for diagnostics after BSOD).
// Steps are executed best-effort: failures are recorded but do not stop execution.
func (r *Runner) RunPanicSteps(ctx context.Context, result *RunResult, steps []config.Step, onProgress ProgressCallback) ([]StepResult, error) {
	var results []StepResult
	notifyProgress := func() {
		if result != nil {
			result.PanicSteps = results
		}
		if onProgress != nil {
			onProgress(result)
		}
	}

	for i, step := range steps {
		stepID := step.ID
		if stepID == "" {
			stepID = fmt.Sprintf("step_%d", i+1)
		}
		stepResult := StepResult{
			ID:     stepID,
			Name:   step.Name,
			Action: step.Action,
		}

		resolvedParams, err := action.ResolveParams(step.Params, r.actx)
		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = fmt.Sprintf("failed to resolve params: %v", err)
			results = append(results, stepResult)
			notifyProgress()
			continue
		}
		stepResult.Params = resolvedParams

		act, err := r.registry.Get(step.Action)
		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = fmt.Sprintf("unknown action: %s", step.Action)
			results = append(results, stepResult)
			notifyProgress()
			continue
		}

		sctx := action.NewStepContext(r.actx)

		stepCtx, stepCancel := context.WithTimeout(ctx, step.Timeout.Duration)

		logging.Info("Executing panic step", "index", i, "id", stepID, "name", step.Name, "action", step.Action, "timeout", step.Timeout.Duration)

		startTime := time.Now()
		err = act.Execute(stepCtx, sctx, resolvedParams)
		duration := time.Since(startTime)
		stepCancel()

		if r.actx.StepOutputs == nil {
			r.actx.StepOutputs = make(map[string]map[string]string)
		}
		r.actx.StepOutputs[stepID] = sctx.Outputs()

		stepResult.DurationMs = duration.Milliseconds()

		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = err.Error()
			logging.Error("Panic step failed", "index", i, "id", stepID, "name", step.Name, "action", step.Action, "duration", duration, "error", err)
		} else {
			stepResult.Status = "passed"
			logging.Info("Panic step passed", "index", i, "id", stepID, "name", step.Name, "action", step.Action, "duration", duration)
		}

		results = append(results, stepResult)
		notifyProgress()
	}

	return results, nil
}

func panicActionDelay(actx *action.ActionContext) time.Duration {
	if actx == nil || actx.Panic == nil || actx.Panic.ActionDelay == nil {
		return 0
	}
	if *actx.Panic.ActionDelay <= 0 {
		return 0
	}
	return *actx.Panic.ActionDelay
}
