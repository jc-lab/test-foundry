// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jc-lab/test-foundry/internal/action"
	"github.com/jc-lab/test-foundry/internal/ipc"
)

// newActionCommand creates the "action" subcommand group.
func newActionCommand(globals *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "action",
		Short: "Execute individual actions against a running VM",
		Long: `Execute individual actions against a running VM session via the IPC daemon.
Each action communicates with the daemon over HTTP to perform operations
on the guest OS or QEMU instance.`,
	}

	cmd.AddCommand(newActionWaitBootCommand(globals))
	cmd.AddCommand(newActionWaitOOBECommand(globals))
	cmd.AddCommand(newActionFileUploadCommand(globals))
	cmd.AddCommand(newActionFileDownloadCommand(globals))
	cmd.AddCommand(newActionExecCommand(globals))
	cmd.AddCommand(newActionScreenshotCommand(globals))
	cmd.AddCommand(newActionShutdownCommand(globals))
	cmd.AddCommand(newActionRebootCommand(globals))
	cmd.AddCommand(newActionDumpCommand(globals))
	cmd.AddCommand(newActionSleepCommand(globals))
	cmd.AddCommand(newActionWaitPanicCommand(globals))

	return cmd
}

// createIPCClient creates an IPC client from the workspace daemon.addr file.
func createIPCClient(globals *GlobalFlags) (*ipc.Client, error) {
	return ipc.NewClientFromWorkspace(globals.WorkDir, globals.VMName)
}

// executeWithParams encodes a typed params struct into map[string]any and calls ExecuteAction.
func executeWithParams(ctx context.Context, client *ipc.Client, actionName string, params any, timeout int) (*ipc.ActionResponse, error) {
	encoded, err := action.EncodeParams(params)
	if err != nil {
		return nil, fmt.Errorf("failed to encode params: %w", err)
	}
	return client.ExecuteAction(ctx, actionName, encoded, timeout)
}

// newActionWaitBootCommand creates the "action wait-boot" subcommand.
func newActionWaitBootCommand(globals *GlobalFlags) *cobra.Command {
	var timeout int
	var params action.WaitBootParams

	cmd := &cobra.Command{
		Use:   "wait-boot",
		Short: "Wait until the guest OS is reachable via SSH",
	}

	cmd.Flags().IntVar(&timeout, "timeout", 120, "Timeout in seconds")
	cmd.Flags().StringVar(&params.RetryInterval, "retry-interval", "5s", "SSH retry interval (e.g. 5s)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		client, err := createIPCClient(globals)
		if err != nil {
			return err
		}
		_, err = executeWithParams(context.Background(), client, "wait-boot", &params, timeout)
		if err != nil {
			return fmt.Errorf("wait-boot failed: %w", err)
		}
		fmt.Println("Guest OS is reachable via SSH.")
		return nil
	}

	return cmd
}

// newActionWaitOOBECommand creates the "action wait-oobe" subcommand.
func newActionWaitOOBECommand(globals *GlobalFlags) *cobra.Command {
	var timeout int

	cmd := &cobra.Command{
		Use:   "wait-oobe",
		Short: "Wait until Windows OOBE is completed",
	}

	cmd.Flags().IntVar(&timeout, "timeout", 600, "Timeout in seconds")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		client, err := createIPCClient(globals)
		if err != nil {
			return err
		}
		_, err = executeWithParams(context.Background(), client, "wait-oobe", &action.WaitOOBEParams{}, timeout)
		if err != nil {
			return fmt.Errorf("wait-oobe failed: %w", err)
		}
		fmt.Println("Windows OOBE completed.")
		return nil
	}

	return cmd
}

