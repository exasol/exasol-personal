## 1. Live bounded status listing

- [x] 1.1 Make overlapping database-probe error suppression restore the original driver logger after the last probe.
- [x] 1.2 Resolve each listed deployment through canonical status under one concurrent five-second bound.

## 2. Verification and documentation

- [x] 2.1 Test concurrent logger restoration, canonical status values, concurrent resolution, timeout behavior, ordering, and tolerant fallbacks.
- [x] 2.2 Update user documentation and the changelog, then verify OpenSpec, formatting, linting, tests, and build.
