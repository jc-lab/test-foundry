// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"runtime"
	"strings"
	"testing"
)

// --- TestBuildArgs_Basic ---

func TestBuildArgs_Basic(t *testing.T) {
	cfg := &MachineConfig{
		QemuPath:      "/usr/bin/qemu-system-x86_64",
		MachineName:   "test-vm",
		OverlayImage:  "/work/overlay.qcow2",
		Memory:        "4G",
		CPUs:          2,
		Headless:      true,
		VNCDisplay:    1,
		QMPSocketPath: "/work/qmp.sock",
		SSHHostPort:   2222,
		SSHGuestPort:  22,
		SerialLog:     "/work/serial.log",
	}

	args := cfg.BuildArgs()
	argStr := strings.Join(args, " ")

	// Machine type with accelerator
	expectedAccel := detectAccelerator()
	assertContainsArg(t, args, "-machine", "q35,accel="+expectedAccel)

	// Memory
	assertContainsArg(t, args, "-m", "4G")

	// CPU
	assertContainsArg(t, args, "-smp", "2")

	// virtio-scsi controller
	assertContainsArg(t, args, "-device", "virtio-scsi-pci,id=scsi0")

	// Drive with overlay (via virtio-scsi)
	if !strings.Contains(argStr, "file=/work/overlay.qcow2,format=qcow2,if=none,id=drive0") {
		t.Errorf("expected overlay drive arg, got: %s", argStr)
	}
	assertContainsArg(t, args, "-device", "scsi-hd,drive=drive0,bus=scsi0.0")

	// pvpanic device
	assertContainsArg(t, args, "-device", "pvpanic")

	// VNC
	assertContainsArg(t, args, "-vnc", ":1")

	// QMP socket
	if runtime.GOOS == "windows" {
		if !strings.Contains(argStr, "tcp:127.0.0.1:/work/qmp.sock,server,nowait") {
			t.Errorf("expected QMP TCP arg on windows, got: %s", argStr)
		}
	} else {
		if !strings.Contains(argStr, "unix:/work/qmp.sock,server,nowait") {
			t.Errorf("expected QMP unix socket arg, got: %s", argStr)
		}
	}

	// Network device
	assertContainsArg(t, args, "-device", "virtio-net-pci,netdev=net0")

	// SSH port forwarding
	if !strings.Contains(argStr, "hostfwd=tcp::2222-:22") {
		t.Errorf("expected SSH port forwarding, got: %s", argStr)
	}

	// Serial log
	if !strings.Contains(argStr, "file:/work/serial.log") {
		t.Errorf("expected serial log arg, got: %s", argStr)
	}
}

// --- TestBuildArgs_Headless ---

func TestBuildArgs_Headless(t *testing.T) {
	t.Run("headless_true", func(t *testing.T) {
		cfg := &MachineConfig{
			OverlayImage:  "/work/overlay.qcow2",
			Memory:        "2G",
			CPUs:          1,
			Headless:      true,
			QMPSocketPath: "/work/qmp.sock",
		}
		args := cfg.BuildArgs()
		assertContainsArg(t, args, "-display", "none")
		assertNotContainsArg(t, args, "-display", "gtk")
	})

	t.Run("headless_false", func(t *testing.T) {
		cfg := &MachineConfig{
			OverlayImage:  "/work/overlay.qcow2",
			Memory:        "2G",
			CPUs:          1,
			Headless:      false,
			QMPSocketPath: "/work/qmp.sock",
		}
		args := cfg.BuildArgs()
		assertContainsArg(t, args, "-display", "gtk")
		assertNotContainsArg(t, args, "-display", "none")
	})
}

// --- TestBuildArgs_UEFI ---

func TestBuildArgs_UEFI(t *testing.T) {
	cfg := &MachineConfig{
		OverlayImage:  "/work/overlay.qcow2",
		Memory:        "2G",
		CPUs:          1,
		Firmware:      "/usr/share/OVMF/OVMF_CODE.fd",
		FirmwareVars:  "/work/efivars.fd",
		QMPSocketPath: "/work/qmp.sock",
	}

	args := cfg.BuildArgs()
	argStr := strings.Join(args, " ")

	// Firmware pflash (readonly)
	if !strings.Contains(argStr, "if=pflash,format=raw,readonly=on,file=/usr/share/OVMF/OVMF_CODE.fd") {
		t.Errorf("expected firmware pflash arg, got: %s", argStr)
	}

	// Firmware vars pflash
	if !strings.Contains(argStr, "if=pflash,format=raw,file=/work/efivars.fd") {
		t.Errorf("expected firmware vars pflash arg, got: %s", argStr)
	}
}

func TestBuildArgs_NoUEFI(t *testing.T) {
	cfg := &MachineConfig{
		OverlayImage:  "/work/overlay.qcow2",
		Memory:        "2G",
		CPUs:          1,
		QMPSocketPath: "/work/qmp.sock",
	}

	args := cfg.BuildArgs()
	argStr := strings.Join(args, " ")

	if strings.Contains(argStr, "pflash") {
		t.Errorf("expected no pflash args when firmware not set, got: %s", argStr)
	}
}

func TestBuildArgs_NoPortForwarding(t *testing.T) {
	cfg := &MachineConfig{
		OverlayImage:  "/work/overlay.qcow2",
		Memory:        "2G",
		CPUs:          1,
		QMPSocketPath: "/work/qmp.sock",
	}

	args := cfg.BuildArgs()
	argStr := strings.Join(args, " ")

	assertContainsArg(t, args, "-netdev", "user,id=net0")
	if strings.Contains(argStr, "hostfwd=") {
		t.Errorf("expected no host forwarding, got: %s", argStr)
	}
}

// --- TestBuildArgs_TPM ---

