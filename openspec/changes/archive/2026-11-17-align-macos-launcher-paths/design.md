## Context

SPOT-32510 defines the final platform layout. PR #520 is merged into `main` and supplies retained-legacy lookup and listing. `internal/launcherpaths/launcherpaths.go` currently uses `os.UserConfigDir` for macOS durable data and resource configuration. Launcher fixtures repeat that choice in `tests/tests/launcher/helpers.py`; the published path table and unreleased changelog do too.

## Goals / Non-Goals

**Goals:** macOS durable data and resource configuration use `~/.exasol/launcher`, with the existing purpose-specific leaves. Keep native cache placement. Align documentation, specifications and tests with Jira.

**Non-Goals:** migration from intermediate unreleased layouts, changing other platform policies, daily CTAs, or a general path/configuration redesign.

## Decisions

1. Resolve macOS durable data from the home directory, alongside the existing Linux policy. Resolve macOS resource configuration from that durable root; preserve Linux XDG configuration and Windows roaming configuration behavior. Keep `os.UserCacheDir` for caches. A configurable path framework would add scope without helping this decision.
2. Keep the released legacy sources as the only migration sources. The existing startup steps automatically target the corrected destinations through `launcherpaths`; adding Application Support as another source would maintain an unreleased layout unnecessarily.
3. Leave existing automatic migration, retained-deployment fallback and directory-listing semantics in place. The second PR owns reminder behavior and migration-specific isolation, so this PR stays independently reviewable.
4. Add exact platform-path assertions to the Go tests, including applicable home/config/cache environment overrides. Update Python platform helpers and exercise actual CLI path reporting and migration with temporary homes. Suffix-only tests do not establish the macOS decision.

## Risks / Trade-offs

- Stale fixtures can mask incorrect paths: assert complete paths and update the shared launcher helper together.
- The available development host is Linux: exercise platform policy through a small testable resolver or platform-specific tests, cross-compile affected packages, and rely on the normal macOS CI lane for native execution. Never claim native macOS evidence from a Linux run.
- Developers using unreleased Application Support data must handle those files themselves. This is an explicit scope decision, not an automatic migration source.

## Migration Plan

Implement on `nh.macos-launcher-paths_SPOT-32510` from refreshed `origin/main`. Migrate released legacy deployments, history and resource settings through the existing startup path. Update the existing `Unreleased` entry. After validation and review, archive this change before committing its OpenSpec artifacts in their own commit and opening the first PR. The daily CTA PR stacks on this branch until it merges.

## Open Questions

None.
