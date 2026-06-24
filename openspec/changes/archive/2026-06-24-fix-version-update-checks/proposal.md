## Why

The launcher can currently report an older official release as an available update when the current launcher is a newer release candidate. This creates false update guidance for prerelease users and undermines the update-check message.

## What Changes

- Compare launcher versions using semantic version precedence instead of string equality.
- Treat prerelease versions intentionally when deciding whether a reported official release is an update.
- Keep the update check best-effort and non-blocking.
- Document the launcher version-check policy so release candidates, final releases, and older versions are handled consistently.
- Add tests for older official releases versus release candidates, equal versions, newer versions, and prerelease-versus-final cases.

## Capabilities

### New Capabilities
- `launcher-version-check`: Covers launcher update-check behavior, including semantic version ordering, prerelease handling, and user-facing update messages.

### Modified Capabilities

## Impact

- Affects launcher update-check code in the Go CLI and deploy support packages.
- Affects user-facing `exasol version --latest` output and automatic update hints.
- Adds unit tests for version comparison behavior.
- Updates version-check documentation.
