# Changelog

Notable user-facing changes to Exasol Personal are documented here.

## Unreleased

### Added

- Added named deployment selection with `--deployment` / `-d`, so users can manage multiple deployments under the default Exasol Personal deployment root without passing full directory paths.

  Example: `exasol status --deployment demo` targets the named `demo` deployment. `--deployment` and `--deployment-dir` cannot be used together.

- Added `exasol deployments list` to show default and named deployment directories, their status, and which deployment is currently active.

  The list helps users distinguish the default deployment from named deployments and spot missing or inactive deployment directories.

- Added the `exasol slc` command group (`install`, `list`, `update`, `remove`) to manage official script language containers (SLCs) in local deployments. Local deployments ship without any SLC, so UDFs in a given language only run once its container is installed.

  Example: `exasol slc install python3` installs the official Python 3 SLC so `PYTHON3` UDFs work; `exasol slc list` shows what is installed.

- Added `exasol slc custom install`/`update`/`remove` to manage user-supplied (custom) script language containers in local deployments, alongside the official `exasol slc` commands. Provide a container with `--file` or an HTTPS `--url`, plus `--alias` and `--language`; it is unpacked into BucketFS and activated for the alias without restarting the database.

  Example: `exasol slc custom install --file mypy.tar.gz --alias MYPY3 --language python` makes `MYPY3` UDFs available. `exasol slc list` shows custom containers next to official ones, and `exasol slc custom remove MYPY3` removes it.

- Added `exasol diag local` to report local deployment runtime and reachability state — VM status, guest IP, bound host ports, per-port reachability, database readiness, and platform support — as JSON. It is safe to run whether or not the deployment is currently running.

  Example: `exasol diag local` prints a JSON diagnostics snapshot for troubleshooting a local deployment.

- Added this changelog as the user-facing release history for Exasol Personal. Release candidates now have a single in-repo place where users can see which features, behavior changes, fixes, and breaking changes may affect them.

### Changed

- Improved README guidance to emphasize local deployment as the fastest way to try Exasol Personal.

- Improved local deployment error reporting: when the local database endpoint cannot be reached and every forwarded port is unreachable, connect/start/stop now report a clearer network-wide reachability error instead of a generic failure.

- Improved the installer script output with clearer getting-started guidance, PATH setup instructions, and platform-specific next steps.

- Clarified in the README where the launcher (`exasol`) is installed and that its directory must be on your `PATH`, via a dedicated "Install the Launcher" section.

### Fixed

- Fixed the personal installer script so it remains compatible with POSIX shell environments.

- Fixed `exasol connect` so `CREATE SCRIPT` / `CREATE FUNCTION` definitions terminate on a line containing only `/` (the EXAplus rule) instead of the first `;`. UDF bodies that contain semicolons (for example Java and R) now parse and run correctly through `-c`, `-f`, and interactive input.

  Example: `exasol connect -f create_udf.sql` no longer fails with `syntax error, unexpected '}'` when the script body contains semicolons.

- Fixed local deployments getting stuck in `operation_in_progress` when a Start or Stop operation failed; the interruption is now recorded so the deployment can be operated on again.

### Breaking Changes

- None.

## 2.1.0 - 2026-07-09

### Added

- Added support for external preset sources. Presets can now be resolved through the runtime artifact system from local files, HTTP sources, and Git repositories, including SSH-backed repositories and zip archives.

  Example: `exasol install https://github.com/org/exasol-preset.git@v1.0.0` lets users install from a preset repository instead of only using bundled presets.

- Added lifecycle JSON ready signals for automation around deployment start and stop commands.

  Example: `exasol start --json` gives scripts a machine-readable readiness signal instead of requiring them to scrape human-oriented terminal output.

- Added Azure support to the deployment cleanup tooling.

- Surfaced the local preset and quick-start path in top-level help so users can discover the fastest local deployment flow earlier.

  Example: `exasol --help` now points users toward the local deployment path instead of requiring them to find it in deeper command help.

### Changed

- Improved blocked-deployment recovery guidance across install, deploy, connect, start, and stop flows so users get more actionable next steps when local state prevents an operation.

- Changed the release workflow to publish from explicit, pre-pushed tags instead of creating tags inside the release workflow.

### Fixed

- Fixed `exasol config set` error handling so invalid deployment-directory state is reported with the standard not-a-deployment-directory guidance.

- Suppressed the database version banner for non-interactive `connect` usage, keeping scripted output cleaner.

- Made lifecycle commands idempotent for repeated start and stop operations.

- Fixed deployment compatibility checks to use the stable release baseline.

### Breaking Changes

- None.

## 2.0.0 - 2026-07-06

### Added

- Added local Exasol deployments for macOS through the local VM runtime. Users can run a local database with the local preset, and the launcher manages the local runtime, deployment share, memory defaults, port overrides, and recovery after improper VM shutdown.

  Example: `exasol install local` starts the local setup flow on supported macOS hosts.

- Added the default deployment directory flow and safe reuse of deployment directories. Users can run common commands without repeatedly passing a deployment directory, while the launcher protects existing state from incompatible preset changes.

  Example: after installing once, `exasol status` uses the default deployment directory when run outside another deployment directory.

- Added STACKIT as a supported cloud infrastructure provider, including account setup documentation and cleanup support.

- Added non-interactive SQL execution and richer output formats to `exasol connect`: inline commands with `-c`, file execution with `-f`, JSON output, CSV output, and standardized JSON results for multi-statement invocations and SQL errors.

  Example: `exasol connect --json -c "SELECT 1"` produces machine-readable output suitable for scripts and agent workflows.

- Added runtime artifact management so OpenTofu and runtime resources can be downloaded on demand and reused through a per-user cache. New cache commands and diagnostics help users inspect and clean cached artifacts.

  Example: `exasol cache list` shows downloaded runtime artifacts, and `exasol diag cache` checks cache health.

- Added improved `exasol info` guidance and JSON output so users can more easily find connection details and next steps.

  Example: `exasol info --json` returns connection details in a script-friendly format.

### Changed

- Moved OpenTofu resolution to runtime instead of requiring platform-specific OpenTofu binaries to be embedded in every launcher build.

- Updated cloud deployment defaults and refactored preset/backend handling so infrastructure resources are resolved more consistently.

- Updated README and help text around local deployment, system requirements, limitations, and Exasol Personal positioning.

- Changed local deployment key handling to use EdDSA keys and OpenSSH-compatible connection keys.

### Fixed

- Fixed large SQL result retrieval so result sets larger than 1,000 rows are fetched correctly.

- Fixed `exasol info` for stopped deployments that no longer have a reachable host.

- Fixed diagnostic logs leaking into default command output.

- Fixed typed JSON output preservation for `exasol connect`.

- Fixed launcher update checks to compare versions semantically.

- Fixed help output for preset-specific CLI arguments.

- Fixed Azure bootstrap blob upload from the deploying client and improved cleanup reliability for Exoscale and STACKIT resources.

### Breaking Changes

- None.
