// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package windows

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jc-lab/test-foundry/internal/guest/windows/transport"
)

const oobeStateQuery = `powershell -Command "$setup=Get-ItemProperty 'HKLM:\SYSTEM\Setup'; $state=Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State'; Write-Output ('SystemSetupInProgress=' + $setup.SystemSetupInProgress); Write-Output ('OOBEInProgress=' + $setup.OOBEInProgress); Write-Output ('ImageState=' + $state.ImageState)"`

// WaitOOBEComplete checks whether Windows OOBE (Out-Of-Box Experience) has completed.
// It polls the registry every 10 seconds until setup is no longer in progress and
// IMAGE_STATE_COMPLETE is observed, or the context is cancelled/timed out.
func WaitOOBEComplete(ctx context.Context, t transport.CommandTransport) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Try immediately before waiting for the first tick.
	if done, err := checkOOBEState(ctx, t); done {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if done, err := checkOOBEState(ctx, t); done {
				return err
			}
		}
	}
}

// checkOOBEState queries the Windows registry for the OOBE state.
func checkOOBEState(ctx context.Context, t transport.CommandTransport) (bool, error) {
	if ctx.Err() != nil {
		return true, ctx.Err()
	}

	stdout, _, _, err := t.RunCommand(ctx, oobeStateQuery)
	if err != nil {
		// Command failure is expected during OOBE; keep polling.
		return false, nil
	}

	complete, err := isOOBEComplete(stdout)
	if err != nil {
		// Parsing failure is treated as "still in progress" to keep polling.
		return false, nil
	}
	return complete, nil
}

func isOOBEComplete(stdout string) (bool, error) {
	values := make(map[string]string)
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	systemSetup, ok := values["SystemSetupInProgress"]
	if !ok {
		return false, fmt.Errorf("missing SystemSetupInProgress")
	}
	oobeInProgress, ok := values["OOBEInProgress"]
	if !ok {
		return false, fmt.Errorf("missing OOBEInProgress")
	}
	imageState, ok := values["ImageState"]
	if !ok {
		return false, fmt.Errorf("missing ImageState")
	}

	if systemSetup == "1" {
		return false, nil
	}
	if oobeInProgress == "1" {
		return false, nil
	}
	if imageState != "IMAGE_STATE_COMPLETE" {
		return false, nil
	}

	return true, nil
}
