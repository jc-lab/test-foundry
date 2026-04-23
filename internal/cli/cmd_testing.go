// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jc-lab/test-foundry/internal/action"
	"github.com/jc-lab/test-foundry/internal/config"
	"github.com/jc-lab/test-foundry/internal/executor"
	"github.com/jc-lab/test-foundry/internal/ipc"
	"github.com/jc-lab/test-foundry/internal/logging"
	"github.com/jc-lab/test-foundry/internal/preboot"
	"github.com/jc-lab/test-foundry/internal/qemu"
	"github.com/jc-lab/test-foundry/internal/workspace"
)

// testFlags holds flags specific to the test command.
type testFlags struct {
	TestPath        string // --test: Test 정의 YAML 경로
	OutputDir       string // --output: 테스트 결과 출력 디렉터리
	NoShutdown      bool   // --no-shutdown: 디버깅용, 실패 시 QEMU를 자동 종료하지 않음
	KeepTestContext bool   // --keep-test-context: 테스트 종료 후 test context 디렉터리 유지
}

// newTestCommand creates the "test" subcommand.
func newTestCommand(globals *GlobalFlags) *cobra.Command {
	flags := &testFlags{}

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run tests against a VM snapshot",
		Long: `Restore a VM snapshot and execute the test steps defined in the
test YAML file. Panic events are detected via pvpanic and trigger
the panic steps for diagnostics collection.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTest(globals, flags)
		},
	}

	cmd.Flags().StringVar(&flags.TestPath, "test", "", "Path to test definition YAML (required)")
	cmd.Flags().StringVar(&flags.OutputDir, "output", "", "Test result output directory (required)")
	cmd.Flags().BoolVar(&flags.NoShutdown, "no-shutdown", false, "Debugging: do not shutdown QEMU on test failure, wait for manual exit")
	cmd.Flags().BoolVar(&flags.KeepTestContext, "keep-test-context", false, "Keep the per-test context directory after the test finishes")
	_ = cmd.MarkFlagRequired("test")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

// runTest executes the test workflow.
func runTest(globals *GlobalFlags, flags *testFlags) error {
	ctx := context.Background()
	tools := globals.resolveTools()

	// 1. Load existing VM context
	layout := workspace.NewLayout(globals.WorkDir, globals.VMName)
	vmCfg, err := workspace.LoadContext(layout)
	if err != nil {
		return fmt.Errorf("failed to load VM context: %w", err)
	}
	logging.Info("Loaded VM context", "image", vmCfg.ImageName, "os", vmCfg.OS, "tpm", vmCfg.TPM)

	// 2. Load test config
	testCfg, err := config.LoadTestConfig(flags.TestPath)
	if err != nil {
		return fmt.Errorf("failed to load test config: %w", err)
	}
	logging.Info("Loaded test config", "name", testCfg.Name, "steps", len(testCfg.Steps))

	testContextName := buildTestContextName(testCfg.Name)
	testLayout, err := workspace.CreateTestContext(layout, testContextName, tools, vmCfg)
	if err != nil {
		return fmt.Errorf("failed to create test context: %w", err)
	}
	if err := workspace.AllocateRuntimeResources(vmCfg); err != nil {
		return fmt.Errorf("failed to allocate runtime resources: %w", err)
	}
	logging.Info("Created test context", "path", testLayout.Root)
	defer func() {
		if flags.KeepTestContext {
			logging.Info("Keeping test context", "path", testLayout.Root)
			return
		}
		if err := os.RemoveAll(testLayout.Root); err != nil {
			logging.Warn("Failed to remove test context", "path", testLayout.Root, "error", err)
			return
		}
		logging.Info("Removed test context", "path", testLayout.Root)
	}()

	if len(testCfg.Preboot.Steps) > 0 {
		registry := preboot.NewRegistry()
		actx := &preboot.ActionContext{
			WorkDir: testLayout.Root,
			TestDir: filepath.Dir(flags.TestPath),
		}
		runner := preboot.NewRunner(registry, actx)
		result, err := runner.RunSteps(ctx, testCfg.Preboot.Steps)
		if err != nil {
			return fmt.Errorf("failed to run test preboot steps: %w", err)
		}
		for _, step := range result.Steps {
			if step.Status == "failed" {
				return fmt.Errorf("test preboot step failed: %s: %s", step.Action, step.Error)
			}
		}
		logging.Info("All test preboot steps completed successfully")
	}

	// 3. Create output directory
	if err := os.MkdirAll(flags.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Track resources for cleanup
	var tpmProc *qemu.TPMProcess
	var machine *qemu.Machine
	var ipcServer *ipc.Server

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
		// Remove daemon files
		_ = os.Remove(testLayout.DaemonAddr())
		_ = os.Remove(testLayout.DaemonPID())
	}

	// 4. Start TPM if enabled in context
	if vmCfg.TPM {
		tpmCfg := &qemu.TPMConfig{
			StateDir:   testLayout.TPMDir(),
			SocketPath: testLayout.TPMSocket(),
			LogPath:    testLayout.TPMLog(),
		}
		tpmProc, err = qemu.StartTPM(ctx, tpmCfg)
		if err != nil {
			cleanup()
			return fmt.Errorf("failed to start TPM: %w", err)
		}
		logging.Info("TPM started")
	}

	// 5. Build MachineConfig and start QEMU
	machineCfg := buildMachineConfig(globals, vmCfg, testLayout)

	machine, err = qemu.StartMachine(ctx, machineCfg)
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to start QEMU: %w", err)
	}
	logging.Info("QEMU started", "ssh_port", vmCfg.SSHPort, "vnc_display", vmCfg.VNCDisplay, "test_context", testContextName)

	// Monitor for QEMU exit — cancel context when QEMU terminates for any reason.
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

	// 6. Create guest and action context
	guest, err := createGuest(vmCfg)
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to create guest: %w", err)
	}

	registry := action.NewRegistry()
	actx := &action.ActionContext{
		Machine: machine,
		Guest:   guest,
		WorkDir: testLayout.Root,
		TestDir: filepath.Dir(flags.TestPath),
	}

	// 7. Start IPC server
	ipcServer, err = ipc.StartServer(ctx, registry, actx)
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to start IPC server: %w", err)
	}
	logging.Info("IPC server started", "addr", ipcServer.Addr())

	// Write daemon files
	if err := os.WriteFile(testLayout.DaemonAddr(), []byte(ipcServer.Addr()), 0644); err != nil {
		cleanup()
		return fmt.Errorf("failed to write daemon.addr: %w", err)
	}
	if err := os.WriteFile(testLayout.DaemonPID(), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		cleanup()
		return fmt.Errorf("failed to write daemon.pid: %w", err)
	}
	if err := os.WriteFile(testLayout.SSHPort(), []byte(strconv.Itoa(vmCfg.SSHPort)), 0644); err != nil {
		cleanup()
		return fmt.Errorf("failed to write ssh.port: %w", err)
	}
	vncPort := qemu.VNCPort(vmCfg.VNCDisplay)
	if err := os.WriteFile(testLayout.VNCPort(), []byte(strconv.Itoa(vncPort)), 0644); err != nil {
		cleanup()
		return fmt.Errorf("failed to write vnc.port: %w", err)
	}

	// 8. Start PanicHandler
	panicHandler := executor.NewPanicHandler(machine)
	panicHandler.Start(ctx)

	// 9. Run test steps with panic detection
	runner := executor.NewRunner(registry, actx)
	result, err := runner.RunSteps(ctx, testCfg.Steps, panicHandler.PanicCh())
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to run test steps: %w", err)
	}

	// 10. If panic detected, run panic steps
	if result.PanicDetected && len(testCfg.Panic.Steps) > 0 {
		logging.Warn("Panic detected, running panic steps")
		panicResults, panicErr := runner.RunPanicSteps(ctx, testCfg.Panic.Steps)
		if panicErr != nil {
			logging.Warn("Panic steps execution error", "error", panicErr)
		}
		result.PanicSteps = panicResults
	}

	// 11. Write test result to output directory
	resultPath := filepath.Join(flags.OutputDir, "test-result.json")
	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to marshal test result: %w", err)
	}
	if err := os.WriteFile(resultPath, resultData, 0644); err != nil {
		cleanup()
		return fmt.Errorf("failed to write test result: %w", err)
	}
	logging.Info("Test result written", "path", resultPath)

	// Count test results
	passed := 0
	failed := 0
	skipped := 0
	for _, s := range result.Steps {
		switch s.Status {
		case "passed":
			passed++
		case "failed":
			failed++
		case "skipped":
			skipped++
		}
	}

	// Check if test failed
	testFailed := (failed > 0 || result.PanicDetected)

	// 12. Handle shutdown based on test result and --no-shutdown flag
	if testFailed && flags.NoShutdown {
		logging.Warn("Test failed but --no-shutdown is set. Waiting for QEMU to exit manually...")
		fmt.Printf("Test %q failed: %d passed, %d failed, %d skipped\n", testCfg.Name, passed, failed, skipped)
		if result.PanicDetected {
			fmt.Println("WARNING: Guest panic was detected during test execution.")
		}
		fmt.Println("QEMU is still running for debugging.")
		fmt.Println("Connect via VNC, SSH, or close the QEMU window when done.")
		<-machine.Done()
		logging.Info("QEMU exited")
		cleanup()
		return fmt.Errorf("test failed (QEMU was left running for debugging)")
	}

	// Graceful shutdown (skip if QEMU already exited)
	if machine.IsRunning() {
		shutdownCtx := context.Background()
		if err := machine.Shutdown(shutdownCtx); err != nil {
			logging.Warn("Graceful shutdown failed", "error", err)
			_ = machine.Kill()
		}
	}
	logging.Info("QEMU shut down")

	// Cleanup IPC and TPM
	_ = ipcServer.Shutdown(context.Background())
	if tpmProc != nil {
		_ = tpmProc.Stop()
	}

	// Remove daemon files
	_ = os.Remove(testLayout.DaemonAddr())
	_ = os.Remove(testLayout.DaemonPID())

	// Print summary
	fmt.Printf("Test %q complete: %d passed, %d failed, %d skipped\n", testCfg.Name, passed, failed, skipped)
	if result.PanicDetected {
		fmt.Println("WARNING: Guest panic was detected during test execution.")
	}

	if testFailed {
		return fmt.Errorf("test %q failed", testCfg.Name)
	}

	return nil
}

func buildTestContextName(testName string) string {
	sanitized := strings.ToLower(testName)
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	sanitized = strings.ReplaceAll(sanitized, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, "\\", "-")
	if sanitized == "" {
		sanitized = "test"
	}
	return fmt.Sprintf("%s-%d", sanitized, time.Now().UnixNano())
}
