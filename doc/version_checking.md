# Version checking

This document describes the **update-checking** feature: how the Exasol Personal launcher detects that a newer release is available.

## Goals and behavior

The version check is intentionally **best-effort**:

- It should **never block** a command from running.
- It should be **rate limited** so normal command usage does not generate excessive requests.
- It should provide an **actionable hint** (and point users to `exasol version --latest` for details).

## When the launcher checks

- Most `exasol` commands (everything except `exasol version`) attempt a version check during command startup.
- The check is performed only when the launcher has access to a **deployment directory**, because the deployment’s persistent launcher state is used for rate limiting. For background on launcher state, see [Deployment state & locking](launcher_state.md).
- `init` and `install` are special: they are frequently run *before* a deployment directory exists. They therefore perform the check directly as part of their workflow.

### Opting out

Users can disable automatic launcher update checks during initialization (for example, in CI or offline environments) using `--no-launcher-version-check`.

## Rate limiting and failure semantics

The launcher performs at most one check per **deployment directory** within a 24-hour window.

To avoid repeated requests in error cases, an attempt is treated as a “check” even when the API call fails or times out; another attempt will not be made for the same deployment directory for 24 hours.

## API endpoint

By default, the launcher uses:

- `https://metrics.exasol.com/v1/version-check`

The request includes basic platform and version information (category, operating system, CPU architecture, and current version) so the service can return the latest compatible release artifact.

The endpoint can be overridden via the environment variable `EXASOL_VERSION_CHECK_URL` (primarily used for tests).

### Example request and response

The response contains a `latestVersion` object with metadata for the newest available release on that platform.

Example:

```shell
curl -s "https://metrics.exasol.com/v1/version-check?category=Exasol%20Personal&operatingSystem=Linux&architecture=x86_64&version=1.0.0" | jq .
```

```json
{
	"latestVersion": {
		"version": "1.2.0",
		"filename": "exasol",
		"url": "https://x-up.s3.eu-west-1.amazonaws.com/releases/exasol-personal/linux/x86_64/1.2.0/exasol",
		"size": 728049984,
		"sha256": "447bc37c024c0325d8d74fa57e05f6820875aca5b0d61d2b8f0d980b1f661931",
		"operatingSystem": "Linux",
		"architecture": "x86_64"
	}
}
```
