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
	// Readable only by its owner for the short time the file exists.
	envFileMode = 0o600
	envDirMode  = 0o700
)

func noCleanup() {
	// Always safe to defer, whether or not a file was written.
}

// Beside the deployment, so a runtime behind a VM boundary reaches it too.
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
