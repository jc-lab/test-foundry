# test-foundry

`test-foundry` is a QEMU-based Windows guest automation test tool.  
It covers VM setup, snapshot creation, test execution, file upload/download, screenshot capture, and QMP-event-based waits such as `wait-panic` and `wait-reset`.

- English: [README.md](README.md)
- Korean: [README-ko.md](README.ko.md)

## Features

- QEMU VM setup and snapshot-based test execution
- Split execution and file-transfer methods for Windows guests (`SSH` / `WinRM`)
- GitHub workflow-like step-based test definition (`wait-boot`, `exec`, `file-upload`, `file-download`, `screenshot`, `shutdown`, and more)
- expression support in test step params
  - `${{ test.dir }}`
  - `${{ vmconfig.<json-key> }}`

## Requirements

- QEMU
- OVMF / UEFI firmware
- qemu-img
- swtpm (optional. linux only)

The example flow is primarily prepared for running Windows guests on Linux hosts.

## Build

```bash
go build ./cmd/test-foundry
```

Or:

```bash
go run ./cmd/test-foundry --help
```

Running the root command shows output like this:

```text
$ go run ./cmd/test-foundry/
test-foundry automates testing of Windows drivers and UEFI applications
using QEMU virtual machines. It provides VM lifecycle management,
snapshot-based test execution, and step-by-step test automation.

Usage:
  test-foundry [command]

Available Commands:
  action      Execute individual actions against a running VM
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  test        Run tests against a VM snapshot
  vm-destroy  Destroy VM context directory
  vm-setup    Create VM context and prepare a snapshot

Flags:
      --headless         Headless mode (VNC only, no display)
  -h, --help             help for test-foundry
      --qemu string      QEMU binary path (default "qemu-system-x86_64")
      --verbose          Verbose logging
      --vm-name string   VM context name (required)
      --workdir string   VM context directory root (default ".testfoundry")

Use "test-foundry [command] --help" for more information about a command.
```

The `action` subcommand help looks like this:

```text
$ go run ./cmd/test-foundry/ action --help
Execute individual actions against a running VM session via the IPC daemon.
Each action communicates with the daemon over HTTP to perform operations
on the guest OS or QEMU instance.

Usage:
  test-foundry action [command]

Available Commands:
  dump          Dump guest memory via QMP
  exec          Execute a command on the guest via SSH
  file-download Download a file from the guest via SFTP
  file-upload   Upload a file to the guest via SFTP
  reboot        Reboot the guest
  screenshot    Capture a screenshot via VNC
  shutdown      Gracefully shut down the guest
  sleep         Wait for a specified duration
  wait-boot     Wait until the guest OS is reachable via SSH
  wait-oobe     Wait until Windows OOBE is completed
  wait-panic    Wait for a pvpanic event from the guest

Flags:
  -h, --help   help for action

Global Flags:
      --headless         Headless mode (VNC only, no display)
      --qemu string      QEMU binary path (default "qemu-system-x86_64")
      --verbose          Verbose logging
      --vm-name string   VM context name (required)
      --workdir string   VM context directory root (default ".testfoundry")

Use "test-foundry action [command] --help" for more information about a command.
```

## Basic Workflow

1. Prepare a base image.
2. Run `vm-setup` to create a VM context and snapshot.
3. Run `test` to restore the snapshot and execute test steps.

The default workdir is `.testfoundry`, and `--vm-name` is used to distinguish VM contexts.

## Example: Windows 11 Test

### 1. Extract a qcow2 image

```bash
./scripts/vagrant-extract-qcow2.sh gusztavvargadr/windows-11 2601.0.0 ./images
```

This prepares the qcow2 image under `./images` for the sample image definition at [examples/images/windows-11.yaml](examples/images/windows-11.yaml).

### 2. Run VM setup

```bash
test-foundry --vm-name="win11" vm-setup --image ./examples/images/windows-11.yaml
```

This step performs:

- VM context creation
- QEMU boot
- waiting for OOBE completion
- WinRM / SSH preparation
- setup step execution
- shutdown and snapshot creation

### 3. Run the test

```bash
test-foundry --vm-name="win11" test --output ./temp --test ./examples/tests/01-hello-world/test.yaml
```

In this command:

- `--output ./temp` is the location for `test-result.json`
- the sample test itself writes output files to `./output/01/...` as defined in [examples/tests/01-hello-world/test.yaml](examples/tests/01-hello-world/test.yaml)

So after the run, artifacts are split across two locations:

- `./temp/test-result.json`
- `./output/01/...`

### 4. Verify generated files

```bash
find output -type f
```

Expected output:

```text
output/01/hello-output.txt
output/01/screenshot.png
```

## Example Test Structure

The sample [examples/tests/01-hello-world/test.yaml](examples/tests/01-hello-world/test.yaml) runs this sequence:

1. `wait-boot`
2. `file-upload`
3. `exec`
4. `file-download`
5. `screenshot`
6. `shutdown`

Test parameters can also use expressions:

```yaml
params:
  src: "${{ test.dir }}/hello.bat"
  name: "${{ vmconfig.machine_name }}"
  ssh_port: "${{ vmconfig.ssh_host_port }}"
```

## Expressions

Expression format:

```text
${{ ... }}
```

Currently supported:

- `${{ test.dir }}`
  - the directory containing the currently running test YAML
- `${{ vmconfig.<key> }}`
  - access to runtime `MachineConfig` JSON fields

Examples:

- `${{ vmconfig.machine_name }}`
- `${{ vmconfig.qmp_socket_path }}`
- `${{ vmconfig.ssh_host_port }}`

## TODO

- Linux guest support

## License

[GPL-2.0-only](LICENSE)
