## Context

The local infrastructure preset currently embeds `memoryMB: 2048`, and the local backend also carries a static default of 2048 MB. The local backend is only supported for macOS Apple Silicon in normal use, but unit tests run on other platforms as well.

## Goals / Non-Goals

**Goals:**
- Default macOS local deployment memory to about 50% of total host memory
- Fail fast when host memory is below the minimum required for local deployment
- Reject user-configured VM memory below the minimum supported size
- Keep the implementation simple and readable
- Preserve deterministic behavior for tests and unsupported platforms

**Non-Goals:**
- Change runner arguments or runtime protocol

## Decisions

- Compute the default in `internal/deploy/local_backend.go`, where local runtime sizing is already resolved.
  This keeps the behavior in one place and avoids changing the runner contract.
- Remove the hardcoded `memoryMB: 2048` from the embedded local infrastructure preset so the backend default can take effect when the user has not configured memory explicitly.
- Read host memory only on macOS, because the local backend is only supported there today.
  This keeps the implementation small and avoids carrying unreachable platform branches.
- Validate resolved local runtime sizing in one place in `local_backend.go`.
  The validation checks host memory first, then configured VM memory, so low-memory hosts fail with a host-focused message instead of a misleading VM-memory error.
- Keep a fixed fallback default if host memory detection fails.
  Host-minimum validation only runs when memory detection succeeds.

## Risks / Trade-offs

- host-memory lookup failure on macOS -> fall back to the existing fixed default to avoid breaking local configuration reads
- Dynamic defaults make one integration test non-static -> update the test to derive the expected memory from the launcher behavior instead of assuming 2048
- host minimum is only enforced when memory detection succeeds -> unsupported or unavailable detection paths may still use the fallback default until those platforms are fully supported
