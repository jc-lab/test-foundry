#!/usr/bin/env bash
# Copyright 2026 JC-Lab
# SPDX-License-Identifier: GPL-2.0-only

#
# vagrant-extract-qcow2.sh
#
# Vagrant box에서 qcow2 이미지를 추출하는 헬퍼 스크립트.
# libvirt provider가 있으면 직접 qcow2를 복사하고,
# 없으면 virtualbox provider에서 VMDK를 다운로드 후 qemu-img로 변환한다.
# Go 코드에 포함되지 않는 독립 스크립트.
#
# Usage:
#   ./scripts/vagrant-extract-qcow2.sh <box-name> [version] [output-dir]
#
# Example:
#   ./scripts/vagrant-extract-qcow2.sh peru/windows-11-enterprise-x64-eval                       # latest, libvirt
#   ./scripts/vagrant-extract-qcow2.sh gusztavvargadr/windows-11 2601.0.0 ./images/              # specific version
#   ./scripts/vagrant-extract-qcow2.sh peru/windows-11-enterprise-x64-eval 20241210.1.0 ./images/
#
# Prerequisites:
#   - vagrant (>= 2.0)
#   - python3 (for metadata.json parsing)
#   - qemu-img (for non-qcow2 disk conversion)
#

set -euo pipefail

