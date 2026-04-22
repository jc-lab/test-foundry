// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package cli

import (
	"runtime"

	"github.com/spf13/cobra"

	"github.com/jc-lab/test-foundry/internal/logging"
	"github.com/jc-lab/test-foundry/internal/qemu"
)

// GlobalFlags holds the common arguments shared across all commands.
type GlobalFlags struct {
	VMName   string // --vm-name: VM context 식별 이름
	WorkDir  string // --workdir: VM context directory root
	QemuPath string // --qemu: QEMU 바이너리 경로
	Headless bool   // --headless: display 없이 VNC만 활성화
	Verbose  bool   // --verbose: 상세 로그 출력

	tools *qemu.Tools
}

// NewRootCommand creates the root cobra command and registers all subcommands.
func NewRootCommand() *cobra.Command {
	flags := &GlobalFlags{}

	cmd := &cobra.Command{
		Use:   "test-foundry",
		Short: "QEMU-based test automation for Windows Driver / UEFI development",
		Long: `test-foundry automates testing of Windows drivers and UEFI applications
using QEMU virtual machines. It provides VM lifecycle management,
snapshot-based test execution, and step-by-step test automation.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			logging.Setup(flags.Verbose)
			flags.resolveTools()
			return nil
		},
	}

	// Register persistent flags
	cmd.PersistentFlags().StringVar(&flags.VMName, "vm-name", "", "VM context name (required)")
	cmd.PersistentFlags().StringVar(&flags.WorkDir, "workdir", ".testfoundry", "VM context directory root")
	cmd.PersistentFlags().StringVar(&flags.QemuPath, "qemu", defaultQemuPath(), "QEMU binary path")
	cmd.PersistentFlags().BoolVar(&flags.Headless, "headless", false, "Headless mode (VNC only, no display)")
	cmd.PersistentFlags().BoolVar(&flags.Verbose, "verbose", false, "Verbose logging")
	_ = cmd.MarkPersistentFlagRequired("vm-name")

	// Register subcommands
	cmd.AddCommand(newVMSetupCommand(flags))
	cmd.AddCommand(newVMDestroyCommand(flags))
	cmd.AddCommand(newTestCommand(flags))
	cmd.AddCommand(newActionCommand(flags))

	return cmd
}

func (f *GlobalFlags) resolveTools() *qemu.Tools {
	if f.tools == nil {
		f.tools = qemu.ResolveTools(f.QemuPath)
	}
	return f.tools
}

// defaultQemuPath returns the default QEMU binary path based on the current OS.
func defaultQemuPath() string {
	if runtime.GOOS == "windows" {
		return "qemu-system-x86_64.exe"
	}
	return "qemu-system-x86_64"
}
