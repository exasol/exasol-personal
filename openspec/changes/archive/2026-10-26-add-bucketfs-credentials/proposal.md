## Why

Cloud deployments already create a default BucketFS bucket, but its credentials are missing from `secrets.json`. Users and agents must retrieve them from the database before uploading drivers or other files.

## What Changes

- Generate distinct read and write passwords for the default cloud bucket during infrastructure provisioning, and set them before installation completes.
- Store credentials by BucketFS service and bucket, including the standard read and write usernames, so the schema can represent multiple services and buckets.
- Keep generated credentials stable across installation retries and deployment lifecycle operations.
- Document how users access the default cloud bucket and how local deployments use filesystem access.

## Capabilities

### New Capabilities

- `bucketfs-credentials`: Credentials for the default cloud bucket, a schema supporting multiple services and buckets, and the corresponding user workflow.

### Modified Capabilities

## Impact

SPOT-33247. Affects the four built-in cloud infrastructure presets, the shared Ubuntu installation preset, Go secrets serialization, tests, user documentation, and the unreleased changelog. Local deployments continue to use filesystem-backed BucketFS without BucketFS credential entries.

Implementation is approved for newly initialized cloud deployments. Existing-deployment adoption and externally changed passwords remain follow-up decisions. Automatic management of additional buckets is outside scope.
