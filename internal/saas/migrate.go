// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package saas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
)

// migrationConnectionName is the EXA connection created on the source database
// that points at the SaaS migration target.
const migrationConnectionName = "EXASOL_SAAS_MIGRATION_TARGET"

// MigrateOptions controls a migration run.
type MigrateOptions struct {
	// Schema limits the migration to a single schema; empty migrates all
	// non-system schemas.
	Schema string
	// DryRun prints the planned statements without executing them.
	DryRun bool
	// ObjectsOnly recreates objects without transferring table data.
	ObjectsOnly bool
	// DataOnly transfers table data without recreating objects.
	DataOnly bool
}

// tableInfo describes a source table to migrate.
type tableInfo struct {
	schema  string
	name    string
	columns []columnInfo
}

type columnInfo struct {
	name           string
	dataType       string
	distributedKey bool
}

// MigrationReport summarizes a completed migration.
type MigrationReport struct {
	Schemas       []string
	Tables        []string
	Views         []string
	Scripts       []string
	RowCounts     map[string]int64 // qualified table -> target row count
	ManualActions []string         // objects whose secrets cannot be migrated
}

// Engine migrates a source database into a SaaS target. The source runs
// EXPORT ... INTO EXA; objects are replayed directly on the target.
type Engine struct {
	Source   generaltypes.Databaser
	Target   generaltypes.Databaser
	resolved *config.DeploymentSaaS
	pw       string
	out      io.Writer
}

// NewEngine builds a migration engine. Both databases must already be connected
// (Target may be nil for a dry run).
func NewEngine(
	source, target generaltypes.Databaser,
	resolved *config.DeploymentSaaS,
	targetPassword string,
	out io.Writer,
) *Engine {
	return &Engine{Source: source, Target: target, resolved: resolved, pw: targetPassword, out: out}
}

// Run executes the migration in dependency order and returns a report.
func (e *Engine) Run(ctx context.Context, opts MigrateOptions) (*MigrationReport, error) {
	report := &MigrationReport{RowCounts: map[string]int64{}}

	slog.Info("starting migration",
		"schema", opts.Schema, "dryRun", opts.DryRun,
		"objectsOnly", opts.ObjectsOnly, "dataOnly", opts.DataOnly)

	schemas, err := e.listSchemas(ctx, opts.Schema)
	if err != nil {
		return nil, err
	}
	report.Schemas = schemas
	slog.Info("enumerated source schemas", "count", len(schemas), "schemas", schemas)

	if !opts.DataOnly {
		slog.Info("recreating schemas on target", "count", len(schemas))
		if err := e.recreateSchemas(ctx, opts, schemas); err != nil {
			return nil, err
		}
	}

	for _, schema := range schemas {
		tables, err := e.listTables(ctx, schema)
		if err != nil {
			return nil, err
		}
		slog.Info("enumerated tables", "schema", schema, "count", len(tables))

		if !opts.DataOnly {
			slog.Info("recreating tables on target", "schema", schema, "count", len(tables))
			if err := e.recreateTables(ctx, opts, tables); err != nil {
				return nil, err
			}
		}
		for _, t := range tables {
			report.Tables = append(report.Tables, qualify(t.schema, t.name))
		}

		if !opts.ObjectsOnly {
			if err := e.ensureExportConnection(ctx, opts); err != nil {
				return nil, err
			}
			slog.Info("transferring table data", "schema", schema, "count", len(tables))
			if err := e.exportTables(ctx, opts, tables, report); err != nil {
				return nil, err
			}
		}
	}

	if !opts.DataOnly {
		slog.Info("recreating views and scripts on target")
		if err := e.recreateViewsAndScripts(ctx, opts, schemas, report); err != nil {
			return nil, err
		}
		e.reportManualObjects(ctx, report)
	}

	slog.Info("migration finished",
		"schemas", len(report.Schemas), "tables", len(report.Tables),
		"views", len(report.Views), "scripts", len(report.Scripts), "dryRun", opts.DryRun)

	return report, nil
}

// --- enumeration ---

func (e *Engine) listSchemas(ctx context.Context, only string) ([]string, error) {
	if strings.TrimSpace(only) != "" {
		return []string{strings.ToUpper(strings.TrimSpace(only))}, nil
	}

	rows, err := e.querySource(ctx,
		`SELECT SCHEMA_NAME FROM EXA_ALL_SCHEMAS `+
			`WHERE SCHEMA_NAME NOT LIKE 'EXA\_%' ESCAPE '\' ORDER BY SCHEMA_NAME`)
	if err != nil {
		return nil, fmt.Errorf("listing schemas: %w", err)
	}

	return firstColumn(rows), nil
}

func (e *Engine) listTables(ctx context.Context, schema string) ([]tableInfo, error) {
	rows, err := e.querySource(ctx,
		fmt.Sprintf(`SELECT TABLE_NAME FROM EXA_ALL_TABLES `+
			`WHERE TABLE_SCHEMA = '%s' ORDER BY TABLE_NAME`, escapeLiteral(schema)))
	if err != nil {
		return nil, fmt.Errorf("listing tables in %s: %w", schema, err)
	}

	var tables []tableInfo
	for _, name := range firstColumn(rows) {
		columns, err := e.listColumns(ctx, schema, name)
		if err != nil {
			return nil, err
		}
		tables = append(tables, tableInfo{schema: schema, name: name, columns: columns})
	}

	return tables, nil
}

