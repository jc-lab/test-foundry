// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jc-lab/test-foundry/internal/logging"

	"github.com/jc-lab/test-foundry/internal/workspace"
)

// newVMDestroyCommand creates the "vm-destroy" subcommand.
func newVMDestroyCommand(globals *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vm-destroy",
		Short: "Destroy VM context directory",
		Long:  `Remove the entire VM context directory including overlay images, logs, and sockets.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVMDestroy(globals)
		},
	}

	return cmd
}

// runVMDestroy executes the vm-destroy workflow.
func runVMDestroy(globals *GlobalFlags) error {
	layout := workspace.NewLayout(globals.WorkDir, globals.VMName)

	if err := workspace.DestroyContext(layout); err != nil {
		return fmt.Errorf("failed to destroy VM context: %w", err)
	}

	logging.Info("VM context destroyed", "vm_name", globals.VMName)
	fmt.Printf("VM context %q destroyed successfully.\n", globals.VMName)
	return nil
}
