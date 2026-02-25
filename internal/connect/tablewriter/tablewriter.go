// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tablewriter

import (
	"io"
	"log/slog"

	"github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
)

type TableWriter struct {
	table *tablewriter.Table
}

func New(writer io.Writer) types.TableFormatter {
	table := tablewriter.NewWriter(writer).Options(
		tablewriter.WithHeaderAutoFormat(tw.Off),
	)

	termWidth, ok := util.GetTerminalWidth()
	if ok {
		const widthOffset = 2
		width := termWidth - widthOffset

		// It's actually terminal width minus offset but no need to mention that in the log.
		slog.Debug("limiting the rendered table width by the terminal width", "width", width)

		table.Options(tablewriter.WithColumnMax(width))
	}

	return &TableWriter{table}
}

func (twr *TableWriter) SetHeader(header []string) {
	twr.table.Header(header)
}

func (twr *TableWriter) SetRows(rows [][]string) error {
	return twr.table.Bulk(rows)
}

func (twr *TableWriter) Render() error {
	return twr.table.Render()
}
