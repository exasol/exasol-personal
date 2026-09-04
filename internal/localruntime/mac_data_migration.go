// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/exasol/exasol-personal/internal/localinstall"
)

const (
	vmSharedNanoDataDir        = "/mnt/host/exa"
	vmNanoDataBackupDir        = "/var/lib/exa.migrated-backup"
	vmSharedNanoDataStagingDir = "/mnt/host/.exa-migration"
	hostLayoutMarkerName       = ".exasol-personal-host-layout"
	hostLayoutMarkerContents   = "1\n"
)

type macDataMigration struct {
	environment localinstall.ExecutionEnvironment
	sourceDir   string
	targetDir   string
	stagingDir  string
	backupDir   string
}

func newMacDataMigration(environment localinstall.ExecutionEnvironment) *macDataMigration {
	return &macDataMigration{
		environment: environment,
		sourceDir:   vmNanoDataDir,
		targetDir:   vmSharedNanoDataDir,
		stagingDir:  vmSharedNanoDataStagingDir,
		backupDir:   vmNanoDataBackupDir,
	}
}

func (migration *macDataMigration) prepare(
	ctx context.Context,
	out, outErr io.Writer,
	stopNano func() error,
) error {
	targetComplete, err := migration.markerMatches(
		ctx, path.Join(migration.targetDir, hostLayoutMarkerName),
	)
	if err != nil {
		return err
	}
	sourceExists, err := migration.environment.PathExists(ctx, migration.sourceDir)
	if err != nil {
		return fmt.Errorf("failed to inspect legacy Nano data at %s: %w", migration.sourceDir, err)
	}
	sourcePopulated, err := migration.environment.DirectoryHasEntries(ctx, migration.sourceDir)
	if err != nil {
		return fmt.Errorf("failed to inspect legacy Nano data at %s: %w", migration.sourceDir, err)
	}
	if targetComplete {
		if sourceExists {
			if err := stopNano(); err != nil {
				return fmt.Errorf("failed to stop Nano before using host data at %s: %w",
					migration.targetDir, err)
			}
		}

		return nil
	}

	targetPopulated, err := migration.environment.DirectoryHasEntries(ctx, migration.targetDir)
	if err != nil {
		return fmt.Errorf("failed to inspect host Nano data at %s: %w", migration.targetDir, err)
	}
	if sourcePopulated && targetPopulated {
		return fmt.Errorf(
			"refusing to migrate Nano data because both %s and %s contain data; "+
				"leave one location empty and retry",
			migration.sourceDir,
			migration.targetDir,
		)
	}
	if !sourcePopulated {
		if sourceExists {
			if err := stopNano(); err != nil {
				return fmt.Errorf("failed to stop Nano before using host data at %s: %w",
					migration.targetDir, err)
			}
		}

		return nil
	}
	if err := stopNano(); err != nil {
		return fmt.Errorf("failed to stop Nano before migrating %s to %s: %w",
			migration.sourceDir, migration.targetDir, err)
	}

	return migration.copyAndPublish(ctx, out, outErr)
}

func (migration *macDataMigration) copyAndPublish(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	readyToPublish, err := migration.resetOrResumeStaging(ctx)
	if err != nil {
		return err
	}
	if readyToPublish {
		return migration.publish(ctx)
	}
	if err := migration.environment.MkdirAll(ctx, migration.stagingDir, dirMode); err != nil {
		return fmt.Errorf("failed to create migration staging directory %s: %w",
			migration.stagingDir, err)
	}
	if err := migration.environment.Run(
		ctx,
		nil,
		out,
		outErr,
		"cp",
		"-a",
		"--sparse=always",
		migration.sourceDir+"/.",
		migration.stagingDir+"/",
	); err != nil {
		return fmt.Errorf(
			"failed to copy legacy Nano data from %s to %s: %w",
			migration.sourceDir,
			migration.targetDir,
			err,
		)
	}
	if err := migration.environment.Sync(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to flush migrated Nano data from %s to %s: %w",
			migration.sourceDir, migration.targetDir, err)
	}
	if err := migration.environment.WriteFileAtomically(
		ctx,
		path.Join(migration.stagingDir, hostLayoutMarkerName),
		[]byte(hostLayoutMarkerContents),
		dirMode,
		markerFileMode,
	); err != nil {
		return fmt.Errorf("failed to mark migrated Nano data at %s: %w",
			migration.stagingDir, err)
	}
	if err := migration.environment.Sync(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to flush completed migration at %s: %w",
			migration.stagingDir, err)
	}

	return migration.publish(ctx)
}

