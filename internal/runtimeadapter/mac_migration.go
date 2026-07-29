// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import "fmt"

const legacyNanoContainerName = "exasol-local-db"

// RenderMacBootHook renders the Personal-owned one-off /exa migration and
// workload startup hook. The target directory is installed by an atomic sibling
// rename, so its existence is the only migration completion marker.
func RenderMacBootHook(dataRoot, legacyDataPath, workloadHelper string) []byte {
	return []byte(fmt.Sprintf(`#!/bin/sh
set -eu

data_root=%s
target="$data_root/exa"
staging="$data_root/exa.migrating"
legacy_source=%s
overlay_source="${legacy_source}.personal-migration-source"
legacy_container=%s
workload_helper=%s

dir_has_entries() {
  [ -d "$1" ] && [ -n "$(find "$1" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null)" ]
}

tree_digest() {
  (cd "$1" && tar -cf - .) | sha256sum | awk '{print $1}'
}

legacy_container_exists() {
  podman container exists "$legacy_container"
}

if [ -e "$target" ] || [ -L "$target" ]; then
  if [ -L "$target" ] || [ ! -d "$target" ]; then
    echo "Personal data target is not a directory: $target" >&2
    exit 1
  fi
  exec "$workload_helper" apply
fi

mkdir -p "$data_root"
if legacy_container_exists; then
  podman stop "$legacy_container"
fi

source=
if [ -L "$legacy_source" ]; then
  echo "Legacy /exa source must not be a symlink: $legacy_source" >&2
  exit 1
elif dir_has_entries "$legacy_source"; then
  source="$legacy_source"
elif legacy_container_exists; then
  if [ -L "$overlay_source" ] || { [ -e "$overlay_source" ] && [ ! -d "$overlay_source" ]; }; then
    echo "Legacy overlay migration source is not a directory: $overlay_source" >&2
    exit 1
  fi
  if [ ! -d "$overlay_source" ]; then
    mkdir -p "$overlay_source"
    if ! podman cp "$legacy_container:/exa/." "$overlay_source"; then
      echo "Failed to copy legacy overlay-backed /exa; the old container was preserved" >&2
      exit 1
    fi
    sync
  fi
  source="$overlay_source"
fi

if [ -L "$staging" ] || { [ -e "$staging" ] && [ ! -d "$staging" ]; }; then
  echo "Migration staging path is not a directory: $staging" >&2
  exit 1
fi
if [ -d "$staging" ]; then
  rm -rf "$staging"
fi
if [ -n "$source" ]; then
  cp -a "$source" "$staging"
  source_digest=$(tree_digest "$source")
  target_digest=$(tree_digest "$staging")
  if [ "$source_digest" != "$target_digest" ]; then
    echo "Verification failed while migrating legacy /exa" >&2
    exit 1
  fi
else
  mkdir "$staging"
fi

sync
mv "$staging" "$target"
sync
exec "$workload_helper" apply
`,
		shellQuote(dataRoot),
		shellQuote(legacyDataPath),
		shellQuote(legacyNanoContainerName),
		shellQuote(workloadHelper),
	))
}
