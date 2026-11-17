## Why

Users with a running legacy deployment receive repeated migration warnings on every launcher invocation, even though ordinary commands remain usable. Daily, actionable guidance lets them schedule the move while keeping actual migration failures visible.

## What Changes

- Present deferred-migration guidance as a CTA once per local calendar day per deployment, including noninteractive text invocations.
- Suppress the CTA under `--json` without consuming that day's reminder.
- Explain the layout change, the reason for deferral, the applicable action and the temporary nature of legacy support.
- Coordinate startup and selected-directory presentation so selection does not add another unthrottled migration warning.
- Keep migration attempts independent from reminder frequency and retain operational diagnostics for failures and collisions.
- Scope persistence and compatibility code to this migration so it can be removed later through small integration points.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `launcher-path-migration`: deferred migration uses daily CTAs with persistent per-deployment state.
- `deployment-directory-resolution`: selected-directory notices report the chosen path, with migration guidance owned by the daily CTA.

## Impact

Tracked by SPOT-34071 under SPOT-33669. Delivered in its own PR after `align-macos-launcher-paths`. Affects startup migration, terminal CTA integration, legacy resolution/listing adapters, migration-specific paths, focused Go and launcher tests, documentation and the existing unreleased changelog entry. No intermediate-layout migration or general notification settings are introduced.
