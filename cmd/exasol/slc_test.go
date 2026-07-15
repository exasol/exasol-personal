// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/deploy"
)

// On an unsupported architecture `slc list` degrades gracefully: SLCStatuses returns an
// empty set (no error), which the renderers must present as the "none available" message
// and an empty JSON array — never an error or a non-zero exit.

//nolint:paralleltest // swaps the process-wide os.Stdout to capture safePrint output.
func TestRenderSLCListTextNoneAvailable(t *testing.T) {
	output := captureStdout(t, func() {
		renderSLCListText([]deploy.SLCStatus{})
	})

	if strings.TrimSpace(
		output,
	) != "No script language containers are available for this platform." {
		t.Fatalf("unexpected text output: %q", output)
	}
}

func TestRenderSLCListJSONEmptyIsArray(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := renderSLCListJSON(&buf, []deploy.SLCStatus{}); err != nil {
		t.Fatalf("expected json render to succeed, got %v", err)
	}

	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Fatalf("expected empty JSON array, got %q", got)
	}
}

// captureStdout runs emit with os.Stdout redirected to a pipe and returns what it wrote.
func captureStdout(t *testing.T, emit func()) string {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = original })

	emit()
	_ = writer.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("failed to read captured output: %v", err)
	}

	return buf.String()
}