func (e *Engine) listColumns(ctx context.Context, schema, table string) ([]columnInfo, error) {
	rows, err := e.querySource(ctx,
		fmt.Sprintf(`SELECT COLUMN_NAME, COLUMN_TYPE, COLUMN_IS_DISTRIBUTION_KEY `+
			`FROM EXA_ALL_COLUMNS WHERE COLUMN_SCHEMA = '%s' AND COLUMN_TABLE = '%s' `+
			`ORDER BY COLUMN_ORDINAL_POSITION`, escapeLiteral(schema), escapeLiteral(table)))
	if err != nil {
		return nil, fmt.Errorf("listing columns of %s: %w", qualify(schema, table), err)
	}

	columns := make([]columnInfo, 0, len(rows))
	for _, row := range rows {
		if len(row) < 3 { //nolint:mnd
			continue
		}
		columns = append(columns, columnInfo{
			name:           row[0],
			dataType:       row[1],
			distributedKey: isTrue(row[2]),
		})
	}

	return columns, nil
}

// --- object recreation ---

func (e *Engine) recreateSchemas(ctx context.Context, opts MigrateOptions, schemas []string) error {
	for _, schema := range schemas {
		stmt := "CREATE SCHEMA IF NOT EXISTS " + quoteIdent(schema)
		if err := e.runTarget(ctx, opts, "schema", stmt); err != nil {
			return err
		}
	}

	return nil
}

func (e *Engine) recreateTables(
	ctx context.Context,
	opts MigrateOptions,
	tables []tableInfo,
) error {
	for _, t := range tables {
		stmt := createTableStatement(t)
		if err := e.runTarget(ctx, opts, "table", stmt); err != nil {
			return err
		}
	}

	return nil
}

func (e *Engine) recreateViewsAndScripts(
	ctx context.Context,
	opts MigrateOptions,
	schemas []string,
	report *MigrationReport,
) error {
	for _, schema := range schemas {
		views, err := e.querySource(ctx,
			fmt.Sprintf(`SELECT VIEW_NAME, VIEW_TEXT FROM EXA_ALL_VIEWS `+
				`WHERE VIEW_SCHEMA = '%s' ORDER BY VIEW_NAME`, escapeLiteral(schema)))
		if err != nil {
			return fmt.Errorf("listing views in %s: %w", schema, err)
		}
		for _, row := range views {
			if len(row) < 2 { //nolint:mnd
				continue
			}
			if err := e.runTarget(ctx, opts, "view", row[1]); err != nil {
				return err
			}
			report.Views = append(report.Views, qualify(schema, row[0]))
		}

		scripts, err := e.querySource(ctx,
			fmt.Sprintf(`SELECT SCRIPT_NAME, SCRIPT_TEXT FROM EXA_ALL_SCRIPTS `+
				`WHERE SCRIPT_SCHEMA = '%s' ORDER BY SCRIPT_NAME`, escapeLiteral(schema)))
		if err != nil {
			return fmt.Errorf("listing scripts in %s: %w", schema, err)
		}
		for _, row := range scripts {
			if len(row) < 2 { //nolint:mnd
				continue
			}
			if err := e.runTarget(ctx, opts, "script", row[1]); err != nil {
				return err
			}
			report.Scripts = append(report.Scripts, qualify(schema, row[0]))
		}
	}

	return nil
}

// reportManualObjects records objects whose secrets cannot be read from the
// source and therefore must be recreated by hand on the target. They are never
// fabricated.
func (e *Engine) reportManualObjects(ctx context.Context, report *MigrationReport) {
	conns, err := e.querySource(
		ctx,
		`SELECT CONNECTION_NAME FROM EXA_DBA_CONNECTIONS ORDER BY CONNECTION_NAME`,
	)
	if err != nil {
		// Connections are visible only with DBA privileges; absence is not fatal.
		return
	}
	for _, name := range firstColumn(conns) {
		if name == migrationConnectionName {
			continue
		}
		report.ManualActions = append(
			report.ManualActions,
			fmt.Sprintf(
				"connection %q: recreate on target with its secret (not readable from source)",
				name,
			),
		)
	}
}

// --- data transfer ---

func (e *Engine) ensureExportConnection(ctx context.Context, opts MigrateOptions) error {
	stmt := fmt.Sprintf(
		"CREATE OR REPLACE CONNECTION %s TO '%s' USER '%s' IDENTIFIED BY '%s'",
		migrationConnectionName, targetConnectionString(e.resolved), e.resolved.Username, e.pw)
	display := fmt.Sprintf(
		"CREATE OR REPLACE CONNECTION %s TO '%s' USER '%s' IDENTIFIED BY '***'",
		migrationConnectionName, targetConnectionString(e.resolved), e.resolved.Username)

	if opts.DryRun {
		e.printPlanned("connection", display)
		return nil
	}
	if _, err := e.Source.Exec(ctx, stmt, 0); err != nil {
		return fmt.Errorf("creating export connection on source: %w", err)
	}

	return nil
}

