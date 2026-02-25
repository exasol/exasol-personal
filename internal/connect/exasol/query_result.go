// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

import (
	"errors"
	"fmt"

	"github.com/exasol/exasol-driver-go/pkg/types"
)

var (
	ErrNumColumns = errors.New("the result set doesn't have expected number of columns")
	ErrNumRows    = errors.New("the result set column doesn't have expected number of rows")
)

type QueryResult struct {
	columnNames []string
	rows        [][]string
}

func (qr *QueryResult) FromResultSet(resultSet *types.SqlQueryResponseResultSet) error {
	columnNames := qr.getColumnNames(resultSet)

	rows, err := qr.getRows(resultSet)
	if err != nil {
		return err
	}

	qr.columnNames = columnNames
	qr.rows = rows

	return nil
}

func (qr *QueryResult) ColumnNames() []string {
	return qr.columnNames
}

func (qr *QueryResult) Rows() [][]string {
	return qr.rows
}

func (*QueryResult) getColumnNames(resultSet *types.SqlQueryResponseResultSet) []string {
	header := make([]string, 0, resultSet.ResultSet.NumColumns)
	for _, col := range resultSet.ResultSet.Columns {
		header = append(header, col.Name)
	}

	return header
}

func (*QueryResult) getRows(resultSet *types.SqlQueryResponseResultSet) ([][]string, error) {
	numRows := resultSet.ResultSet.NumRows
	numColumns := resultSet.ResultSet.NumColumns

	// Validate that the database actually returned the expected
	// number of rows and columns.
	if len(resultSet.ResultSet.Data) != numColumns {
		return nil, fmt.Errorf(
			"%w: expected=%d got=%d",
			ErrNumColumns,
			numRows, len(resultSet.ResultSet.Data),
		)
	}

	for _, col := range resultSet.ResultSet.Data {
		if len(col) != numRows {
			return nil, fmt.Errorf("%w: expected=%d got=%d", ErrNumRows, numRows, len(col))
		}
	}

	data := make([][]string, 0, resultSet.ResultSet.NumRows)

	for row := range resultSet.ResultSet.NumRows {
		rowData := make([]string, 0, numColumns)

		for _, col := range resultSet.ResultSet.Data {
			rowData = append(rowData, fmt.Sprint(col[row]))
		}

		data = append(data, rowData)
	}

	return data, nil
}
