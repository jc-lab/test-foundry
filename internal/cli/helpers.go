// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package cli

import (
	"fmt"

	"github.com/jc-lab/test-foundry/internal/guest"
	"github.com/jc-lab/test-foundry/internal/guest/linux"
	"github.com/jc-lab/test-foundry/internal/guest/windows"
	"github.com/jc-lab/test-foundry/internal/guest/windows/transport"
	"github.com/jc-lab/test-foundry/internal/qemu"
	"github.com/jc-lab/test-foundry/internal/workspace"
)

type guestTransports struct {
	command transport.CommandTransport
	files   transport.FileTransport
}

func buildTransportConfig(vmCfg *workspace.VMConfig) transport.Config {
	return transport.Config{
		OS:       vmCfg.OS,
		Host:     "127.0.0.1",
		Username: vmCfg.Username,
		Password: vmCfg.Password,
		KeyFile:  vmCfg.KeyFile,
		UseTLS:   vmCfg.UseTLS,
	}
}

func createTransportForMethod(vmCfg *workspace.VMConfig, method string) (transport.Connector, error) {
	cfg := buildTransportConfig(vmCfg)
	switch method {
	case "winrm":
		cfg.Port = vmCfg.WinRMPort
		return transport.NewWinRMTransport(cfg), nil
	case "ssh":
		cfg.Port = vmCfg.SSHPort
		return transport.NewSSHTransport(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported transport method %q", method)
	}
}

// createGuestTransports builds the command/file transports from the VM config.
func createGuestTransports(vmCfg *workspace.VMConfig) (*guestTransports, error) {
	execConnector, err := createTransportForMethod(vmCfg, vmCfg.ExecMethod)
	if err != nil {
		return nil, err
	}
	command, ok := execConnector.(transport.CommandTransport)
	if !ok {
		return nil, fmt.Errorf("transport %q does not support command execution", vmCfg.ExecMethod)
	}

	if vmCfg.FileMethod == "" || vmCfg.FileMethod == vmCfg.ExecMethod {
		files, ok := execConnector.(transport.FileTransport)
		if !ok {
			return nil, fmt.Errorf("transport %q does not support file transfer", vmCfg.ExecMethod)
		}
		return &guestTransports{
			command: command,
			files:   files,
		}, nil
	}

	fileConnector, err := createTransportForMethod(vmCfg, vmCfg.FileMethod)
	if err != nil {
		return nil, err
	}
	files, ok := fileConnector.(transport.FileTransport)
	if !ok {
		return nil, fmt.Errorf("transport %q does not support file transfer", vmCfg.FileMethod)
	}
	return &guestTransports{
		command: command,
		files:   files,
	}, nil
}

func createGuest(vmCfg *workspace.VMConfig) (guest.Guest, error) {
	transports, err := createGuestTransports(vmCfg)
	if err != nil {
		return nil, err
	}
	switch vmCfg.OS {
	case "windows":
		return windows.NewWindowsGuest(transports.command, transports.files), nil
	case "linux":
		return linux.NewLinuxGuest(transports.command, transports.files), nil
	default:
		return nil, fmt.Errorf("unsupported guest OS %q", vmCfg.OS)
	}
}

// buildMachineConfig creates a qemu.MachineConfig from the VM config, layout, and global flags.
func buildMachineConfig(globals *GlobalFlags, vmCfg *workspace.VMConfig, layout *workspace.Layout) *qemu.MachineConfig {
	cfg := &qemu.MachineConfig{
		QemuPath:       globals.QemuPath,
		MachineName:    globals.VMName,
		OverlayImage:   layout.OverlayImage(),
		Firmware:       vmCfg.Firmware,
		Memory:         vmCfg.Memory,
		CPUs:           vmCfg.CPUs,
		Headless:       globals.Headless,
		VNCDisplay:     vmCfg.VNCDisplay,
		QMPSocketPath:  layout.QMPSocket(),
		SSHHostPort:    vmCfg.SSHPort,
		SSHGuestPort:   vmCfg.SSHGuestPort,
		WinRMHostPort:  vmCfg.WinRMPort,
		WinRMGuestPort: vmCfg.WinRMGuestPort,
		SerialLog:      layout.SerialLog(),
		TPMEnabled:     vmCfg.TPM,
		TPMSocketPath:  layout.TPMSocket(),
		ExtraArgs:      vmCfg.ExtraArgs,
	}

	// EFI vars: use the local copy in the workspace (not the original)
	if vmCfg.FirmwareVars != "" {
		cfg.FirmwareVars = layout.EFIVars()
	}

	return cfg
}
