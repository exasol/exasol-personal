# Orphaned Deployment Cleanup

This document describes the high-level architecture and rationale of the orphaned deployment cleanup tool of the Exasol Personal 

## Purpose
Losing the original deployment directory (and its IaC state) can leave cloud resources running and accruing cost. The cleanup feature provides a safe, guided way to discover and remove those orphaned resources using only their shared deployment tag.

## Scope
- AWS deployments created by the launcher.
- Single region operation (user-specified or inferred from `AWS_REGION` / `AWS_DEFAULT_REGION`).
- Uses a consistent tag key: `Deployment` whose value is the deployment identifier.
- Focused strictly on discovery, inspection, and ordered deletion—not recovery or migration.

## User Workflow
1. `exasol-cleanup discover` — List all deployment IDs found via the tag (with region, created time, state summary).
2. `exasol-cleanup show <deployment-id>` — Inspect all resources for a specific deployment.
3. `exasol-cleanup run <deployment-id>` — Produce a dry-run plan. Use `--execute` to perform deletions.

## Design Decisions
| Decision | Rationale |
|----------|-----------|
| Tag-based discovery | Works without lost Terraform/OpenTofu state; relies only on tag metadata. |
| Single-region default | Matches existing deployment pattern; reduces accidental multi-region deletion. |
| Static ordered phases | Predictable and simple; lowers risk of dependency-related AWS errors. |
| Dry-run as default | Prevents accidental data loss; encourages review before destructive action. |
| Continue-on-error | Maximizes resource reclamation even with partial failures. |
| AWS SDK v2 | Modern modular SDK; context-aware, pluggable retries. |
| Explicit execution flag | Guarantees user intent before performing deletions. |

## Cleanup Phases
1. Instances (terminate)
2. Volumes (delete)
3. Network attachments (detach IGWs, disassociate non‑main route tables, delete non‑default security groups)
4. Subnets (delete)
5. VPCs (delete)
6. Parameters (delete configuration remnants)

Default or protected constructs (main route tables, default security groups) are skipped and reported.

## Safety Model
- Dry-run enumerates planned actions per phase.
- Deletions only occur with `--execute`.
- Each resource deletion is independent; failures are logged and do not abort the entire run.
- Protected resources are explicitly marked as skipped.

### Discovery safety filters
- Only resources with either `Project = exasol-personal` or `Deployment` values matching this regex pattern `exasol-[a-f0-9]{8}` are considered.
- This dual filter reduces the chance of matching unrelated workloads while accommodating legacy or partial tag sets.
### Owner filtering
- `--owner` flag filters deployments by the `Owner` tag using a simple `*` wildcard (substring match).
- Defaults to caller identity ARN; use `--owner=*` to list all owners.

## Observability
- Structured logging includes region, account ID, deployment ID, and execution mode.
- Per-action outcome (success, failed, skipped) is recorded.
- Optional JSON output enables scripting/automation.

## Extensibility
- New resource types: add classification + handler + phase entry.
- Future multi-region support can parallelize the discovery step without changing commands.

## Non-Goals
- Reconstructing lost configuration/state files.
- Migrating or backing up data.
- Cross-account or organization-wide cost analysis.

## Summary
The cleanup feature adds a recoverability layer focused on cost control and operational hygiene. It provides deterministic, safe, and transparent deletion of orphaned deployments with minimal user input.
