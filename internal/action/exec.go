// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"bytes"
	"context"
	"fmt"

	"github.com/jc-lab/test-foundry/internal/logging"
)

// lineWriter buffers writes and logs each complete line with a fixed prefix.
type lineWriter struct {
	prefix string
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (n int, err error) {
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexAny(w.buf, "\r\n")
		if idx < 0 {
			break
		}
		line := w.buf[:idx]
		if len(line) > 0 {
			logging.Info(w.prefix + ": " + string(line))
		}
		w.buf = w.buf[idx+1:]
	}
	return len(p), nil
}

// flush logs any remaining buffered content that had no trailing newline.
func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		logging.Info(w.prefix + ": " + string(w.buf))
		w.buf = w.buf[:0]
	}
}

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

	stdoutW := &lineWriter{prefix: "stdout"}
	stderrW := &lineWriter{prefix: "stderr"}
	result, err := actx.Guest.Exec(ctx, stdoutW, stderrW, p.Cmd, p.Args...)
	stdoutW.flush()
	stderrW.flush()
	if err != nil {
		return fmt.Errorf("exec: command execution failed: %w", err)
	}

	if p.ExpectExitCode != nil {
		if result.ExitCode != *p.ExpectExitCode {
			return fmt.Errorf("exec: expected exit code %d but got %d",
				*p.ExpectExitCode, result.ExitCode)
		}
	}

	return nil
}