if [ $# -lt 1 ]; then
    echo "Usage: $0 <box-name> [version] [output-dir]"
    echo ""
    echo "Arguments:"
    echo "  box-name    Vagrant box name (e.g., peru/windows-11-enterprise-x64-eval)"
    echo "  version     Box version (e.g., 20241210.1.0). If omitted, uses the latest."
    echo "  output-dir  Directory to save extracted qcow2 (default: current directory)"
    exit 1
fi

BOX_NAME="$1"
BOX_VERSION_ARG="${2:-}"
OUTPUT_DIR="${3:-.}"

# Vagrant stores boxes in ~/.vagrant.d/boxes/
# Box name slashes are replaced with -VAGRANTSLASH-
BOX_DIR_NAME=$(echo "${BOX_NAME}" | sed 's|/|-VAGRANTSLASH-|g')
BOX_BASE_DIR="${HOME}/.vagrant.d/boxes/${BOX_DIR_NAME}"

# ---------------------------------------------------------------------------
# Try providers in order: libvirt (native qcow2), then virtualbox (vmdk -> convert)
# ---------------------------------------------------------------------------
PROVIDERS=("libvirt" "virtualbox")
BOX_PROVIDER=""
BOX_VERSION=""

# find_provider_dir searches for the provider directory under a version dir.
# Vagrant box layout varies:
#   New format: {version}/{arch}/{provider}/   (e.g., 2601.0.0/amd64/libvirt/)
#   Old format: {version}/{provider}/          (e.g., 2601.0.0/libvirt/)
# Sets BOX_DIR to the resolved path on success.
find_provider_dir() {
    local version_dir="$1"
    local provider="$2"

    # Try new format first: {version}/{arch}/{provider}/
    for arch_dir in "${version_dir}"/*/; do
        if [ -d "${arch_dir}${provider}" ]; then
            BOX_DIR="${arch_dir}${provider}"
            return 0
        fi
    done

    # Try old format: {version}/{provider}/
    if [ -d "${version_dir}/${provider}" ]; then
        BOX_DIR="${version_dir}/${provider}"
        return 0
    fi

    return 1
}

# Step 1: Check if the box already exists locally (any provider)
resolve_local_box() {
    if [ ! -d "${BOX_BASE_DIR}" ]; then
        return 1
    fi

    local version
    if [ -n "${BOX_VERSION_ARG}" ]; then
        version="${BOX_VERSION_ARG}"
    else
        version=$(ls -1 "${BOX_BASE_DIR}" | grep -v metadata_url | sort -V | tail -n 1)
    fi

    if [ -z "${version}" ] || [ ! -d "${BOX_BASE_DIR}/${version}" ]; then
        return 1
    fi

    for provider in "${PROVIDERS[@]}"; do
        if find_provider_dir "${BOX_BASE_DIR}/${version}" "${provider}"; then
            BOX_VERSION="${version}"
            BOX_PROVIDER="${provider}"
            return 0
        fi
    done
    return 1
}

if resolve_local_box; then
    echo "==> Box already exists locally: provider=${BOX_PROVIDER}, version=${BOX_VERSION}"
else
    # Step 2: Not found locally — download, trying each provider in order
    for provider in "${PROVIDERS[@]}"; do
        echo "==> Downloading box: provider=${provider}"

        VAGRANT_ADD_ARGS=(box add --provider "${provider}")
        if [ -n "${BOX_VERSION_ARG}" ]; then
            VAGRANT_ADD_ARGS+=(--box-version "${BOX_VERSION_ARG}")
        fi
        VAGRANT_ADD_ARGS+=("${BOX_NAME}")

        echo "    vagrant ${VAGRANT_ADD_ARGS[*]}"
        if vagrant "${VAGRANT_ADD_ARGS[@]}" 2>&1; then
            # Download succeeded — resolve again
            if resolve_local_box; then
                echo "==> Downloaded box: provider=${BOX_PROVIDER}, version=${BOX_VERSION}"
                break
            fi
        fi
        echo "    Provider '${provider}' not available, trying next..."
    done
fi

if [ -z "${BOX_PROVIDER}" ]; then
    echo "Error: Could not find box '${BOX_NAME}' with any supported provider (${PROVIDERS[*]})"
    echo ""
    echo "Possible causes:"
    echo "  - The box name may not exist on Vagrant Cloud"
    echo "  - The box may not support libvirt or virtualbox providers"
    echo "  - Network error during download"
    if [ -d "${BOX_BASE_DIR}" ]; then
        echo ""
        echo "Existing box contents:"
        find "${BOX_BASE_DIR}" -maxdepth 3 -type d | head -20
    fi
    exit 1
fi

# BOX_DIR is already set by resolve_local_box() / find_provider_dir()

# Create output directory
mkdir -p "${OUTPUT_DIR}"

# Generate output filename from box name and version
SAFE_NAME=$(echo "${BOX_NAME}" | tr '/' '-')
OUTPUT_FILE="${OUTPUT_DIR}/${SAFE_NAME}-${BOX_VERSION}.qcow2"

# ---------------------------------------------------------------------------
# Read disk info from metadata.json and extract / convert
# ---------------------------------------------------------------------------
METADATA_FILE="${BOX_DIR}/metadata.json"

if [ ! -f "${METADATA_FILE}" ]; then
    echo "Error: metadata.json not found in ${BOX_DIR}"
    echo "Contents:"
    ls -la "${BOX_DIR}/"
    exit 1
fi

echo "==> Reading metadata: ${METADATA_FILE}"

# Parse metadata.json — use python3 as a portable JSON parser (jq may not be installed)
DISK_PATH=$(python3 -c "
import json, sys
meta = json.load(open(sys.argv[1]))
disks = meta.get('disks', [])
if disks:
    print(disks[0]['path'])
" "${METADATA_FILE}" 2>/dev/null || true)

DISK_FORMAT=$(python3 -c "
import json, sys
meta = json.load(open(sys.argv[1]))
disks = meta.get('disks', [])
if disks:
    print(disks[0].get('format', ''))
" "${METADATA_FILE}" 2>/dev/null || true)

if [ -z "${DISK_PATH}" ]; then
    echo "Error: No disk entry found in metadata.json"
    echo "Contents of metadata.json:"
    cat "${METADATA_FILE}"
    exit 1
fi

DISK_FILE="${BOX_DIR}/${DISK_PATH}"

if [ ! -f "${DISK_FILE}" ]; then
    echo "Error: Disk file not found: ${DISK_FILE}"
    echo "Contents of ${BOX_DIR}:"
    ls -la "${BOX_DIR}/"
    exit 1
fi

echo "    Disk: ${DISK_PATH} (format: ${DISK_FORMAT:-unknown})"

if [ "${DISK_FORMAT}" = "qcow2" ]; then
    echo "==> Copying qcow2 image..."
    echo "    Source: ${DISK_FILE}"
    echo "    Target: ${OUTPUT_FILE}"
    cp "${DISK_FILE}" "${OUTPUT_FILE}"
else
    # Need conversion (vmdk, vdi, raw, etc.)
    if ! command -v qemu-img &>/dev/null; then
        echo "Error: qemu-img is required to convert '${DISK_FORMAT}' to qcow2"
        echo "Install: apt install qemu-utils  OR  dnf install qemu-img"
        exit 1
    fi

    CONVERT_ARGS=(-O qcow2)
    if [ -n "${DISK_FORMAT}" ]; then
        CONVERT_ARGS=(-f "${DISK_FORMAT}" "${CONVERT_ARGS[@]}")
    fi

    echo "==> Converting disk image to qcow2 (${DISK_FORMAT:-auto} -> qcow2)..."
    echo "    Source: ${DISK_FILE}"
    echo "    Target: ${OUTPUT_FILE}"
    qemu-img convert "${CONVERT_ARGS[@]}" "${DISK_FILE}" "${OUTPUT_FILE}"
fi

echo "==> Done! Image saved to: ${OUTPUT_FILE}"
echo ""
echo "Image info:"
qemu-img info "${OUTPUT_FILE}" 2>/dev/null || echo "(qemu-img not available for info)"