// newActionFileUploadCommand creates the "action file-upload" subcommand.
func newActionFileUploadCommand(globals *GlobalFlags) *cobra.Command {
	var params action.FileUploadParams

	cmd := &cobra.Command{
		Use:   "file-upload",
		Short: "Upload a file to the guest via SFTP",
	}

	cmd.Flags().StringVar(&params.Src, "src", "", "Local file path (required)")
	cmd.Flags().StringVar(&params.Dst, "dst", "", "Remote destination path (required)")
	_ = cmd.MarkFlagRequired("src")
	_ = cmd.MarkFlagRequired("dst")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		client, err := createIPCClient(globals)
		if err != nil {
			return err
		}
		if err := client.UploadFile(context.Background(), params.Src, params.Dst); err != nil {
			return fmt.Errorf("file-upload failed: %w", err)
		}
		fmt.Printf("File uploaded: %s -> %s\n", params.Src, params.Dst)
		return nil
	}

	return cmd
}

// newActionFileDownloadCommand creates the "action file-download" subcommand.
func newActionFileDownloadCommand(globals *GlobalFlags) *cobra.Command {
	var params action.FileDownloadParams

	cmd := &cobra.Command{
		Use:   "file-download",
		Short: "Download a file from the guest via SFTP",
	}

	cmd.Flags().StringVar(&params.Src, "src", "", "Remote file path (required)")
	cmd.Flags().StringVar(&params.Dst, "dst", "", "Local destination path (required)")
	_ = cmd.MarkFlagRequired("src")
	_ = cmd.MarkFlagRequired("dst")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		client, err := createIPCClient(globals)
		if err != nil {
			return err
		}
		if err := client.DownloadFile(context.Background(), params.Src, params.Dst); err != nil {
			return fmt.Errorf("file-download failed: %w", err)
		}
		fmt.Printf("File downloaded: %s -> %s\n", params.Src, params.Dst)
		return nil
	}

	return cmd
}

// newActionExecCommand creates the "action exec" subcommand.
func newActionExecCommand(globals *GlobalFlags) *cobra.Command {
	var params action.ExecParams
	var timeout int
	var expectExitCode int

	cmd := &cobra.Command{
		Use:   "exec",
		Short: "Execute a command on the guest via SSH",
	}

	cmd.Flags().StringVar(&params.Cmd, "cmd", "", "Command to execute (required)")
	cmd.Flags().IntVar(&timeout, "timeout", 60, "Timeout in seconds")
	cmd.Flags().IntVar(&expectExitCode, "expect-exit-code", 0, "Expected exit code")
	_ = cmd.MarkFlagRequired("cmd")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		client, err := createIPCClient(globals)
		if err != nil {
			return err
		}

		if len(args) > 0 {
			params.Args = args
		}
		if cmd.Flags().Changed("expect-exit-code") {
			params.ExpectExitCode = &expectExitCode
		}

		resp, err := executeWithParams(context.Background(), client, "exec", &params, timeout)
		if err != nil {
			return fmt.Errorf("exec failed: %w", err)
		}

		if resp != nil && resp.Data != nil {
			dataBytes, jsonErr := json.Marshal(resp.Data)
			if jsonErr == nil {
				var execData ipc.ExecResponseData
				if json.Unmarshal(dataBytes, &execData) == nil {
					if execData.Stdout != "" {
						fmt.Print(execData.Stdout)
					}
					if execData.Stderr != "" {
						fmt.Fprintf(cmd.ErrOrStderr(), "%s", execData.Stderr)
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "Exit code: %d\n", execData.ExitCode)
					return nil
				}
			}
		}

		fmt.Println("Command executed successfully.")
		return nil
	}

	return cmd
}

// newActionScreenshotCommand creates the "action screenshot" subcommand.
func newActionScreenshotCommand(globals *GlobalFlags) *cobra.Command {
	var params action.ScreenshotParams

	cmd := &cobra.Command{
		Use:   "screenshot",
		Short: "Capture a screenshot via VNC",
	}

	cmd.Flags().StringVar(&params.Output, "output", "", "Screenshot output path (PNG) (required)")
	_ = cmd.MarkFlagRequired("output")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		client, err := createIPCClient(globals)
		if err != nil {
			return err
		}
		if err := client.Screenshot(context.Background(), params.Output); err != nil {
			return fmt.Errorf("screenshot failed: %w", err)
		}
		fmt.Printf("Screenshot saved to %s\n", params.Output)
		return nil
	}

	return cmd
}

