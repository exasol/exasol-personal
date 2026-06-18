// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package saas

import (
	"bytes"
	"context"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/stretchr/testify/require"
)

const sampleSchema = "SAMPLE"

func TestCreateTableStatement_IncludesDistributionKey(t *testing.T) {
	t.Parallel()

	stmt := createTableStatement(tableInfo{
		schema: sampleSchema,
		name:   "PRODUCTS",
		columns: []columnInfo{
			{name: "PRODUCT_ID", dataType: "DECIMAL(18,0)", distributedKey: true},
			{name: "PRICE_USD", dataType: "DOUBLE"},
		},
	})

	require.Contains(t, stmt, `CREATE OR REPLACE TABLE "SAMPLE"."PRODUCTS"`)
	require.Contains(t, stmt, `"PRODUCT_ID" DECIMAL(18,0)`)
	require.Contains(t, stmt, `"PRICE_USD" DOUBLE`)
	require.Contains(t, stmt, `DISTRIBUTE BY "PRODUCT_ID"`)
}

func TestCreateTableStatement_NoDistributionKey(t *testing.T) {
	t.Parallel()

	stmt := createTableStatement(tableInfo{
		schema:  "S",
		name:    "T",
		columns: []columnInfo{{name: "C", dataType: "VARCHAR(10) UTF8"}},
	})

	require.NotContains(t, stmt, "DISTRIBUTE BY")
}

func TestQuoteIdentEscapesQuotes(t *testing.T) {
	t.Parallel()
	require.Equal(t, `"a""b"`, quoteIdent(`a"b`))
}

func TestIsTrue(t *testing.T) {
	t.Parallel()
	require.True(t, isTrue("TRUE"))
	require.True(t, isTrue("1"))
	require.False(t, isTrue("false"))
	require.False(t, isTrue(""))
}

func TestEngineRun_DryRunPlansStatementsWithoutExecuting(t *testing.T) {
	t.Parallel()

	source := &fakeDB{responses: map[string]fakeResult{
		"EXA_ALL_SCHEMAS": {rows: [][]string{{sampleSchema}}},
		"EXA_ALL_TABLES":  {rows: [][]string{{"PRODUCTS"}}},
		"EXA_ALL_COLUMNS": {rows: [][]string{
			{"PRODUCT_ID", "DECIMAL(18,0)", "true"},
			{"PRICE_USD", "DOUBLE", "false"},
		}},
		"EXA_ALL_VIEWS":       {rows: [][]string{}},
		"EXA_ALL_SCRIPTS":     {rows: [][]string{}},
		"EXA_DBA_CONNECTIONS": {rows: [][]string{}},
	}}

	target := &config.DeploymentSaaS{
		Host: "host", Port: 8563, Username: "migrator",
	}
	var out bytes.Buffer
	engine := NewEngine(source, nil, target, "secret", &out)

	report, err := engine.Run(context.Background(), MigrateOptions{DryRun: true})
	require.NoError(t, err)
	require.Equal(t, []string{sampleSchema}, report.Schemas)

	planned := out.String()
	require.Contains(t, planned, `CREATE SCHEMA IF NOT EXISTS "SAMPLE"`)
	require.Contains(t, planned, `CREATE OR REPLACE TABLE "SAMPLE"."PRODUCTS"`)
	require.Contains(t, planned, `DISTRIBUTE BY "PRODUCT_ID"`)
	require.Contains(
		t,
		planned,
		`EXPORT "SAMPLE"."PRODUCTS" INTO EXA AT `+migrationConnectionName+` TABLE`,
	)
	// The target password must never appear in planned output.
	require.NotContains(t, planned, "secret")

	// Dry run must not execute DDL against the source beyond read-only catalog queries.
	for _, q := range source.executed {
		require.NotContains(t, q, "EXPORT ")
		require.NotContains(t, q, "CREATE OR REPLACE CONNECTION")
	}
}
