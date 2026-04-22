// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"fmt"
	"runtime"
)

// MachineConfig holds the full QEMU machine configuration used to build command-line arguments.
type MachineConfig struct {
	QemuPath       string   `json:"qemu_path"`
	MachineName    string   `json:"machine_name"`
	OverlayImage   string   `json:"overlay_image"`
	Firmware       string   `json:"firmware,omitempty"`
	FirmwareVars   string   `json:"firmware_vars,omitempty"`
	Memory         string   `json:"memory"`
	CPUs           int      `json:"cpus"`
	Headless       bool     `json:"headless"`
	VNCDisplay     int      `json:"vnc_display"`
	QMPSocketPath  string   `json:"qmp_socket_path"`
	SSHHostPort    int      `json:"ssh_host_port,omitempty"`
	SSHGuestPort   int      `json:"ssh_guest_port,omitempty"`
	WinRMHostPort  int      `json:"winrm_host_port,omitempty"`  // WinRM host forwarding port (0 = disabled)
	WinRMGuestPort int      `json:"winrm_guest_port,omitempty"` // WinRM guest port (default 5985)
	SerialLog      string   `json:"serial_log,omitempty"`
	TPMEnabled     bool     `json:"tpm_enabled"`
	TPMSocketPath  string   `json:"tpm_socket_path,omitempty"`
	ExtraArgs      []string `json:"extra_args,omitempty"`
}

// BuildArgs builds the QEMU command-line arguments from the MachineConfig.
func (c *MachineConfig) BuildArgs() []string {
	var args []string

	// 1. Machine type and accelerator
	args = append(args, "-machine", fmt.Sprintf("q35,accel=%s", detectAccelerator()))

	// 2. Memory and CPU
	args = append(args, "-m", c.Memory)
	args = append(args, "-cpu", "host")
	args = append(args, "-smp", fmt.Sprintf("%d", c.CPUs))

	args = append(args, "-monitor", "vc")

	// 3. Storage controller (virtio-scsi) + disk drive
	args = append(args, "-device", "virtio-scsi-pci,id=scsi0")
	args = append(args, "-drive", fmt.Sprintf("file=%s,format=qcow2,if=none,id=drive0", c.OverlayImage))
	args = append(args, "-device", "scsi-hd,drive=drive0,bus=scsi0.0")

	// 4. UEFI firmware (pflash)
	if c.Firmware != "" {
		args = append(args, "-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", c.Firmware))
	}
	if c.FirmwareVars != "" {
		args = append(args, "-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", c.FirmwareVars))
	}

	// 5. pvpanic device & vmcoreinfo device
	args = append(args, "-device", "pvpanic")
	args = append(args, "-device", "vmcoreinfo")

	// 6. Display
	if c.Headless {
		args = append(args, "-display", "none")
	} else {
		args = append(args, "-display", "gtk")
	}

	// 7. VNC (always enabled for screenshots)
	args = append(args, "-vnc", fmt.Sprintf(":%d", c.VNCDisplay))

	// 8. QMP socket
	if runtime.GOOS == "windows" {
		// Windows: use TCP instead of UNIX socket
		args = append(args, "-qmp", fmt.Sprintf("tcp:127.0.0.1:%s,server,nowait", c.QMPSocketPath))
	} else {
		args = append(args, "-qmp", fmt.Sprintf("unix:%s,server,nowait", c.QMPSocketPath))
	}

	// 9. Network (port forwarding)
	var hostfwd string
	if c.SSHHostPort > 0 && c.SSHGuestPort > 0 {
		hostfwd = fmt.Sprintf("hostfwd=tcp::%d-:%d", c.SSHHostPort, c.SSHGuestPort)
	}
	if c.WinRMHostPort > 0 && c.WinRMGuestPort > 0 {
		if hostfwd != "" {
			hostfwd += ","
		}
		hostfwd += fmt.Sprintf("hostfwd=tcp::%d-:%d", c.WinRMHostPort, c.WinRMGuestPort)
	}
	args = append(args, "-device", "virtio-net-pci,netdev=net0")
	if hostfwd != "" {
		args = append(args, "-netdev", fmt.Sprintf("user,id=net0,%s", hostfwd))
	} else {
		args = append(args, "-netdev", "user,id=net0")
	}

	// 10. Serial console
	if c.SerialLog != "" {
		args = append(args, "-serial", fmt.Sprintf("file:%s", c.SerialLog))
	}

	// 11. TPM device
	if c.TPMEnabled && c.TPMSocketPath != "" {
		args = append(args, "-chardev", fmt.Sprintf("socket,id=chrtpm,path=%s", c.TPMSocketPath))
		args = append(args, "-tpmdev", "emulator,id=tpm0,chardev=chrtpm")
		args = append(args, "-device", "tpm-tis,tpmdev=tpm0")
	}

	// 12. Extra args from image YAML
	args = append(args, c.ExtraArgs...)

	return args
}

// detectAccelerator returns the appropriate QEMU accelerator for the current OS.
func detectAccelerator() string {
	switch runtime.GOOS {
	case "linux":
		return "kvm"
	case "windows":
		return "whpx"
	default:
		return "tcg"
	}
}
