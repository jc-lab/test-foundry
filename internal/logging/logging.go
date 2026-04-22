// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

// Package logging provides a centralized slog-based logger for test-foundry.
// The log level is controlled by the --verbose flag:
//   - Default: INFO level (important status messages only)
//   - Verbose: DEBUG level (detailed operation logs)
package logging

import (
	"io"
	"log/slog"
	"os"
)

var (
	// Logger is the global structured logger instance.
	Logger *slog.Logger
)

func init() {
	// Default to INFO level until Setup is called
	Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// Setup initializes the global logger with the appropriate level.
// Call this from CLI root after parsing --verbose flag.
func Setup(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))

	slog.SetDefault(Logger)
}

// SetOutput sets a custom writer for the logger (useful for testing).
func SetOutput(w io.Writer, verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	Logger = slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	}))

	slog.SetDefault(Logger)
}

// Convenience wrappers that use the global Logger.

func Debug(msg string, args ...any) {
	Logger.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	Logger.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	Logger.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	Logger.Error(msg, args...)
}
