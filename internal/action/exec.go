// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"

	"github.com/jc-lab/test-foundry/internal/logging"
)

// ExecAction executes a command on the guest via SSH.
type ExecAction struct{}

func (a *ExecAction) Name() string { return "exec" }

func (a *ExecAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p ExecParams
	if err := DecodeParams(params, &p); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	if p.Cmd == "" {
		return fmt.Errorf("exec: 'cmd' param is required")
	}

	result, err := actx.Guest.Exec(ctx, p.Cmd, p.Args...)
	if err != nil {
		return fmt.Errorf("exec: command execution failed: %w", err)
	}

	logging.Debug("Command executed", "cmd", p.Cmd, "args", p.Args, "exit_code", result.ExitCode)
	if result.Stdout != "" {
		logging.Debug("Command stdout", "cmd", p.Cmd, "stdout", result.Stdout)
	}
	if result.Stderr != "" {
		logging.Debug("Command stderr", "cmd", p.Cmd, "stderr", result.Stderr)
	}

	if p.ExpectExitCode != nil {
		if result.ExitCode != *p.ExpectExitCode {
			return fmt.Errorf("exec: expected exit code %d but got %d\nstdout: %s\nstderr: %s",
				*p.ExpectExitCode, result.ExitCode, result.Stdout, result.Stderr)
		}
	}

	return nil
}
