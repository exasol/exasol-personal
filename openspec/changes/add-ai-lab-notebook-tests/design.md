# Design — AI Lab notebook run-through test harness

## Goal

Execute the AI Lab's bundled notebooks top-to-bottom in a clean, deployment-faithful environment, with **no AWS deployment and no human reading tracebacks**, and produce a classified per-notebook report that can gate `exasol/ai-lab` and `notebook-connector` upgrades.

## Why this is possible without AWS

The AI Lab is just the `exasol/ai-lab` container plus a seeded Secure Configuration Storage (SCS). Nothing about running the notebooks requires AWS specifically — it requires (a) the container configured the way a real host configures it, and (b) a reachable Exasol database + BucketFS. Both can be produced locally:

- The **container configuration** is fully captured by `installAiLab.sh` (Podman socket mount → `/var/run/docker.sock`, `~/.config/containers/registries.conf` with `docker.io`, the `DockerRegistryImageChecker` patch, the SCS seeding). The harness reuses these so it tests what we actually ship.
- The **database** comes from the AI Lab's own **ITDE** (Integrated Test Docker Environment, a.k.a. docker-db), selected via the `use_itde` SCS key. This is the same mechanism the upstream `ai-lab` project uses to test itself — self-contained, no cloud.

`papermill` runs a notebook non-interactively, parameterized, and surfaces the failing cell and traceback as structured output — which is precisely the "run-through + capture the error" loop, automated.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│ harness host (this WSL box, a dev machine, or a CI runner)     │
│  - Docker or Podman                                            │
│  - papermill (in a venv, or invoked inside the ai-lab image)   │
│                                                                │
│   1. start exasol/ai-lab container                             │
│      • mount podman.sock → /var/run/docker.sock (if Podman)    │
│      • write registries.conf (docker.io)                       │
│      • apply DockerRegistryImageChecker patch                  │
│   2. bring up ITDE / docker-db  ── use_itde=true ──┐           │
│   3. seed SCS (same keys as installAiLab.sh)       │           │
│   4. discover *.ipynb in the image                 ▼           │
│   5. for each notebook: papermill execute  ──► Exasol docker-db│
│   6. classify result, append to report                         │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
                  report.{md,json}  (per-notebook status + cause)