func TestBuildArgs_TPM(t *testing.T) {
	t.Run("tpm_enabled", func(t *testing.T) {
		cfg := &MachineConfig{
			OverlayImage:  "/work/overlay.qcow2",
			Memory:        "2G",
			CPUs:          1,
			TPMEnabled:    true,
			TPMSocketPath: "/work/tpm/swtpm.sock",
			QMPSocketPath: "/work/qmp.sock",
		}

		args := cfg.BuildArgs()
		argStr := strings.Join(args, " ")

		if !strings.Contains(argStr, "socket,id=chrtpm,path=/work/tpm/swtpm.sock") {
			t.Errorf("expected TPM chardev arg, got: %s", argStr)
		}
		if !strings.Contains(argStr, "emulator,id=tpm0,chardev=chrtpm") {
			t.Errorf("expected TPM tpmdev arg, got: %s", argStr)
		}
		assertContainsArg(t, args, "-device", "tpm-tis,tpmdev=tpm0")
	})

	t.Run("tpm_disabled", func(t *testing.T) {
		cfg := &MachineConfig{
			OverlayImage:  "/work/overlay.qcow2",
			Memory:        "2G",
			CPUs:          1,
			TPMEnabled:    false,
			QMPSocketPath: "/work/qmp.sock",
		}

		args := cfg.BuildArgs()
		argStr := strings.Join(args, " ")

		if strings.Contains(argStr, "chrtpm") {
			t.Errorf("expected no TPM args when TPM disabled, got: %s", argStr)
		}
	})

	t.Run("tpm_enabled_no_socket", func(t *testing.T) {
		cfg := &MachineConfig{
			OverlayImage:  "/work/overlay.qcow2",
			Memory:        "2G",
			CPUs:          1,
			TPMEnabled:    true,
			TPMSocketPath: "", // empty socket path
			QMPSocketPath: "/work/qmp.sock",
		}

		args := cfg.BuildArgs()
		argStr := strings.Join(args, " ")

		// TPM should not be added if socket path is empty
		if strings.Contains(argStr, "chrtpm") {
			t.Errorf("expected no TPM args when socket path is empty, got: %s", argStr)
		}
	})
}

// --- TestBuildArgs_ExtraArgs ---

func TestBuildArgs_ExtraArgs(t *testing.T) {
	cfg := &MachineConfig{
		OverlayImage:  "/work/overlay.qcow2",
		Memory:        "2G",
		CPUs:          1,
		QMPSocketPath: "/work/qmp.sock",
		ExtraArgs:     []string{"-usb", "-device", "usb-tablet"},
	}

	args := cfg.BuildArgs()

	// Extra args should be at the end
	lastThree := args[len(args)-3:]
	if lastThree[0] != "-usb" || lastThree[1] != "-device" || lastThree[2] != "usb-tablet" {
		t.Errorf("expected extra args at end, got last 3: %v", lastThree)
	}
}

// --- TestDetectAccelerator ---

func TestDetectAccelerator(t *testing.T) {
	accel := detectAccelerator()

	switch runtime.GOOS {
	case "linux":
		if accel != "kvm" {
			t.Errorf("on linux expected kvm, got %q", accel)
		}
	case "windows":
		if accel != "whpx" {
			t.Errorf("on windows expected whpx, got %q", accel)
		}
	default:
		if accel != "tcg" {
			t.Errorf("on %s expected tcg, got %q", runtime.GOOS, accel)
		}
	}

	// Verify indirectly via BuildArgs
	cfg := &MachineConfig{
		OverlayImage:  "/work/overlay.qcow2",
		Memory:        "2G",
		CPUs:          1,
		QMPSocketPath: "/work/qmp.sock",
	}
	args := cfg.BuildArgs()
	expectedMachine := "q35,accel=" + accel
	assertContainsArg(t, args, "-machine", expectedMachine)
}

// --- TestFindFreeVNCDisplay ---

func TestFindFreeVNCDisplay(t *testing.T) {
	display, err := FindFreeVNCDisplay()
	if err != nil {
		if strings.Contains(err.Error(), "no free VNC display available") {
			t.Skipf("VNC port probing is not available in this environment: %v", err)
		}
		t.Fatalf("FindFreeVNCDisplay failed: %v", err)
	}
	if display < 0 {
		t.Errorf("expected display >= 0, got %d", display)
	}
	if display >= 100 {
		t.Errorf("expected display < 100, got %d", display)
	}
}

// --- TestVNCPort ---

func TestVNCPort(t *testing.T) {
	tests := []struct {
		display int
		want    int
	}{
		{0, 5900},
		{1, 5901},
		{10, 5910},
		{99, 5999},
	}

	for _, tt := range tests {
		got := VNCPort(tt.display)
		if got != tt.want {
			t.Errorf("VNCPort(%d) = %d, want %d", tt.display, got, tt.want)
		}
	}
}

// --- Helper functions ---

// assertContainsArg checks that args contains the given flag/value pair.
func assertContainsArg(t *testing.T, args []string, flag, value interface{}) {
	t.Helper()
	flagStr := flag.(string)
	valueStr := ""
	if v, ok := value.(string); ok {
		valueStr = v
	} else if v, ok := value.(int); ok {
		valueStr = strings.TrimSpace(strings.Repeat(" ", v)) // not used in practice
	}

	for i := 0; i < len(args)-1; i++ {
		if args[i] == flagStr && args[i+1] == valueStr {
			return
		}
	}
	t.Errorf("expected args to contain %q %q, got: %v", flagStr, valueStr, args)
}

// assertNotContainsArg checks that args does NOT contain the given flag/value pair.
func assertNotContainsArg(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			t.Errorf("expected args NOT to contain %q %q, but found it", flag, value)
			return
		}
	}
}
