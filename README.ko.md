# test-foundry

`test-foundry`는 QEMU 기반의 Windows 게스트 자동화 테스트 도구입니다.  
VM 준비, 스냅샷 생성, 테스트 실행, 파일 업로드/다운로드, 화면 캡처, QMP 이벤트 기반 대기(`wait-panic`, `wait-reset`)를 하나의 흐름으로 다룹니다.

- English: [README.md](README.md)
- Korean: [README-ko.md](README.ko.md)

## 주요 기능

- QEMU VM 생성 및 스냅샷 기반 테스트 실행
- GitHub workflow 유사 step 기반 테스트 정의 (`wait-boot`, `exec`, `file-upload`, `file-download`, `screenshot`, `shutdown`, `poweroff` 등의 action 지원)
- 부팅 전 오프라인 디스크 수정용 `preboot.steps` 지원 (`efi-add-file` 등)
- test step param 에 대해 expression 지원
  - `${{ test.dir }}`
  - `${{ output.dir }}`
  - `${{ env.<name> }}`
  - `${{ vmconfig.<json-key> }}`

## 요구 사항

- QEMU
- OVMF/UEFI firmware
- qemu-img
- swtpm (optional. linux only)

Linux 환경에서 Windows 게스트를 테스트하는 흐름을 기준으로 예제가 준비되어 있습니다.

## 빌드

```bash
go build ./cmd/test-foundry
```

또는:

```bash
go run ./cmd/test-foundry --help
```

루트 명령을 실행하면 다음과 같은 help를 볼 수 있습니다.

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

`action` 하위 명령 help는 다음과 같습니다.

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
  poweroff      Forcefully power off the VM
  reboot        Reboot the guest
  screenshot    Capture a screenshot via VNC
  shutdown      Gracefully shut down the guest
  sleep         Wait for a specified duration
  wait-boot     Wait until the guest OS is reachable via SSH
  wait-oobe     Wait until Windows OOBE is completed
  wait-panic    Wait for a pvpanic event from the guest
  wait-reset    Wait for a reset event from the guest

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

## 기본 사용 흐름

1. 베이스 이미지를 준비합니다.
2. `vm-setup`으로 VM context와 스냅샷을 생성합니다.
3. `test`로 스냅샷을 복원한 뒤 테스트를 실행합니다.

기본 workdir은 `.testfoundry`이며, `--vm-name`으로 VM context를 구분합니다.

## 예제: Windows 11 테스트

### 1. qcow2 이미지 추출

```bash
./scripts/vagrant-extract-qcow2.sh gusztavvargadr/windows-11 2601.0.0 ./images
```

이 명령은 예제 이미지 설정 파일 [examples/images/windows-11.yaml](examples/images/windows-11.yaml)에서 사용하는 qcow2 이미지를 `./images` 아래에 준비합니다.

### 2. VM setup

```bash
test-foundry --vm-name="win11" vm-setup --image ./examples/images/windows-11.yaml
```

이 단계에서는:

- VM context 생성
- QEMU 부팅
- OOBE 완료 대기
- WinRM/SSH 준비
- setup step 실행
- 종료 후 스냅샷 저장

이 수행됩니다.

### 3. 테스트 실행

```bash
test-foundry --vm-name="win11" test --output ./temp --test ./examples/tests/01-hello-world/test.yaml
```

이 명령에서:

- `--output ./temp` 는 `test-result.json` 저장 위치입니다.
- 예제 테스트 자체는 [examples/tests/01-hello-world/test.yaml](examples/tests/01-hello-world/test.yaml) 안에서 결과 파일 경로를 `./output/01/...` 로 정의하고 있습니다.

즉, 실행 후 산출물은 두 군데로 나뉩니다.

- `./temp/test-result.json`
- `./output/01/...`

### 4. 생성 파일 확인

```bash
find output -type f
```

예상 결과:

```text
output/01/hello-output.txt
output/01/screenshot.png
```

### Video

https://github.com/user-attachments/assets/df809996-ffa4-41ff-853d-884219c2b46c

## 테스트 정의 예시

예제 [examples/tests/01-hello-world/test.yaml](examples/tests/01-hello-world/test.yaml) 은 다음 흐름으로 동작합니다.

1. `wait-boot`
2. `file-upload`
3. `exec`
4. `file-download`
5. `screenshot`
6. `shutdown`

또한 테스트 파라미터에서는 expression을 사용할 수 있습니다.

```yaml
params:
  src: "${{ test.dir }}/hello.bat"
  name: "${{ vmconfig.machine_name }}"
  ssh_port: "${{ vmconfig.ssh_host_port }}"
```

## Preboot Step

`preboot.steps`는 QEMU를 부팅하기 전에 qcow2 디스크를 오프라인으로 수정할 때 사용합니다. 현재는 EFI System Partition의 FAT32 파일시스템에 파일을 넣는 `efi-add-file` action을 지원합니다.

- image YAML의 `preboot.steps`
- test YAML의 `preboot.steps`

둘 다 지원하므로, 베이스 이미지 준비 단계와 개별 테스트 단계에서 모두 EFI 파일을 패치할 수 있습니다.

예:

```yaml
preboot:
  steps:
    - action: efi-add-file
      params:
        src: "${{ test.dir }}/bootx64.efi"
        dst: /EFI/Boot/bootx64.efi
```

전체 예시는 [examples/tests/03-patch-efi/test.yaml](examples/tests/03-patch-efi/test.yaml) 과 [examples/tests/03-patch-efi/shellx64.efi](examples/tests/03-patch-efi/shellx64.efi) 를 참고할 수 있습니다. 이 예제는 EFI 부트 경로를 패치한 뒤 스크린샷을 찍고 `poweroff`로 종료합니다.

## Expression

Expression 형식:

```text
${{ ... }}
```

현재 지원:

- `${{ test.dir }}`
  - 현재 실행 중인 test YAML 파일의 디렉터리
- `${{ env.<name> }}`
  - 현재 프로세스 환경 변수 접근
- `${{ vmconfig.<key> }}`
  - 런타임 `MachineConfig`의 JSON 필드 접근

예:

- `${{ vmconfig.machine_name }}`
- `${{ vmconfig.qmp_socket_path }}`
- `${{ vmconfig.ssh_host_port }}`
- `${{ env.HOME }}`

## TODO

- Linux guest 지원

## Documentations

- [docs/image-definition.md](docs/image-definition.md)
- [docs/test-definition.md](docs/test-definition.md)

## License

[GPL-2.0-only](LICENSE)
