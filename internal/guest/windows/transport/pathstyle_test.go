// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package transport

import (
	"testing"
)

func TestNewSSHTransportUsesWindowsGuestPathStyle(t *testing.T) {
	tp := NewSSHTransport(Config{
		OS: "windows",
	})

	got := tp.pathStyle.Dir(`C:\temp\artifact\result.txt`)
	want := `C:\temp\artifact`
	if got != want {
		t.Fatalf("pathStyle.Dir() = %q, want %q", got, want)
	}
}

func TestNewWinRMTransportUsesWindowsGuestPathStyle(t *testing.T) {
	tp := NewWinRMTransport(Config{
		OS: "windows",
	})

	got := tp.pathStyle.Clean(`C:/temp/artifact/result.txt`)
	want := `C:\temp\artifact\result.txt`
	if got != want {
		t.Fatalf("pathStyle.Clean() = %q, want %q", got, want)
	}
}
