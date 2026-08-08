// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tablewriter

import (
	"bytes"
	"strings"
	"testing"
)

func TestTableWriter_RendersHeaderAndRows(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	table := New(&buffer)

	table.SetHeader([]string{"ID", "NAME"})
	if err := table.SetRows([][]string{{"1", "Alice"}, {"2", "Bob"}}); err != nil {
		t.Fatalf("expected no error setting rows, got %v", err)
	}
	if err := table.Render(); err != nil {
		t.Fatalf("expected no error rendering, got %v", err)
	}

	output := buffer.String()
	for _, want := range []string{"ID", "NAME", "1", "Alice", "2", "Bob"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected rendered output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestTableWriter_RendersEmptyTableWithoutError(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	table := New(&buffer)

	table.SetHeader([]string{"ID"})
	if err := table.Render(); err != nil {
		t.Fatalf("expected no error rendering an empty table, got %v", err)
	}
	if !strings.Contains(buffer.String(), "ID") {
		t.Fatalf("expected the header to still be rendered, got:\n%s", buffer.String())
	}
}
