## 1. Harness foundation

- [ ] 1.1 Choose harness location (`tests/ai_lab/`) and add a `task ai-lab:notebooks` target
- [ ] 1.2 Container runtime bring-up: start `exasol/ai-lab` configured like `installAiLab.sh` (Podman socket mount → `/var/run/docker.sock`, `registries.conf` with `docker.io`, `DockerRegistryImageChecker` patch); support both Docker and Podman hosts
- [ ] 1.3 Database bring-up in ITDE mode (`use_itde=true`); confirm the AI Lab can start and reach its docker-db
- [ ] 1.4 SCS seeding helper reusing the exact keys from `installAiLab.sh` (DB + BucketFS params), parameterized for ITDE vs external mode

## 2. Notebook discovery & execution

- [ ] 2.1 Discover `*.ipynb` in the image at runtime (no hardcoded list); record the discovered set in the report
- [ ] 2.2 Execute each notebook with `papermill` (per-notebook timeout); decide inside-container vs host-kernel execution (design open question 1)
- [ ] 2.3 Capture failing cell + traceback as structured output

## 3. Classification & reporting

- [ ] 3.1 Rule-based classifier: `PASS` / `FAIL (integration)` / `FAIL (upstream)` / `SKIP (needs X)`; default unmatched → `FAIL (integration)`
- [ ] 3.2 Seed the pattern table with the four known integration signatures (missing docker.sock, socket `PermissionError`, short-name registry, `Already exists`) and the known upstream signature (#1489 apt pin)
- [ ] 3.3 Write `report.md` + `report.json` (discovered notebooks × result × cause)
- [ ] 3.4 Exit codes: fail the gate on any `FAIL (integration)`; `FAIL (upstream)` warns by default with a known-issue allowlist (design open question 2)

## 4. CI integration (optional)

- [ ] 4.1 GitHub Actions workflow, manually-triggered + on `ai-lab`/`notebook-connector` version bump; not in the fast PR suite
- [ ] 4.2 Decide image pinning strategy: pinned-digest gate + scheduled `:latest` canary (design open question 3)

## 5. Validation

- [ ] 5.1 Run the harness locally in ITDE mode; confirm light notebooks (config, BucketFS, SQL) report `PASS`
- [ ] 5.2 Confirm `export_as_is` reports `PASS` (pulls the prebuilt image from Docker Hub, no local build); confirm a customize/rebuild notebook reports `FAIL (upstream)` linking #1489 (not `FAIL (integration)`)
- [ ] 5.3 Confirm credential-gated notebooks report `SKIP (needs X)` with the specific missing requirement
- [ ] 5.4 Regression check: temporarily revert one of the three `installAiLab.sh` fixes and confirm the harness flips the relevant notebook to `FAIL (integration)` with the right signature

## 6. Documentation

- [ ] 6.1 Document how to run the harness (local ITDE, external mode against a live deployment) and how to read the report
- [ ] 6.2 Note resource expectations and known limits (heavy ML notebooks, SLC build cost, credential-gated skips)
