#!/bin/bash
set -euo pipefail

require_env() {
    local name="$1"
    if [[ -z "${!name:-}" ]]; then
        echo "missing required environment variable: ${name}" >&2
        exit 1
    fi
}

download_and_verify() {
    local url="$1"
    local expected_sha="$2"
    local destination="$3"

    curl --fail --silent --show-error --location "$url" -o "$destination"
    echo "${expected_sha}  ${destination}" | shasum -a 256 -c -
}

require_env LOCAL_RUNTIME_RUNFILE_URL
require_env LOCAL_RUNTIME_RUNFILE_SHA256
require_env LOCAL_RUNTIME_KERNEL_URL
require_env LOCAL_RUNTIME_KERNEL_SHA256
require_env LOCAL_RUNTIME_INITRD_URL
require_env LOCAL_RUNTIME_INITRD_SHA256

payload_version="${LOCAL_RUNTIME_PAYLOAD_VERSION:-}"
if [[ -z "${payload_version}" ]]; then
    payload_version="$(git describe --tags --abbrev=0 2>/dev/null || true)"
    payload_version="${payload_version#v}"
fi
if [[ -z "${payload_version}" ]]; then
    echo "LOCAL_RUNTIME_PAYLOAD_VERSION is required when no git tag is available" >&2
    exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

runfile_path="${tmp_dir}/payload.run"
kernel_path="${tmp_dir}/vmlinux.container"
initrd_path="${tmp_dir}/ubuntu-initrd.cpio.gz"

download_and_verify "${LOCAL_RUNTIME_RUNFILE_URL}" "${LOCAL_RUNTIME_RUNFILE_SHA256}" "${runfile_path}"
download_and_verify "${LOCAL_RUNTIME_KERNEL_URL}" "${LOCAL_RUNTIME_KERNEL_SHA256}" "${kernel_path}"
download_and_verify "${LOCAL_RUNTIME_INITRD_URL}" "${LOCAL_RUNTIME_INITRD_SHA256}" "${initrd_path}"

task local-runtime-bundle \
    LOCAL_RUNTIME_VERSION="${payload_version}" \
    LOCAL_RUNTIME_RUNFILE="${runfile_path}" \
    LOCAL_RUNTIME_KERNEL="${kernel_path}" \
    LOCAL_RUNTIME_INITRD="${initrd_path}"

metadata_path="assets/localruntimebin/generated/darwin/arm64/metadata.json"
bundle_path="assets/localruntimebin/generated/darwin/arm64/payload.tar.gz"

test -s "${metadata_path}"
test -s "${bundle_path}"
grep -F "\"version\": \"${payload_version}\"" "${metadata_path}" >/dev/null
grep -F "\"architecture\": \"arm64\"" "${metadata_path}" >/dev/null
