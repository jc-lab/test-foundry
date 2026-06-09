// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/jc-lab/test-foundry/internal/logging"
)

const execStdoutMaxSize = 4 * 1024

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

func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		logging.Info(w.prefix + ": " + string(w.buf))
		w.buf = w.buf[:0]
	}
}

// captureWriter accumulates up to max bytes and silently discards the rest.
type captureWriter struct {
	buf []byte
	max int
}

func (w *captureWriter) Write(p []byte) (int, error) {
	if rem := w.max - len(w.buf); rem > 0 {
		if len(p) > rem {
			p = p[:rem]
		}
		w.buf = append(w.buf, p...)
	}
	return len(p), nil
}

func (w *captureWriter) String() string { return string(w.buf) }

// ExecAction executes a command on the guest via SSH.
type ExecAction struct{}

func (a *ExecAction) Name() string { return "exec" }

func (a *ExecAction) Execute(ctx context.Context, sctx *StepContext, params map[string]any) error {
	var p ExecParams
	if err := DecodeParams(params, &p); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	if p.Cmd == "" {
		return fmt.Errorf("exec: 'cmd' param is required")
	}

	capture := &captureWriter{max: execStdoutMaxSize}
	stdoutW := &lineWriter{prefix: "stdout"}
	stderrW := &lineWriter{prefix: "stderr"}
	stdout := io.MultiWriter(stdoutW, capture)

	result, err := sctx.Guest.Exec(ctx, stdout, stderrW, p.Cmd, p.Args...)
	stdoutW.flush()
	stderrW.flush()

	sctx.SetOutput("stdout", capture.String())

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
