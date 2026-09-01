// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

const EmbeddedURLScheme = "embedded://"

// EmbeddedSource: injected data keeps generation independent of earlier build output.
type EmbeddedSource struct {
	blobs fs.FS
}

func (*EmbeddedSource) Handles(loc Locator) bool {
	return strings.HasPrefix(loc.URL, EmbeddedURLScheme)
}

func (*EmbeddedSource) Probe(_ context.Context, _ Locator) (Probe, error) {
	return Probe{}, nil
}

// Fetch: streaming avoids loading large container images into memory.
func (e *EmbeddedSource) Fetch(_ context.Context, loc Locator, dstPath string) error {
	name := strings.TrimPrefix(loc.URL, EmbeddedURLScheme)
	if e.blobs == nil {
		return fmt.Errorf("no embedded data available for %q", name)
	}

	source, err := e.blobs.Open(name)
	if err != nil {
		return fmt.Errorf("no embedded data for %q: %w", name, err)
	}
	defer func() { _ = source.Close() }()

	target, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() { _ = target.Close() }()

	if _, err := io.Copy(target, source); err != nil {
		return err
	}

	return target.Close()
}
