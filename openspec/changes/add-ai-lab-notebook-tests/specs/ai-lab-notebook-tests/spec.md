## ADDED Requirements

### Requirement: Headless notebook execution without a cloud deployment
The system SHALL execute the AI Lab's bundled notebooks non-interactively against a self-contained Exasol database, without requiring a cloud deployment or interactive user input.

#### Scenario: Run notebooks against the integrated test database
- **WHEN** the harness runs in its default mode
- **THEN** it starts the `exasol/ai-lab` container, brings up the integrated test database (ITDE), seeds the SCS, and executes the notebooks without any cloud provisioning or SSH access

#### Scenario: Optional run against a live deployment
- **WHEN** the harness is invoked in external mode against an existing `exasol install` deployment
- **THEN** it seeds the SCS with that deployment's database and BucketFS parameters (as `installAiLab.sh` does) and executes the notebooks against the live database

### Requirement: Deployment-faithful container configuration
The harness SHALL configure the AI Lab container with the same settings that `installAiLab.sh` applies, so that it tests the behavior of a real deployment.

#### Scenario: Podman host configuration is reproduced
- **WHEN** the harness starts the AI Lab container on a Podman host
- **THEN** it mounts the Podman API socket at `/var/run/docker.sock`, configures `docker.io` as an unqualified-search registry, and applies the `DockerRegistryImageChecker` compatibility patch

### Requirement: Notebooks are discovered at runtime
The system SHALL determine the set of notebooks to execute by discovering them in the image at runtime, rather than from a hardcoded list.

#### Scenario: New notebook is picked up automatically
- **WHEN** the AI Lab image contains a notebook not previously seen by the harness
- **THEN** the harness discovers and executes it, and records it in the report

### Requirement: Per-notebook results are classified
The system SHALL report a result for each executed notebook classified as pass, integration failure, upstream failure, or skipped-needs-credential, including the failing cell and cause for failures.

#### Scenario: Integration failure is attributed to our wiring
- **WHEN** a notebook fails because of host/container configuration the launcher is responsible for
- **THEN** the harness reports `FAIL (integration)` with the captured traceback

#### Scenario: Upstream failure is attributed to a dependency
- **WHEN** a notebook fails because of a defect in `ai-lab`, `notebook-connector`, or a flavor (e.g. stale flavor package pins)
- **THEN** the harness reports `FAIL (upstream)` and references the known upstream issue when matched

#### Scenario: Credential-gated notebook is skipped, not failed
- **WHEN** a notebook requires an external credential or service the harness does not provide
- **THEN** the harness reports `SKIP (needs <requirement>)` rather than a failure

#### Scenario: Unrecognized failure is not hidden
- **WHEN** a notebook fails with a traceback that matches no known pattern
- **THEN** the harness defaults to `FAIL (integration)` so the regression is surfaced rather than silently skipped

### Requirement: Usable as a regression gate
The system SHALL produce a machine-readable report and an exit status suitable for gating dependency upgrades in CI.

#### Scenario: Gate fails on integration regression
- **WHEN** any notebook reports `FAIL (integration)`
- **THEN** the harness exits non-zero

#### Scenario: Known-upstream breakage does not red-wall the gate
- **WHEN** the only failures are `FAIL (upstream)` entries on the known-issue allowlist
- **THEN** the harness reports them as warnings and does not fail the gate by default