```

### Two database modes

| Mode | DB source | SCS keys | Use |
|---|---|---|---|
| **ITDE** (default for the harness) | docker-db started by the AI Lab | `use_itde=true`, `storage_backend=onprem` | CI / local; no cloud, no creds |
| **External** (what we did manually) | a real `exasol install` deployment | `use_itde=false`, `db_host_name=host.containers.internal`, etc. (as `installAiLab.sh` seeds) | optional end-to-end check against a live deployment |

The harness defaults to ITDE. External mode is a flag for an occasional full-fidelity run against a real deployment.

## Execution & classification

For each discovered notebook the harness runs `papermill <nb> <out> --kernel python3` (timeout per notebook) and maps the outcome:

| Result | Meaning | Example trigger |
|---|---|---|
| `PASS` | All cells executed without error | — |
| `FAIL (integration)` | Our host wiring is wrong | missing `/var/run/docker.sock`; short-name registry error; `DockerRegistryImageChecker` crash |
| `FAIL (upstream)` | Defect in `ai-lab` / `notebook-connector` / a flavor | flavor apt pin removed (#1489); a broken notebook cell |
| `SKIP (needs X)` | Requires an external secret/service we don't provide | Bedrock/SageMaker/OpenAI key, Hugging Face token, external S3 bucket |

Classification is rule-based on the captured traceback (pattern table maintained alongside the harness), defaulting to `FAIL (integration)` when unmatched so we never hide a regression behind a vague skip. The three integration signatures we already know are seeded into the pattern table from day one.

## Notebook coverage matrix

Coverage is **discovered at runtime**, not hardcoded — the matrix below is the expected shape and the unattended-runnability call per category. (Exact filenames vary by image version; the `script_languages_container/` set is what we observed on `exasol/ai-lab:latest` this session.)

| Category | Example notebooks | Unattended? | Needs |
|---|---|---|---|
| Getting started / configuration | main config / "configure the AI Lab" | ✅ (ITDE) | — |
| Data access / BucketFS | bucketfs access, IMPORT/EXPORT | ✅ (ITDE) | — |
| SLC — export / use | `script_languages_container/export_as_is`, `configure_slc_repository`, `test_slc`, `using_the_*` | ✅ (pulls prebuilt image from Docker Hub; verified 2026-06-25) | socket access (fixes in `installAiLab.sh`) |
| SLC — customize / rebuild | `customize`, `advanced` | ⛔ blocked | forces a local SLC build → **#1489** (stale flavor apt pins) |
| SQL / basic analytics | sql examples | ✅ (ITDE) | — |
| Machine learning (in-DB) | sklearn / in-database training | ✅ (ITDE) likely; verify resources | possibly RAM |
| Transformers / Hugging Face | `te_*` text-embedding/NLP | ⛔ SKIP by default | Hugging Face token; model downloads; RAM/GPU |
| Cloud ML services | SageMaker Autopilot, Bedrock | ⛔ SKIP by default | AWS creds + cloud resources |
| Cloud storage / external | external S3, cloud connections | ⛔ SKIP by default | cloud creds/buckets |

The harness emits the **actual** matrix each run (discovered notebooks × result), so this table is a planning aid, not the source of truth.

## Where it lives & how it's invoked

- Tooling under `tests/ai_lab/` (Python; aligns with the existing `tests/` Python suite). A `task ai-lab:notebooks` target wraps it.
- Default invocation: ITDE mode, run all discovered notebooks, write `report.md` + `report.json`, exit non-zero if any `FAIL (integration)` (upstream/skip do not fail the gate by default; configurable).
- CI: a **manually-triggered / version-bump-triggered** GitHub Actions workflow, not part of the fast PR suite — a full run pulls images, starts ITDE, and (for SLC) compiles containers, so it is minutes-to-tens-of-minutes and resource-heavy.

## Resource notes & honest limits

- This dev box: 14 CPU / 15 GB / ~933 GB free, **no container runtime installed** — Docker/Podman must be installed to run locally. 15 GB is fine for ITDE + light notebooks; heavy ML notebooks and parallel SLC builds may need a larger target.
- `export_as_is` **passes**: it pulls the prebuilt release image from Docker Hub (verified 2026-06-25 on a clean deploy with an unmodified flavor — 0 local builds), so #1489 does not affect it. Only notebooks that force a local rebuild (`customize` / `advanced`) will report `FAIL (upstream)` until #1489 is resolved; that is correct and informative, not a harness bug.
- Credential-gated notebooks are intentionally `SKIP` — wiring real cloud/LLM creds into CI is a separate decision with its own security tradeoffs.
- The harness validates **our integration and the notebooks' executability**, not model quality or numerical results.

## Alternatives considered

- **Keep testing against a real `exasol install` deployment.** Highest fidelity but slow, costs cloud money, needs SSH, and can't run in CI unattended. Kept as optional "external mode," not the default.
- **Unit-test the SCS seeding only.** Cheap, but would not have caught any of the three integration bugs — they only appear when a notebook actually drives `exaslct`/Docker through the container. Rejected as insufficient.
- **`nbconvert --execute` instead of `papermill`.** Works, but `papermill` gives cleaner per-cell error capture and parameterization (e.g. inject ITDE vs external). Minor preference.

## Open questions

1. Run `papermill` inside the AI Lab container (uses the image's exact `jupyterenv`) vs. from the host against the container's kernel — inside is more faithful; confirm during implementation.
2. Should `FAIL (upstream)` block CI or just warn? Default: warn (so known-upstream breakage like #1489 doesn't red-wall the gate), with an allowlist of known-upstream issues.
3. Pin the `exasol/ai-lab` image by digest per run for reproducibility, or always test `:latest` to catch upstream drift early? Likely both: a pinned gate + a scheduled `:latest` canary.
