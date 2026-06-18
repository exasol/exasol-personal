// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/saas"
	"github.com/spf13/cobra"
)

var saasMigrationFlags = struct {
	DB          string
	Schema      string
	TargetUser  string
	DryRun      bool
	ObjectsOnly bool
	DataOnly    bool
}{}

func newSaasMigrationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migration",
		Short: "Migrate the deployment into a SaaS database",
		Long: `Migrate the local deployment's schemas, tables, table data, and database
objects into a SaaS database identified by its UUID.

Objects (schemas, tables with distribution keys, views, scripts) are recreated on
the target, then table data is transferred with EXPORT ... INTO EXA. Per-table row
counts are validated after transfer. Connectivity is verified before any data moves.`,
		Example: `  exasol saas migration --db <db_uuid> --target-user migrator
  exasol saas migration --db <db_uuid> --schema SAMPLE --dry-run`,
		Args: cobra.NoArgs,
		RunE: runSaasMigration,
	}

	cmd.Flags().StringVar(&saasMigrationFlags.DB, "db", "", "Target SaaS database UUID")
	cmd.Flags().
		StringVar(&saasMigrationFlags.Schema, "schema", "", "Limit migration to one schema")
	cmd.Flags().StringVar(&saasMigrationFlags.TargetUser, "target-user", "", "SaaS database user")
	cmd.Flags().
		BoolVar(&saasMigrationFlags.DryRun, "dry-run", false, "Print planned statements only")
	cmd.Flags().
		BoolVar(&saasMigrationFlags.ObjectsOnly, "objects-only", false, "Recreate objects only")
	cmd.Flags().BoolVar(&saasMigrationFlags.DataOnly, "data-only", false, "Transfer data only")
	cmd.MarkFlagsMutuallyExclusive("objects-only", "data-only")
	_ = cmd.MarkFlagRequired("db")

	return cmd
}

func runSaasMigration(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	ctx := cmd.Context()
	deployment := commonFlags.Deployment()

	sctx, err := saasClientForCommand(deployment)
	if err != nil {
		return err
	}

	resolved, err := resolveMigrationTarget(ctx, sctx, deployment)
	if err != nil {
		return err
	}

	// The PAT is used as the database connection credential (an OpenID refresh
	// token that the driver redeems during login).
	token := sctx.Token

	// Verify connectivity before transferring any data (skip for a pure dry run).
	if !saasMigrationFlags.DryRun {
		checks, _, allOK := saas.RunConnectionTest(
			ctx,
			sctx.API,
			saas.DefaultDBFactory,
			sctx.Account,
			sctx.Region,
			saas.ConnTestInput{
				DBUUID:   saasMigrationFlags.DB,
				Username: saasMigrationFlags.TargetUser,
				Token:    token,
			},
		)
		printChecks(cmd, checks)
		if !allOK {
			return errors.New("aborting: SaaS connection test failed")
		}
	}

	source, err := openSourceDatabase(ctx, deployment)
	if err != nil {
		return err
	}
	defer source.Close()

	var target generaltypes.Databaser
	if !saasMigrationFlags.DryRun {
		target, err = saas.DefaultDBFactory(token, resolved.Host, resolved.Port)
		if err != nil {
			return err
		}
		if err := target.Connect(ctx); err != nil {
			return fmt.Errorf("connecting to saas target: %w", err)
		}
		defer target.Close()
	}

	engine := saas.NewEngine(source, target, resolved, token, cmd.OutOrStdout())
	report, err := engine.Run(ctx, saas.MigrateOptions{
		Schema:      saasMigrationFlags.Schema,
		DryRun:      saasMigrationFlags.DryRun,
		ObjectsOnly: saasMigrationFlags.ObjectsOnly,
		DataOnly:    saasMigrationFlags.DataOnly,
	})
	if err != nil {
		return err
	}

	printMigrationReport(cmd, report)

	return nil
}

func resolveMigrationTarget(
	ctx context.Context,
	sctx saasContext,
	deployment config.DeploymentDir,
) (*config.DeploymentSaaS, error) {
	if saasMigrationFlags.TargetUser == "" {
		return nil, errors.New("a SaaS database user is required: pass --target-user")
	}

	database, err := sctx.API.ResolveDatabase(ctx, saasMigrationFlags.DB)
	if err != nil {
		return nil, fmt.Errorf("resolving target database: %w", err)
	}

	target := saas.FromDatabase(sctx.Account, sctx.Region, saasMigrationFlags.TargetUser, database)
	if err := saas.SaveTarget(deployment, target); err != nil {
		return nil, err
	}

	return &target, nil
}

func openSourceDatabase(
	ctx context.Context,
	deployment config.DeploymentDir,
) (generaltypes.Databaser, error) {
	connectionInfo, err := config.ResolveConnectionInfo(deployment)
	if err != nil {
		return nil, err
	}

	source, err := connect.NewExasolConnection(deployment, connectionInfo, "sys", "",
		connectionInfo.InsecureSkipCertValidation)
	if err != nil {
		return nil, err
	}
	if err := source.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connecting to source database: %w", err)
	}

	return source, nil
}

func printMigrationReport(cmd *cobra.Command, report *saas.MigrationReport) {
	out := cmd.OutOrStdout()
	if saasMigrationFlags.DryRun {
		_, _ = fmt.Fprintln(out, "Dry run complete; no changes were made.")
		return
	}

	_, _ = fmt.Fprintf(
		out,
		"Migration complete: %d schema(s), %d table(s), %d view(s), %d script(s).\n",
		len(report.Schemas),
		len(report.Tables),
		len(report.Views),
		len(report.Scripts),
	)
	for table, count := range report.RowCounts {
		_, _ = fmt.Fprintf(out, "  %s: %d rows\n", table, count)
	}
	for _, action := range report.ManualActions {
		_, _ = fmt.Fprintf(out, "  manual: %s\n", action)
	}
}

func registerSaasMigrationCmd(parent *cobra.Command) {
	cmd := newSaasMigrationCmd()
	requireMinorVersionCompatibility(cmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(cmd)
	registerDeploymentDirFlag(cmd, commonFlags)
	parent.AddCommand(cmd)
}
