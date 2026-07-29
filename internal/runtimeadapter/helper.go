// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"fmt"
	"strconv"
	"strings"
)

// RenderWorkloadHelper returns the private helper used both inside local-vm
// and by direct Podman adapters.
func RenderWorkloadHelper(
	spec WorkloadSpec,
	manifestPath, imageArchivePath string,
) ([]byte, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	workloadName := WorkloadName(spec.DeploymentID)
	containerName := workloadName + "-db"
	var slcPulls strings.Builder
	for _, mount := range spec.SLCMounts {
		if _, err := fmt.Fprintf(
			&slcPulls,
			"if ! podman image exists %s; then podman pull %s; fi\n",
			shellQuote(mount.Image),
			shellQuote(mount.Image),
		); err != nil {
			return nil, fmt.Errorf("failed to render SLC image setup: %w", err)
		}
	}

	//nolint:dupword // The generated shell script necessarily has repeated fi terminators.
	script := fmt.Sprintf(`#!/bin/sh
set -eu

mode=${1:-}
manifest=%s
image_archive=%s
pod_name=%s
container_name=%s

case "$mode" in
  apply)
    if ! podman image exists %s; then
      podman load --input "$image_archive"
    fi
%s    podman kube play --replace "$manifest"
    ;;
  down)
    if podman pod exists "$pod_name"; then
      podman kube down "$manifest"
    else
      code=$?
      if [ "$code" -ne 1 ]; then
        exit "$code"
      fi
    fi
    ;;
  status)
    podman pod inspect --format '{{.State}}' "$pod_name"
    ;;
  logs)
    exec podman logs "$container_name"
    ;;
  *)
    echo "usage: $0 {apply|down|status|logs}" >&2
    exit 2
    ;;
esac
`,
		shellQuote(manifestPath),
		shellQuote(imageArchivePath),
		shellQuote(workloadName),
		shellQuote(containerName),
		shellQuote(spec.ImageReference),
		slcPulls.String(),
	)

	return []byte(script), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func parsePublishedPort(value string) int {
	value = strings.TrimSpace(value)
	index := strings.LastIndex(value, ":")
	if index < 0 {
		return 0
	}
	port, err := strconv.Atoi(value[index+1:])
	if err != nil || port < 1 || port > 65535 {
		return 0
	}

	return port
}
