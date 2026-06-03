// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	prettyJSONResult = "{\n" +
		"  \"columns\": [\n" +
		"    \"ID\",\n" +
		"    \"NAME\"\n" +
		"  ],\n" +
		"  \"rows\": [\n" +
		"    [\n" +
		"      \"1\",\n" +
		"      \"Alice\"\n" +
		"    ],\n" +
		"    [\n" +
		"      \"2\",\n" +
		"      \"Bob\"\n" +
		"    ]\n" +
		"  ]\n" +
		"}\n"
	compactJSONResult = "{\"columns\":[\"ID\",\"NAME\"]," +
		"\"rows\":[[\"1\",\"Alice\"],[\"2\",\"Bob\"]]}\n"
	emptyPrettyJSON = "{\n" +
		"  \"columns\": [],\n" +
		"  \"rows\": []\n" +
		"}\n"
	unknownPrettyJSON = "{\n" +
		"  \"columns\": [\n" +
		"    \"ID\"\n" +
		"  ],\n" +
		"  \"rows\": [\n" +
		"    [\n" +
		"      \"1\"\n" +
		"    ]\n" +
		"  ]\n" +
		"}\n"
)

type stubQueryResult struct {
	columnNames []string
	rows        [][]string
	truncated   bool
}

func (s stubQueryResult) ColumnNames() []string {
	return s.columnNames
}

func (s stubQueryResult) Rows() [][]string {
	return s.rows
}

func (s stubQueryResult) Truncated() bool {
	return s.truncated
}

func TestPrintResultJSON(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		format     JSONFormat
		result     stubQueryResult
		expected   string
		normalized JSONFormat
	}{
		{
			name:   "renders columns and rows as pretty json",
			format: JSONFormatPretty,
			result: stubQueryResult{
				columnNames: []string{"ID", "NAME"},
				rows:        [][]string{{"1", "Alice"}, {"2", "Bob"}},
			},
			expected:   prettyJSONResult,
			normalized: JSONFormatPretty,
		},
		{
			name:   "renders columns and rows as compact json",
			format: JSONFormatCompact,
			result: stubQueryResult{
				columnNames: []string{"ID", "NAME"},
				rows:        [][]string{{"1", "Alice"}, {"2", "Bob"}},
			},
			expected:   compactJSONResult,
			normalized: JSONFormatCompact,
		},
		{
			name:   "renders empty results as empty arrays",
			format: JSONFormatPretty,
			result: stubQueryResult{
				columnNames: []string{},
				rows:        [][]string{},
			},
			expected:   emptyPrettyJSON,
			normalized: JSONFormatPretty,
		},
		{
			name:   "unknown format falls back to pretty json",
			format: JSONFormat("surprise"),
			result: stubQueryResult{
				columnNames: []string{"ID"},
				rows:        [][]string{{"1"}},
			},
			expected:   unknownPrettyJSON,
			normalized: JSONFormatPretty,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			// Given a query result that should be emitted as JSON.
			// When the result is rendered in the requested JSON mode.
			printer := newJSONResultPrinter(test.format)
			err := printer(&buf, test.result)

			// Then the output is valid JSON in the expected shape.
			require.NoError(t, err)
			require.Equal(t, test.expected, buf.String())
			require.Equal(t, test.normalized, normalizeJSONFormat(test.format))

			var decoded jsonQueryResult
			require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
			require.Equal(t, test.result.columnNames, decoded.Columns)
			require.Equal(t, test.result.rows, decoded.Rows)
		})
	}
}

func TestEffectiveMaxRows(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		requested   int
		modeDefault int
		expected    int
	}{
		{name: "unset, interactive", requested: MaxRowsUnset, modeDefault: 100, expected: 100},
		{name: "unset, unlimited", requested: MaxRowsUnset, modeDefault: 0, expected: 0},
		{name: "explicit over interactive", requested: 5, modeDefault: 100, expected: 5},
		{name: "explicit over unlimited", requested: 5, modeDefault: 0, expected: 5},
		{name: "explicit zero is unlimited", requested: 0, modeDefault: 100, expected: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.expected, effectiveMaxRows(test.requested, test.modeDefault))
		})
	}
}

func TestPrintTruncationFooter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	require.NoError(t, printTruncationFooter(&buf, 100))

	require.Equal(t,
		"-- showing first 100 rows (output truncated; use --max-rows 0 to see all)\n",
		buf.String(),
	)
}

func TestParseJSONFormat(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		input     string
		expected  JSONFormat
		wantError bool
	}{
		{name: "empty defaults to pretty", input: "", expected: JSONFormatPretty},
		{name: "pretty accepted", input: "pretty", expected: JSONFormatPretty},
		{name: "compact accepted", input: "compact", expected: JSONFormatCompact},
		{name: "case and whitespace normalized", input: "  PRETTY  ", expected: JSONFormatPretty},
		{name: "invalid value rejected", input: "yaml", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			format, err := ParseJSONFormat(test.input)
			if test.wantError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expected, format)
		})
	}
}
