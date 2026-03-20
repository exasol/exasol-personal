// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package shared

import (
	"fmt"
	"io"
	"strings"
)

// RenderTable prints a fixed-width table given headers, widths and row data.
// Values longer than width are truncated with an ellipsis (last 1 char becomes '…').
// nolint: revive
func RenderTable(writer io.Writer, headers []string, widths []int, rows [][]string) {
	// Header
	for i, h := range headers {
		fmt.Fprint(writer, formatCell(strings.ToUpper(h), widths[i]))
		if i < len(headers)-1 {
			fmt.Fprint(writer, " ")
		}
	}
	fmt.Fprint(writer, "\n")
	// Separator line
	for i := range headers {
		fmt.Fprint(writer, strings.Repeat("-", widths[i]))
		if i < len(headers)-1 {
			fmt.Fprint(writer, " ")
		}
	}
	fmt.Fprint(writer, "\n")
	// Rows
	for _, r := range rows {
		for i, val := range r {
			fmt.Fprint(writer, formatCell(val, widths[i]))
			if i < len(r)-1 {
				fmt.Fprint(writer, " ")
			}
		}
		fmt.Fprint(writer, "\n")
	}
}

func formatCell(str string, width int) string {
	if width <= 0 {
		return str
	}
	if len(str) > width { // truncate with ellipsis if possible
		if width > 1 {
			return str[:width-1] + "…"
		}

		return str[:width]
	}

	return str + strings.Repeat(" ", width-len(str))
}