// newActionShutdownCommand creates the "action shutdown" subcommand.
func newActionShutdownCommand(globals *GlobalFlags) *cobra.Command {
	var timeout int

	cmd := &cobra.Command{
		Use:   "shutdown",
		Short: "Gracefully shut down the guest",
	}

	cmd.Flags().IntVar(&timeout, "timeout", 120, "Timeout in seconds")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		client, err := createIPCClient(globals)
		if err != nil {
			return err
		}
		_, err = executeWithParams(context.Background(), client, "shutdown", &action.ShutdownParams{}, timeout)
		if err != nil {
			return fmt.Errorf("shutdown failed: %w", err)
		}
		fmt.Println("Guest shutdown initiated.")
		return nil
	}

	return cmd
}

// newActionRebootCommand creates the "action reboot" subcommand.
func newActionRebootCommand(globals *GlobalFlags) *cobra.Command {
	var timeout int

	cmd := &cobra.Command{
		Use:   "reboot",
		Short: "Reboot the guest",
	}

	cmd.Flags().IntVar(&timeout, "timeout", 120, "Timeout in seconds")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		client, err := createIPCClient(globals)
		if err != nil {
			return err
		}
		_, err = executeWithParams(context.Background(), client, "reboot", &action.RebootParams{}, timeout)
		if err != nil {
			return fmt.Errorf("reboot failed: %w", err)
		}
		fmt.Println("Guest reboot initiated.")
		return nil
	}

	return cmd
}

// newActionDumpCommand creates the "action dump" subcommand.
func newActionDumpCommand(globals *GlobalFlags) *cobra.Command {
	var params action.DumpParams
	var timeout int

	cmd := &cobra.Command{
		Use:   "dump",
		Short: "Dump guest memory via QMP",
	}

	cmd.Flags().StringVar(&params.Output, "output", "", "Memory dump output path (required)")
	cmd.Flags().IntVar(&timeout, "timeout", 120, "Timeout in seconds")
	_ = cmd.MarkFlagRequired("output")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		client, err := createIPCClient(globals)
		if err != nil {
			return err
		}
		_, err = executeWithParams(context.Background(), client, "dump", &params, timeout)
		if err != nil {
			return fmt.Errorf("dump failed: %w", err)
		}
		fmt.Printf("Memory dump saved to %s\n", params.Output)
		return nil
	}

	return cmd
}

// newActionSleepCommand creates the "action sleep" subcommand.
func newActionSleepCommand(globals *GlobalFlags) *cobra.Command {
	var params action.SleepParams

	cmd := &cobra.Command{
		Use:   "sleep",
		Short: "Wait for a specified duration",
	}

	cmd.Flags().StringVar(&params.Duration, "duration", "", "Duration to wait (e.g. 10s, 1m30s) (required)")
	_ = cmd.MarkFlagRequired("duration")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		client, err := createIPCClient(globals)
		if err != nil {
			return err
		}
		_, err = executeWithParams(context.Background(), client, "sleep", &params, 0)
		if err != nil {
			return fmt.Errorf("sleep failed: %w", err)
		}
		fmt.Printf("Slept for %s\n", params.Duration)
		return nil
	}

	return cmd
}

// newActionWaitPanicCommand creates the "action wait-panic" subcommand.
func newActionWaitPanicCommand(globals *GlobalFlags) *cobra.Command {
	var timeout int

	cmd := &cobra.Command{
		Use:   "wait-panic",
		Short: "Wait for a pvpanic event from the guest",
	}

	cmd.Flags().IntVar(&timeout, "timeout", 300, "Timeout in seconds")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		client, err := createIPCClient(globals)
		if err != nil {
			return err
		}
		_, err = executeWithParams(context.Background(), client, "wait-panic", &action.WaitPanicParams{}, timeout)
		if err != nil {
			return fmt.Errorf("wait-panic failed: %w", err)
		}
		fmt.Println("Guest panic detected.")
		return nil
	}

	return cmd
}
