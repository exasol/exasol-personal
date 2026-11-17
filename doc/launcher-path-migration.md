# Retiring the legacy launcher layout

The legacy filesystem layout is supported for a transition period. Its compatibility code is scoped to the path migration so that retiring it does not require redesigning permanent path resolution.

To retire it, remove the startup migration hook from `cmd/exasol/root.go` and the migration-specific command files: `startup_migration.go` and `launcher_path_migration_*.go`. Replace managed selection and listing adapters with operations on the current managed root, and remove the explicit legacy-path redirection call. Remove the source resolvers in `internal/launcherpaths/launcher_path_migration.go` and their tests. Keep permanent destination paths in `launcherpaths.go`.

Remove the migration-specific tests, specifications and published transition guidance in the same change. Preserve reusable helpers only when surviving callers need them. Reminder markers are migration-local and are retired when a deployment moves; they are not general launcher settings.