func (e *Engine) exportTables(
	ctx context.Context,
	opts MigrateOptions,
	tables []tableInfo,
	report *MigrationReport,
) error {
	for _, table := range tables {
		qualified := qualify(table.schema, table.name)
		// AT <name> references the named connection; quoting it would make Exasol
		// treat the value as an inline connection string instead.
		stmt := fmt.Sprintf("EXPORT %s INTO EXA AT %s TABLE %s TRUNCATE",
			qualifiedIdent(table), migrationConnectionName, qualifiedIdent(table))

		if opts.DryRun {
			e.printPlanned("data", stmt)
			continue
		}
		slog.Info("exporting table", "table", qualified)
		if _, err := e.Source.Exec(ctx, stmt, 0); err != nil {
			return fmt.Errorf("exporting %s: %w", qualified, err)
		}
		if err := e.validateRowCount(ctx, table, report); err != nil {
			return err
		}
		slog.Info("exported table", "table", qualified, "rows", report.RowCounts[qualified])
	}

	return nil
}

func (e *Engine) validateRowCount(
	ctx context.Context,
	table tableInfo,
	report *MigrationReport,
) error {
	qualified := qualify(table.schema, table.name)
	countSQL := "SELECT COUNT(*) FROM " + qualifiedIdent(table)

	sourceCount, err := e.scalarCount(ctx, e.Source, countSQL)
	if err != nil {
		return fmt.Errorf("counting source rows of %s: %w", qualified, err)
	}
	targetCount, err := e.scalarCount(ctx, e.Target, countSQL)
	if err != nil {
		return fmt.Errorf("counting target rows of %s: %w", qualified, err)
	}

	report.RowCounts[qualified] = targetCount
	if sourceCount != targetCount {
		return fmt.Errorf("row count mismatch for %s: source=%d target=%d",
			qualified, sourceCount, targetCount)
	}

	return nil
}

// --- low-level helpers ---

func (e *Engine) querySource(ctx context.Context, sql string) ([][]string, error) {
	result, err := e.Source.Exec(ctx, sql, 0)
	if err != nil {
		return nil, err
	}

	return result.Rows(), nil
}

func (*Engine) scalarCount(
	ctx context.Context,
	database generaltypes.Databaser,
	sql string,
) (int64, error) {
	result, err := database.Exec(ctx, sql, 1)
	if err != nil {
		return 0, err
	}
	rows := result.Rows()
	if len(rows) == 0 || len(rows[0]) == 0 {
		return 0, errors.New("count query returned no value")
	}

	return strconv.ParseInt(strings.TrimSpace(rows[0][0]), 10, 64)
}

// runTarget executes (or, in dry-run, prints) an object-DDL statement on the target.
func (e *Engine) runTarget(ctx context.Context, opts MigrateOptions, phase, stmt string) error {
	if opts.DryRun {
		e.printPlanned(phase, stmt)
		return nil
	}
	if _, err := e.Target.Exec(ctx, stmt, 0); err != nil {
		return fmt.Errorf("recreating %s on target: %w", phase, err)
	}

	return nil
}

func (e *Engine) printPlanned(phase, stmt string) {
	if e.out == nil {
		return
	}
	_, _ = fmt.Fprintf(e.out, "[%s] %s;\n", phase, strings.TrimSpace(stmt))
}

// --- SQL generation ---

func createTableStatement(table tableInfo) string {
	var builder strings.Builder
	_, _ = fmt.Fprintf(&builder, "CREATE OR REPLACE TABLE %s (\n", qualifiedIdent(table))

	var distribution []string
	for i, col := range table.columns {
		_, _ = fmt.Fprintf(&builder, "  %s %s", quoteIdent(col.name), col.dataType)
		if i < len(table.columns)-1 {
			_, _ = builder.WriteString(",")
		}
		_, _ = builder.WriteString("\n")
		if col.distributedKey {
			distribution = append(distribution, quoteIdent(col.name))
		}
	}

	if len(distribution) > 0 {
		_, _ = fmt.Fprintf(&builder, "  , DISTRIBUTE BY %s\n", strings.Join(distribution, ", "))
	}
	_, _ = builder.WriteString(")")

	return builder.String()
}

func qualify(schema, name string) string {
	return schema + "." + name
}

func qualifiedIdent(table tableInfo) string {
	return quoteIdent(table.schema) + "." + quoteIdent(table.name)
}

// quoteIdent double-quotes an identifier, escaping embedded quotes.
func quoteIdent(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// escapeLiteral escapes single quotes for use inside an SQL string literal.
func escapeLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func firstColumn(rows [][]string) []string {
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		if len(row) > 0 {
			values = append(values, row[0])
		}
	}

	return values
}

func isTrue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "t", "1", "yes", "y":
		return true
	default:
		return false
	}
}
