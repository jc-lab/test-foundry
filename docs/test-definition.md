# Test Definition

A test definition YAML describes the steps that test-foundry should run against an existing VM snapshot.
It can also define offline `preboot` actions and `panic` actions for crash handling.

## Example

```yaml
name: "hello-world"
description: "Upload and run a hello-world bat file, capture stdout and screenshot"

steps:
  - action: wait-boot
    timeout: 120s
    params:
      retry_interval: 5s

  - action: file-upload
    timeout: 30s
    params:
      src: "${{ test.dir }}/hello.bat"
      dst: "C:\\Temp\\hello.bat"

  - action: exec
    timeout: 30s
    params:
      cmd: "cmd.exe"
      args: ["/c", "C:\\Temp\\hello.bat > C:\\Temp\\hello-output.txt 2>&1"]
      expect_exit_code: 0

  - action: file-download
    timeout: 30s
    params:
      src: "C:\\Temp\\hello-output.txt"
      dst: "./output/01/hello-output.txt"

  - action: screenshot
    timeout: 10s
    params:
      output: "./output/01/screenshot.png"

  - action: shutdown
    timeout: 120s
```

## Top-Level Fields

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | Human-readable test name. |
| `description` | no | Free-form description of the test. |
| `include` | no | List of YAML files to merge into the test definition. |
| `preboot` | no | Offline disk-preparation steps that run before the test boot sequence. |
| `steps` | yes | Main test steps. At least one step is required. |
| `panic` | no | Panic-handling steps executed when a guest panic is detected. |

## Include Files

`include` lets you share common blocks such as panic handlers.
The loader reads include files in order and merges only these top-level arrays:

- `preboot.steps`
- `steps`
- `panic.steps`

Merge rules:

- Later include files win over earlier include files.
- The main YAML always wins over included content.
- If the main YAML defines a step array, the included version of that array is ignored.
- Nested includes are not processed.

The sample [`examples/tests/common/panic-handler.yaml`](../examples/tests/common/panic-handler.yaml) is a good pattern for shared panic handling.

## Step Format

Every step uses the same base shape:

```yaml
- action: exec
  timeout: 30s
  params:
    cmd: "sc"
    args: ["query", "mydriver"]
```

| Field | Required | Description |
| --- | --- | --- |
| `action` | yes | Action name. |
| `timeout` | yes | Maximum duration for the step. It must be a valid duration for `steps`, `panic.steps`, and any other runtime step list. |
| `params` | no | Action-specific parameters. |

Durations use standard Go duration syntax such as `10s`, `5m`, or `1h30m`.

## Supported Actions

### `wait-boot`

Waits until the guest becomes reachable.

```yaml
params:
  retry_interval: 5s
```

| Param | Required | Description |
| --- | --- | --- |
| `retry_interval` | no | Retry interval while waiting for SSH. Defaults to `5s`. |

### `wait-oobe`

Waits until Windows OOBE is complete.

This action does not require params.

### `file-upload`

Uploads a local file to the guest.

```yaml
params:
  src: "./build/mydriver.inf"
  dst: "C:\\Drivers\\mydriver.inf"
```

| Param | Required | Description |
| --- | --- | --- |
| `src` | yes | Local source file path. |
| `dst` | yes | Destination path in the guest. |

### `file-download`

Downloads a file from the guest to the local machine.

| Param | Required | Description |
| --- | --- | --- |
| `src` | yes | Source file path in the guest. |
| `dst` | yes | Local destination path. |

### `exec`

Runs a command in the guest.

```yaml
params:
  cmd: "powershell"
  args:
    - "-Command"
    - "Set-ExecutionPolicy Bypass -Scope Process -Force"
  expect_exit_code: 0
```

| Param | Required | Description |
| --- | --- | --- |
| `cmd` | yes | Command to execute. |
| `args` | no | Command arguments. |
| `expect_exit_code` | no | If set, the action fails unless the command exits with this code. |

### `screenshot`

Captures the VM display and saves it as PNG.

| Param | Required | Description |
| --- | --- | --- |
| `output` | yes | Path to the output PNG file. |

### `shutdown`

Requests a graceful guest shutdown.

This action does not require params.

### `poweroff`

Forces the VM process to exit.

This action does not require params.

### `reboot`

Reboots the guest and waits for it to come back.

This action does not require params.

### `dump`

Captures a guest memory dump through QMP.

```yaml
params:
  format: "win-dmp"
  output: "./output/02/memory.dump"
```

| Param | Required | Description |
| --- | --- | --- |
| `format` | no | Dump format passed to QEMU. Common values include `win-dmp`, `elf`, and the QEMU-supported compressed formats. |
| `output` | yes | Output path for the memory dump. |

### `sleep`

Sleeps for the requested duration.

```yaml
params:
  duration: 5s
```

| Param | Required | Description |
| --- | --- | --- |
| `duration` | yes | Duration to sleep, using Go duration syntax. |

### `wait-panic`

Waits for a pvpanic event from the guest.

This action does not require params.

## Preboot Steps

`preboot.steps` can also appear in a test definition. This is useful when a test needs a one-off EFI
change before the main test sequence starts.

The same preboot actions as image definitions are available:

- `efi-add-file`
- `efi-get-file`

Example:

```yaml
preboot:
  steps:
    - action: efi-add-file
      params:
        src: "${{ test.dir }}/shellx64.efi"
        dst: "/EFI/Boot/bootx64.efi"
```

Preboot expressions support:

- `${{ test.dir }}`
- `${{ env.NAME }}`

## Panic Handling

`panic.steps` runs when the guest triggers pvpanic and the test runner detects a crash.
This is a good place to take screenshots or capture memory dumps.

```yaml
panic:
  steps:
    - action: screenshot
      timeout: 10s
      params:
        output: "./output/02/bsod-screenshot.png"

    - action: dump
      timeout: 300s
      params:
        format: "win-dmp"
        output: "./output/02/memory.dump"
```

## Expressions

Step parameters support expression strings in the form:

```text
${{ ... }}
```

In test steps, the following expressions are available:

- `${{ test.dir }}` for the directory containing the current test YAML
- `${{ env.NAME }}` for process environment variables
- `${{ vmconfig.<path> }}` for runtime VM configuration data

Expressions can be used inside a full string or embedded in a larger string.

## Minimal Test

```yaml
name: "smoke"
steps:
  - action: wait-boot
    timeout: 5m
  - action: shutdown
    timeout: 2m
```
