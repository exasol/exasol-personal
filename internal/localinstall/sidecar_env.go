// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	// Resolved connection values stay readable only by their owner for the
	// short time the file exists.
	envFileMode = 0o600
	envDirMode  = 0o700
)

func noCleanup() {
	// No file was written, so callers can defer the returned cleanup without
	// first checking whether there is anything to remove.
}

// envFile writes resolved values where the runtime reads them and returns the path
// to pass to --env-file, alongside the removal the caller defers. The file
// lives beside the deployment rather than in a shared temporary directory so
// a runtime behind a virtual machine boundary can reach it too.
func envFile(directory RuntimePath, name string, values map[string]string) (
	RuntimePath, func(), error,
) {
	if directory.HostPath == "" || directory.RuntimePath == "" {
		return RuntimePath{}, noCleanup, errors.New(
			"sidecar runtime was created without an environment directory",
		)
	}
	content, err := formatEnvFile(values)
	if err != nil {
		return RuntimePath{}, noCleanup, err
	}
	if err := os.MkdirAll(directory.HostPath, envDirMode); err != nil {
		return RuntimePath{}, noCleanup, fmt.Errorf("prepare environment directory: %w", err)
	}
	hostPath := filepath.Join(directory.HostPath, name+".env")
	if err := os.WriteFile(hostPath, content, envFileMode); err != nil {
		return RuntimePath{}, noCleanup, fmt.Errorf("write environment file: %w", err)
	}

	path := RuntimePath{
		HostPath:    hostPath,
		RuntimePath: directory.RuntimePath + "/" + name + ".env",
	}

	return path, func() { _ = os.Remove(hostPath) }, nil
}

// formatEnvFile rejects values the one-line-per-variable format cannot carry,
// naming the variable so the user can correct the source it came from.
func formatEnvFile(values map[string]string) ([]byte, error) {
	var content bytes.Buffer
	for _, name := range slices.Sorted(maps.Keys(values)) {
		value := values[name]
		if strings.ContainsAny(value, "\n\r\x00") {
			return nil, fmt.Errorf(
				"environment value for %s contains a line break and cannot be passed "+
					"to the container", name,
			)
		}
		_, _ = fmt.Fprintf(&content, "%s=%s\n", name, value)
	}

	return content.Bytes(), nil
}
