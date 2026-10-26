## Context

The [BucketFS overview](https://docs.exasol.com/db/latest/administration/on-premise/bucketfs/bucketfs_overview.htm) confirms that installation creates service `bfsdefault` and bucket `default`. Services group buckets and define their network ports. Each bucket has separate read and write passwords; the standard HTTP usernames are `r` and `w`, rather than separately created per-bucket user accounts. See [file access examples](https://docs.exasol.com/db/7.1/database_concepts/bucketfs/file_access.htm).

Users upload drivers, libraries, models, and other files through HTTPS or Exasol Admin. The default service uses HTTPS port 2581. For a private bucket, database users receive access through a SQL connection targeting `bucketfs:<service>/<bucket>` and a grant; UDFs then use `/buckets/<service>/<bucket>/...`. Link the user guide to [database access](https://docs.exasol.com/db/latest/administration/on-premise/bucketfs/database_access.htm) for the complete workflow.

Today the four built-in cloud presets generate database and Admin UI passwords, include them in their installation payloads, and write root `secrets.json`. Bucket passwords are generated inside the installed database. COS supports setting both through `bucket_modify`, as documented in [Change bucket password](https://docs.exasol.com/db/latest/administration/on-premise/bucketfs/change_bucket_password.htm). The shared Ubuntu post-install phase already runs ConfD through C4 to configure TLS.

Local deployments run Nano with filesystem-backed BucketFS. They have no HTTP read/write password pair. The user confirmed that local deployments remain filesystem-based and receive no BucketFS credential entries.

The user approved implementation of the proposed new-deployment behavior and nested schema. Existing-deployment adoption and external password rotation remain open follow-up decisions; the implementation handoff must not invent a migration or rotation policy.

## Goals / Non-Goals

**Goals:** Make the default bucket usable immediately after a new cloud installation using credentials in `secrets.json`; represent credentials by service and bucket; use stable infrastructure-generated values and the shared installer across cloud providers.

**Non-Goals:** Local HTTP service or local BucketFS credentials, automatic creation or discovery of additional buckets, new BucketFS management commands, and changes to bucket access policy.

## Decisions

### Represent services and buckets explicitly

Proposed `secrets.json` fragment:

```json
{
  "bucketfs": {
    "bfsdefault": {
      "buckets": {
        "default": {
          "read": {"username": "r", "password": "<generated-read-password>"},
          "write": {"username": "w", "password": "<generated-write-password>"}
        }
      }
    }
  }
}
```

Both service and bucket collections are maps keyed by name. A bucket name is unique within a service, so the same bucket name in another service has its own credentials. The explicit username/password objects make each credential directly usable by clients. The names `r` and `w` describe the standard access identities; the map does not introduce arbitrary BucketFS account management.

Existing database and Admin UI fields keep their current shape. The BucketFS field is optional for local and older secrets documents. Go serialization preserves every represented service and bucket. This schema supports multiple entries; this change automatically provisions only `bfsdefault/default`. Editing this file is not a mechanism for creating buckets, and arbitrary manual additions are not automatically managed by OpenTofu.

Avoid top-level `bucketfsReadPassword` and `bucketfsWritePassword`: those cannot identify the service or bucket. Keep connection endpoints outside the credential model; clients use the deployment's node address and the service's HTTPS port.

### Generate credentials in infrastructure provisioning

Add two distinct 32-character alphanumeric `random_password` resources to each built-in cloud preset. Persist them in OpenTofu state, include them in the nested secrets output, and transport them through the existing installation payload. Generate once per deployment rather than on each script invocation. The encoded transport representation is not encryption and requires the same protections as the other payload secrets.

OpenTofu remains the writer of the cloud secrets artifact. Consequently subsequent applies reproduce the same complete credential document, including after start or stop. This avoids a second writer appending fields that OpenTofu later removes. Go's shared secrets model must understand the new subtree whenever it reads and serializes the document.

Retrieving generated database credentials through ConfD would support existing deployments, but would require launcher-side export, additional file-ownership handling, and preservation across applies. For new installations, generating the desired credentials first is simpler.

### Configure the existing default bucket in the shared installer

Extend the shared Ubuntu post-install phase with one script executed on access node `n11`, after ConfD is ready and before installation is reported complete. Use the existing C4 connection pattern to run `bucket_modify` for `bfsdefault/default` with the two generated values. No dedicated C4 BucketFS password installation parameter was found in the inspected source; the ConfD modification is the verified integration option.

The operation requires the bucket's public flag. Read and preserve that flag rather than copying the documentation's `public: False` example, which would change access policy. The read is for the flag, not to obtain generated passwords; capture its response privately because it can include credentials.

All four built-in cloud security configurations already allow inbound TCP 2581 from `allowed_cidr`. Their external rules do not open HTTP port 2580. No new firewall rule is needed for the default service; actual HTTPS reachability still needs live verification.

Use the same input values on every retry. Record completion only after the operation succeeds. Keep command responses and secret-bearing data out of ordinary task output, verbose streams, and replay buffers. Confirm the exact command and encoding behavior against the database version shipped by Personal during implementation validation.

### Document the complete consumer workflow

Add a concise section to the existing user documentation showing how to select the `bfsdefault/default` credential entry, locate the node endpoint, and upload a file to `https://<node>:2581/default/<path>` with the write credential. Explain that the write password permits reading as well. Link to Exasol's instructions for creating further buckets, managing access, and granting SQL access rather than duplicating them. Link local users to the existing filesystem workflow.

## Risks / Trade-offs

- Password update fails after infrastructure writes secrets: report installation failure and allow a retry with the same values; do not report completed deployment with unusable credentials.
- Installer output or ConfD responses expose credentials: keep responses private, use protected temporary material where required, and test success and failure with verbose logging.
- Retry after interruption reapplies the values: safe before completion because the desired values are stable; normal lifecycle operations should not become implicit password-rotation operations.
- Infrastructure recreates or loses password resources: treat replacement as a credential change requiring coordinated database configuration, not a harmless output rewrite.
- Four preset copies drift: validate the common output and payload contract for all four providers, with live evidence on supported test providers. STACKIT currently has no working live coverage in the test suite.
- New binary with an old extracted installation preset: embedded preset changes do not upgrade existing deployment directories. Adoption requires the explicit migration decision below.

## Migration Plan

Start with new deployments initialized from the updated built-in presets. Retain reading compatibility for secrets documents without `bucketfs`. Review the existing deployment-version compatibility contract before implementation and avoid treating a newer binary as an automatic preset migration.

Do not rotate existing deployed credentials as part of an incidental upgrade. Existing-deployment support needs an explicit choice between credential retrieval, an approved rotation/migration workflow, and documenting the feature as available on newly initialized deployments. The user approved the new-deployment path; existing-deployment adoption remains a separate follow-up decision.

## Validation Approach

Use Go tests for optional fields and round trips with multiple services, duplicate bucket names across services, and distinct credentials. Add provider contract checks for stable generated outputs and installation payloads. Exercise the installer with a controlled ConfD/C4 boundary for success, failure, retry, public-flag preservation, and secret handling.

Use live cloud evidence to upload, download, and delete a disposable file with the stored credentials, then repeat after stop/start. Use a private test bucket or an isolated test to establish read authentication; a public bucket's successful GET alone does not prove the read password is correct. Product-facing tests should identify `openspec("bucketfs-credentials")`. Local coverage verifies the existing filesystem workflow and the optional secrets subtree. Run repository formatting, linting, documentation, and relevant test checks once implementation exists.

## Open Questions

1. Should this story support existing deployments, and through credential retrieval or an explicit password rotation? New deployments have a complete proposed path; old extracted presets need a separate adoption path.
2. After a user changes a managed bucket's passwords through Exasol Admin or ConfD, who owns reconciliation? Proposed default: installation establishes initial credentials, ordinary lifecycle operations preserve them, and external rotation requires an explicit update workflow. We should not promise that `secrets.json` continuously reflects out-of-band changes without that workflow.
3. Reconcile Jira wording with the confirmed local filesystem decision and the selected existing-deployment policy through the delivery workflow's approval boundary.