func (migration *macDataMigration) resetOrResumeStaging(ctx context.Context) (bool, error) {
	stagingExists, err := migration.environment.PathExists(ctx, migration.stagingDir)
	if err != nil {
		return false, fmt.Errorf("failed to inspect migration staging directory %s: %w",
			migration.stagingDir, err)
	}
	if !stagingExists {
		return false, nil
	}
	complete, err := migration.markerMatches(
		ctx, path.Join(migration.stagingDir, hostLayoutMarkerName),
	)
	if err != nil {
		return false, err
	}
	if complete {
		return true, nil
	}
	if err := migration.environment.RemoveAll(ctx, migration.stagingDir); err != nil {
		return false, fmt.Errorf("failed to reset migration staging directory %s: %w",
			migration.stagingDir, err)
	}

	return false, nil
}

func (migration *macDataMigration) publish(ctx context.Context) error {
	targetExists, err := migration.environment.PathExists(ctx, migration.targetDir)
	if err != nil {
		return fmt.Errorf("failed to inspect host Nano data at %s: %w", migration.targetDir, err)
	}
	if targetExists {
		targetPopulated, inspectErr := migration.environment.DirectoryHasEntries(
			ctx, migration.targetDir,
		)
		if inspectErr != nil {
			return fmt.Errorf("failed to inspect host Nano data at %s: %w",
				migration.targetDir, inspectErr)
		}
		if targetPopulated {
			return fmt.Errorf("refusing to publish migrated Nano data because %s contains data",
				migration.targetDir)
		}
		if err := migration.environment.RemoveDir(ctx, migration.targetDir); err != nil {
			return fmt.Errorf("failed to remove empty host Nano directory %s: %w",
				migration.targetDir, err)
		}
	}
	if err := migration.environment.Rename(
		ctx, migration.stagingDir, migration.targetDir,
	); err != nil {
		return fmt.Errorf("failed to publish migrated Nano data from %s to %s: %w",
			migration.sourceDir, migration.targetDir, err)
	}

	return nil
}

func (migration *macDataMigration) finalize(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	complete, err := migration.markerMatches(
		ctx, path.Join(migration.targetDir, hostLayoutMarkerName),
	)
	if err != nil {
		return err
	}
	if !complete {
		if err := migration.environment.WriteFileAtomically(
			ctx,
			path.Join(migration.targetDir, hostLayoutMarkerName),
			[]byte(hostLayoutMarkerContents),
			dirMode,
			markerFileMode,
		); err != nil {
			return fmt.Errorf("failed to record host Nano layout at %s: %w",
				migration.targetDir, err)
		}
	}
	sourceExists, err := migration.environment.PathExists(ctx, migration.sourceDir)
	if err != nil {
		return fmt.Errorf("failed to inspect legacy Nano data at %s: %w", migration.sourceDir, err)
	}
	if sourceExists {
		backupExists, inspectErr := migration.environment.PathExists(ctx, migration.backupDir)
		if inspectErr != nil {
			return fmt.Errorf("failed to inspect migration backup at %s: %w",
				migration.backupDir, inspectErr)
		}
		if backupExists {
			return fmt.Errorf(
				"refusing to replace migration backup %s while retiring legacy Nano data at %s",
				migration.backupDir,
				migration.sourceDir,
			)
		}
		if err := migration.environment.Rename(
			ctx, migration.sourceDir, migration.backupDir,
		); err != nil {
			return fmt.Errorf("failed to retain legacy Nano data from %s at %s: %w",
				migration.sourceDir, migration.backupDir, err)
		}
	}
	if err := migration.environment.Sync(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to flush completed host Nano layout at %s: %w",
			migration.targetDir, err)
	}

	return nil
}

func (migration *macDataMigration) markerMatches(
	ctx context.Context,
	markerPath string,
) (bool, error) {
	exists, err := migration.environment.PathExists(ctx, markerPath)
	if err != nil || !exists {
		return false, err
	}
	err = migration.environment.Run(
		ctx,
		nil,
		io.Discard,
		io.Discard,
		"sh",
		"-c",
		`actual=$(cat "$1"); [ "$actual" = "$2" ]`,
		"sh",
		markerPath,
		strings.TrimSuffix(hostLayoutMarkerContents, "\n"),
	)
	if err == nil {
		return true, nil
	}
	if commandExitedWith(err, 1) {
		return false, nil
	}

	return false, fmt.Errorf("failed to read migration marker %s: %w", markerPath, err)
}
