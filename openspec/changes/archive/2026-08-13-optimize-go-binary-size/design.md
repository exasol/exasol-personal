## Context

The launcher is distributed as raw Go binaries for supported platforms. The current release build already favors static binaries, but it does not apply all low-risk Go size flags consistently across local Task builds and GoReleaser.

## Goals / Non-Goals

**Goals:**

- Reduce raw executable size for Linux, macOS, and Windows launcher builds.
- Keep local Task builds and release builds aligned.
- Document the accepted and rejected size optimization techniques.

**Non-Goals:**

- No end-user feature removal.
- No executable packing, alternative compiler toolchains, or dependency replacement solely for size.
- No changes to release archive formats.

## Decisions

- Use Go-native build flags by default: `CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"`, and `-gcflags=all=-l`.
- Apply the same optimization policy to Task and GoReleaser so CI, local builds, and releases remain comparable.
- Document higher-risk techniques as rejected so future changes do not reintroduce signing, debugging, or maintenance risks without a new decision.

## Risks / Trade-offs

- Disabling inlining can affect performance and stack shape. Mitigation: keep the change limited to the CLI launcher and validate with unit tests and build checks.
- Stripping debug data reduces post-build debugger visibility. Mitigation: preserve normal panic stack trace usability and document the tradeoff.
- Future cgo-dependent features may conflict with `CGO_ENABLED=0`. Mitigation: revisit this decision only if a concrete feature requires cgo.
