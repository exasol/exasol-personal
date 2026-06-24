## Context

Launcher update checks currently compare the reported latest version to the current launcher version by string equality. Any different non-empty version is treated as an update, so a prerelease launcher such as `2.0.0-rc1` can be told that an older official release such as `1.4.1` is available.

The repository already uses `github.com/blang/semver/v4`, so the fix can use the existing semantic-version parser without introducing a new dependency.

## Goals / Non-Goals

**Goals:**
- Use semantic version precedence for launcher update availability.
- Preserve the best-effort, non-blocking behavior of automatic update hints.
- Make `exasol version --latest` accurate when the reported latest version is newer, equal, or older than the current launcher version.
- Document the prerelease comparison policy.

**Non-Goals:**
- Change the version-check service API.
- Change release publishing.
- Automatically install launcher updates.
- Change deployment directory compatibility checks.

## Decisions

- Treat an update as available only when `latestVersion.version` has greater semantic precedence than the current launcher version.
  - Rationale: this directly models "newer" and prevents false positives from older official releases.
  - Alternative considered: compare only major/minor/patch and ignore prerelease metadata. That would make `2.0.0` and `2.0.0-rc1` appear equal, hiding a real final-release update from prerelease users.

- Use SemVer prerelease precedence as implemented by the existing semver library.
  - Rationale: release candidates are valid SemVer prereleases and should sort before their final release but after older release lines.
  - Alternative considered: special-case `rc` strings. That is narrower and less reliable than the established SemVer rules.

- Keep invalid or missing version data as a check failure rather than an available update.
  - Rationale: silent checks must not show misleading update hints. Explicit `version --latest` calls can surface the error to the user.

- Route version-check output according to whether it is primary command output or metadata.
  - Rationale: stdout must remain parseable for commands that produce data, especially JSON. Implicit update hints are metadata and belong on stderr, while explicit `exasol version` output is the requested command output and belongs on stdout.
  - Alternative considered: send all human-readable update information to stderr. That would make `exasol version --latest` surprising because its directly requested report would not be emitted on the primary output stream.

- Use the terminal-message queue for explicit version command output instead of direct stdout writes.
  - Rationale: this keeps output routing centralized and consistent with the existing terminal notice/output infrastructure.
  - Alternative considered: write explicit version output directly to stdout. That works functionally, but bypasses the project's established command-output flush path.

- Render human-readable latest-version output with an embedded text template.
  - Rationale: the multi-line report is easier to read and maintain as a template than as manual string concatenation.
  - Alternative considered: build the report with a string builder. That avoids direct stream writes, but makes the output structure harder to review.

## Risks / Trade-offs

- Invalid service responses may suppress automatic hints until the next scheduled check.
  - Mitigation: the check remains best-effort, logs debug details, and explicit `exasol version --latest` can report the parsing problem.
- Users running prerelease launchers may see that the latest official release is older than their current version.
  - Mitigation: the explicit command should say the reported latest official version is not newer instead of calling it an update.
- Mixed stdout/stderr behavior can be surprising if the policy is not documented.
  - Mitigation: document that explicit version commands write primary output to stdout, while implicit hints use stderr.
