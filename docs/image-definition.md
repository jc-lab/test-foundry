# Image Definition

An image definition YAML describes a reusable VM base image and how test-foundry should connect to it.
It is used by `vm-setup`, where the tool boots the VM, runs the image setup steps, and then
creates a snapshot for later test execution.

## Example

```yaml
name: "windows-11-24h2"
os: "windows"
description: "Windows 11 24H2 Enterprise evaluation image"

qemu:
  # The locations of the image and firmware must not be changed even after vm-setup.
  image: "./images/gusztavvargadr-windows-11-2601.0.0.qcow2"
  firmware: "/usr/share/OVMF/OVMF_CODE_4M.fd"
  firmware_vars: "/usr/share/OVMF/OVMF_VARS_4M.fd"
  memory: "4G"
  cpus: 2
  cpu: host
  extra_args:
    - "-device"
    - "virtio-tablet-pci"

connection:
  exec_method: "winrm"
  file_method: "ssh"
  username: "vagrant"
  password: "vagrant"
  ssh_port: 22
  winrm_port: 5985

setup:
  steps:
    - action: wait-boot
      timeout: 10m
      params:
        retry_interval: 5s

    - action: shutdown
      timeout: 120s
```

## Top-Level Fields

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | Human-readable image name. |
| `os` | yes | Guest OS family. Supported values are `windows` and `linux`. |
| `description` | no | Free-form description of the image. |
| `qemu` | yes | QEMU and disk image settings. |
| `connection` | yes | Guest access settings for command execution and file transfer. |
| `preboot` | no | Offline disk-preparation steps that run before QEMU boots. |
| `setup` | no | Online setup steps that run during `vm-setup`. |

## QEMU Settings

`qemu.image` must point to an existing qcow2 file. The loader validates that the file exists before
continuing.

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `image` | yes | - | Path to the base qcow2 image. |
| `firmware` | no | - | UEFI firmware binary, such as an OVMF code image. |
| `firmware_vars` | no | - | UEFI variable store image. |
| `memory` | no | `2G` | Guest RAM size passed to QEMU. |
| `cpu` | no | `host` | CPU model passed to QEMU. |
| `cpus` | no | `2` | Number of virtual CPUs. |
| `extra_args` | no | - | Additional raw QEMU arguments. |

## Connection Settings

The connection block controls how test-foundry talks to the guest.

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `exec_method` | no | `ssh` | Command execution transport. Supported values are `ssh` and `winrm`. |
| `file_method` | no | same as `exec_method` | File transfer transport. Supported values are `ssh` and `winrm`. |
| `username` | yes | - | Guest account name used for authentication. |
| `password` | no | - | Password for SSH or WinRM authentication. Required for WinRM. |
| `key_file` | no | - | SSH private key path. Used only for SSH. |
| `port` | no | - | Compatibility field. If set, it is copied to `ssh_port` when `exec_method` is `ssh`, or to `winrm_port` when `exec_method` is `winrm`. |
| `ssh_port` | no | `22` | SSH port exposed by the guest. |
| `winrm_port` | no | `5985` or `5986` | WinRM port. If `use_tls` is true, the default becomes `5986`. |
| `use_tls` | no | `false` | WinRM only. Enables HTTPS transport. |

Rules enforced by the loader:

- `exec_method` must be `ssh` or `winrm`.
- `file_method` must be `ssh` or `winrm`.
- `username` is required.
- SSH needs either `password` or `key_file` if either execution or file transfer uses SSH.
- WinRM requires `password` if either execution or file transfer uses WinRM.

## Preboot Steps

`preboot.steps` runs before the VM boots. This is for offline disk edits, especially EFI content.
The built-in preboot actions are:

- `efi-add-file`
- `efi-get-file`

Each step uses the common step shape:

```yaml
- action: efi-add-file
  timeout: 30s
  params:
    src: "${{ test.dir }}/shellx64.efi"
    dst: "/EFI/Boot/bootx64.efi"
```

### Preboot Step Fields

| Field | Required | Description |
| --- | --- | --- |
| `action` | yes | Preboot action name. |
| `timeout` | no | Maximum step duration. If omitted or invalid, the loader applies `30s` for image preboot steps. |
| `params` | no | Action-specific parameters. |

### Preboot Expressions

Preboot step parameters support expressions in string values. The available expressions are:

- `${{ test.dir }}`
- `${{ env.NAME }}`

`test.dir` resolves to the directory containing the YAML file being processed, which makes it easy to
reference companion files inside the example directory.

## Setup Steps

`setup.steps` runs during `vm-setup` after QEMU starts and the guest becomes reachable.
The same step format is used as test steps, and each setup step must use a valid timeout:

```yaml
setup:
  steps:
    - action: wait-boot
      timeout: 10m
      params:
        retry_interval: 5s
```

Runtime setup steps support the shared action set documented in `docs/test-definition.md`.

## Minimal Image

```yaml
name: "my-image"
os: "linux"
qemu:
  image: "./images/base.qcow2"
connection:
  username: "test"
  password: "test"
setup:
  steps:
    - action: wait-boot
      timeout: 5m
```
