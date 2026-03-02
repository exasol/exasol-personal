// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"log/slog"
)

// MaybeLogVersionUpdateHint performs a best-effort silent version check and logs a
// hint if an update is available.
//
// Design decision: this lives in the deploy package because it relies on the
// version-checking mechanism and its locking semantics. The cmd layer should not
// need to understand those details.
func MaybeLogVersionUpdateHint(ctx context.Context, deploymentDir string, currentVersion string) {
	result, err := PerformSilentVersionCheck(ctx, deploymentDir, currentVersion)
	if err != nil {
		slog.Debug("launcher version update check failed", "error", err)
		return
	}
	if !result.Checked {
		return
	}
	if result.UpdateAvailable {
		slog.Info(
			"A new version of Exasol Personal is available",
			"current", currentVersion,
			"latest", result.LatestVersion,
			"info", "Run 'exasol version --latest' for more details",
		)
	}
}
