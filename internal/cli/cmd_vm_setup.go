// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/jc-lab/test-foundry/internal/action"
	"github.com/jc-lab/test-foundry/internal/config"
	"github.com/jc-lab/test-foundry/internal/executor"
	"github.com/jc-lab/test-foundry/internal/ipc"
	"github.com/jc-lab/test-foundry/internal/logging"
	"github.com/jc-lab/test-foundry/internal/qemu"
	"github.com/jc-lab/test-foundry/internal/workspace"
)

// vmSetupFlags holds flags specific to the vm-setup command.
type vmSetupFlags struct {
	ImagePath  string // --image: Image 정의 YAML 경로
	TPM        bool   // --tpm: swtpm TPM 2.0 에뮬레이션 활성화
	NoShutdown bool   // --no-shutdown: 디버깅용, QEMU를 자동 종료하지 않음
}

// newVMSetupCommand creates the "vm-setup" subcommand.
func newVMSetupCommand(globals *GlobalFlags) *cobra.Command {
	flags := &vmSetupFlags{}

	cmd := &cobra.Command{
		Use:   "vm-setup",
		Short: "Create VM context and prepare a snapshot",
		Long: `Create a new VM context directory, boot from the image definition,
execute setup steps (wait for boot, OOBE, etc.), and save a snapshot
for later test execution.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVMSetup(globals, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ImagePath, "image", "", "Path to image definition YAML (required)")
	cmd.Flags().BoolVar(&flags.TPM, "tpm", false, "Enable TPM 2.0 emulation via swtpm")
	cmd.Flags().BoolVar(&flags.NoShutdown, "no-shutdown", false, "Debugging: do not shutdown QEMU on step failure, wait for manual exit")
	_ = cmd.MarkFlagRequired("image")

	return cmd
}

// runVMSetup executes the vm-setup workflow.
func runVMSetup(globals *GlobalFlags, flags *vmSetupFlags) error {
	ctx := context.Background()

	// 1. Load image config
	imgCfg, err := config.LoadImageConfig(flags.ImagePath)
	if err != nil {
		return fmt.Errorf("failed to load image config: %w", err)
	}
	logging.Info("Loaded image config", "name", imgCfg.Name, "os", imgCfg.OS)

	// 2. Create workspace layout and context
	layout := workspace.NewLayout(globals.WorkDir, globals.VMName)

	// Determine guest ports for enabled connection methods.
	sshGuestPort := 0
	if imgCfg.Connection.ExecMethod == "ssh" || imgCfg.Connection.FileMethod == "ssh" {
		sshGuestPort = imgCfg.Connection.SSHPort
	}
	winrmGuestPort := 0
	if imgCfg.Connection.ExecMethod == "winrm" || imgCfg.Connection.FileMethod == "winrm" {
		winrmGuestPort = imgCfg.Connection.WinRMPort
	}

	vmCfg := &workspace.VMConfig{
		// Image metadata
		ImagePath: imgCfg.QEMU.Image,
		ImageName: imgCfg.Name,
		OS:        imgCfg.OS,

		// QEMU settings
		Firmware:     imgCfg.QEMU.Firmware,
		FirmwareVars: imgCfg.QEMU.FirmwareVars,
		Memory:       imgCfg.QEMU.Memory,
		CPUs:         imgCfg.QEMU.CPUs,
		ExtraArgs:    imgCfg.QEMU.ExtraArgs,

		// Features
		TPM: flags.TPM,

		// Connection
		ExecMethod:     imgCfg.Connection.ExecMethod,
		FileMethod:     imgCfg.Connection.FileMethod,
		Username:       imgCfg.Connection.Username,
		Password:       imgCfg.Connection.Password,
		KeyFile:        imgCfg.Connection.KeyFile,
		UseTLS:         imgCfg.Connection.UseTLS,
		SSHGuestPort:   sshGuestPort,
		WinRMGuestPort: winrmGuestPort,
	}

	if err := workspace.CreateContext(layout, globals.QemuPath, vmCfg, imgCfg.QEMU.Image, imgCfg.QEMU.FirmwareVars); err != nil {
		return fmt.Errorf("failed to create VM context: %w", err)
	}
	logging.Info("Created VM context", "path", layout.Root)

	// Track resources for cleanup
	var tpmProc *qemu.TPMProcess
	var machine *qemu.Machine
	var ipcServer *ipc.Server

	// Cleanup function
	cleanup := func() {
		if ipcServer != nil {
			_ = ipcServer.Shutdown(context.Background())
		}
		if machine != nil {
			_ = machine.Kill()
		}
		if tpmProc != nil {
			_ = tpmProc.Stop()
		}
	}
	defer func() {
		// Only cleanup on error (deferred cleanup);
		// on success we shut down gracefully in the main flow
	}()

	// 3. Start TPM if requested
	if flags.TPM {
		tpmCfg := &qemu.TPMConfig{
			StateDir:   layout.TPMDir(),
			SocketPath: layout.TPMSocket(),
			LogPath:    layout.TPMLog(),
		}
		tpmProc, err = qemu.StartTPM(ctx, tpmCfg)
		if err != nil {
			cleanup()
			return fmt.Errorf("failed to start TPM: %w", err)
		}
		logging.Info("TPM started")
	}

	// 4. Build QEMU machine config and start
	machineCfg := buildMachineConfig(globals, vmCfg, layout)

	machine, err = qemu.StartMachine(ctx, machineCfg)
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to start QEMU: %w", err)
	}
	logging.Info("QEMU started", "ssh_port", vmCfg.SSHPort, "vnc_display", vmCfg.VNCDisplay)

	// Monitor for QEMU exit — cancel context when QEMU terminates for any reason.
	// This ensures all ongoing operations (SSH, steps, etc.) are immediately aborted.
	qemuCtx, qemuCancel := context.WithCancelCause(ctx)
	defer qemuCancel(nil)
	go func() {
		<-machine.Done()
		exitErr := machine.ExitError()
		if exitErr != nil {
			logging.Error("QEMU process exited abnormally", "error", exitErr)
			qemuCancel(fmt.Errorf("QEMU exited abnormally: %w", exitErr))
		} else {
			logging.Info("QEMU process exited")
			qemuCancel(fmt.Errorf("QEMU process exited"))
		}
	}()
	ctx = qemuCtx

	// 5. Create guest and action context
	guest, err := createGuest(vmCfg)
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to create guest: %w", err)
	}

	registry := action.NewRegistry()
	actx := &action.ActionContext{
		Machine: machine,
		Guest:   guest,
		WorkDir: layout.Root,
	}

	// 6. Start IPC server
	ipcServer, err = ipc.StartServer(ctx, registry, actx)
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to start IPC server: %w", err)
	}
	logging.Info("IPC server started", "addr", ipcServer.Addr())

	// 7. Write daemon files
	if err := os.WriteFile(layout.DaemonAddr(), []byte(ipcServer.Addr()), 0644); err != nil {
		cleanup()
		return fmt.Errorf("failed to write daemon.addr: %w", err)
	}
	if err := os.WriteFile(layout.DaemonPID(), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		cleanup()
		return fmt.Errorf("failed to write daemon.pid: %w", err)
	}
	if err := os.WriteFile(layout.SSHPort(), []byte(strconv.Itoa(vmCfg.SSHPort)), 0644); err != nil {
		cleanup()
		return fmt.Errorf("failed to write ssh.port: %w", err)
	}
	vncPort := qemu.VNCPort(vmCfg.VNCDisplay)
	if err := os.WriteFile(layout.VNCPort(), []byte(strconv.Itoa(vncPort)), 0644); err != nil {
		cleanup()
		return fmt.Errorf("failed to write vnc.port: %w", err)
	}

	// 8. Run setup steps
	runner := executor.NewRunner(registry, actx)
	result, err := runner.RunSteps(ctx, imgCfg.Setup.Steps, nil)
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to run setup steps: %w", err)
	}

	// Check for step failures
	var setupFailed bool
	for _, step := range result.Steps {
		if step.Status == "failed" {
			setupFailed = true
			logging.Error("Setup step failed", "action", step.Action, "error", step.Error)
			break
		}
	}

	if setupFailed {
		if flags.NoShutdown {
			logging.Warn("Setup failed but --no-shutdown is set. Waiting for QEMU to exit manually...")
			fmt.Println("Setup failed. QEMU is still running for debugging.")
			fmt.Println("Connect via VNC, SSH, or close the QEMU window when done.")
			<-machine.Done()
			logging.Info("QEMU exited")
			cleanup()
			return fmt.Errorf("setup failed (QEMU was left running for debugging)")
		} else {
			cleanup()
			return fmt.Errorf("setup step failed")
		}
	}

	logging.Info("All setup steps completed successfully")

	// 9. Shutdown QEMU before taking snapshot (pflash does not support live snapshots)
	if machine.IsRunning() {
		shutdownCtx := context.Background()
		if err := machine.Shutdown(shutdownCtx); err != nil {
			logging.Warn("Graceful shutdown failed, killing", "error", err)
			_ = machine.Kill()
		}
	}
	logging.Info("QEMU shut down")

	// Cleanup IPC and TPM before snapshot (QEMU is stopped)
	_ = ipcServer.Shutdown(context.Background())
	ipcServer = nil
	if tpmProc != nil {
		_ = tpmProc.Stop()
		tpmProc = nil
	}
	_ = os.Remove(layout.DaemonAddr())
	_ = os.Remove(layout.DaemonPID())

	// 10. Save snapshot on disk (qemu-img + efivars/tpm copy)
	snapPaths := &qemu.SnapshotPaths{
		QemuImgPath:  qemu.ResolveQemuImg(globals.QemuPath),
		OverlayImage: layout.OverlayImage(),
		EFIVars:      layout.EFIVars(),
		SnapshotDir:  layout.SnapshotDir(),
		SnapshotName: "",
	}
	if flags.TPM {
		snapPaths.TPMStateDir = layout.TPMDir()
	}
	if imgCfg.QEMU.FirmwareVars == "" {
		snapPaths.EFIVars = ""
	}

	if err := qemu.SaveSnapshot(snapPaths); err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	logging.Info("VM setup complete", "vm_name", globals.VMName)
	return nil
}
