## Purpose

Makes default cloud BucketFS credentials available by service and bucket so users and automation can access uploaded files after installation.

## ADDED Requirements

### Requirement: Bucket credentials are identified by service and bucket

The system SHALL represent BucketFS credentials in `secrets.json` as an optional `bucketfs` object keyed by service name, where each service has a `buckets` object keyed by bucket name and each bucket has `read` and `write` objects containing `username` and `password` strings.

#### Scenario: Default cloud bucket credentials

- **WHEN** a user inspects `secrets.json` after a successful new built-in cloud deployment
- **THEN** `bucketfs.bfsdefault.buckets.default.read` contains username `r` and its password, and `bucketfs.bfsdefault.buckets.default.write` contains username `w` and its password

#### Scenario: Multiple services and buckets

- **WHEN** the launcher reads and writes a secrets document containing credentials for multiple services and multiple buckets, including an identical bucket name in two services
- **THEN** each service and bucket retains its own read and write credentials

#### Scenario: Secrets document without BucketFS credentials

- **WHEN** the launcher reads a local or existing deployment's secrets document containing database credentials and an omitted `bucketfs` object
- **THEN** the database credentials are available through the existing secrets contract

### Requirement: Default cloud bucket accepts the stored credentials

For new deployments initialized from the updated built-in AWS, Azure, Exoscale, and STACKIT presets, the system SHALL complete installation with a deployment-specific pair of distinct read and write passwords of at least 32 letters and digits for `bfsdefault/default`, matching the credentials stored in `secrets.json`.

#### Scenario: Upload and download through HTTPS

- **WHEN** a user connects to the default BucketFS HTTPS endpoint from a permitted source after successful installation and uploads a file to `/default/<path>` using the stored write credential
- **THEN** the upload succeeds and the same file can be downloaded using that credential

#### Scenario: Read authentication for a private default bucket

- **WHEN** the default bucket requires read authentication and a user requests an existing file through its HTTPS endpoint with the stored read credential
- **THEN** the service returns the file

### Requirement: Credential setup is retryable

The system SHALL report successful installation only after the default bucket accepts the stored credentials, and SHALL reuse the same desired credentials when retrying an incomplete installation.

#### Scenario: Credential configuration fails

- **WHEN** installation cannot configure the default bucket's credentials
- **THEN** the command reports installation failure with the failing step identified and retains the desired credentials for a retry

#### Scenario: Installation is retried

- **WHEN** a user retries installation after a failed or interrupted credential-configuration step
- **THEN** a successful retry establishes the same credentials already stored for that deployment

### Requirement: Lifecycle operations preserve managed credentials

The system SHALL retain the default bucket's stored credentials across repeated infrastructure applies and ordinary lifecycle operations for deployments initialized with this feature.

#### Scenario: Stop and start

- **WHEN** a user stops and starts a cloud deployment whose bucket credentials have been established by installation
- **THEN** `secrets.json` retains the same default-bucket credentials, and those credentials continue to allow access after startup

#### Scenario: Infrastructure operation fails or is interrupted

- **WHEN** an infrastructure apply fails or is interrupted after installation has established the default bucket's credentials
- **THEN** the stored credential values remain available for the deployment's recovery workflow

### Requirement: BucketFS credential handling protects sensitive values

The system SHALL protect stored BucketFS credentials using owner-only secrets-file access and SHALL describe installation and lifecycle results in terminal output and persistent launcher logs using credential-free operation context at normal and verbose logging levels.

#### Scenario: Stored credential file access

- **WHEN** a built-in cloud preset writes `secrets.json` on a platform with POSIX file permissions
- **THEN** the file has mode `0600`

#### Scenario: Credential configuration output

- **WHEN** credential configuration succeeds or fails with verbose logging enabled
- **THEN** terminal output and persistent launcher logs identify the operation and its result using service and bucket names, with sensitive credential values confined to protected credential-bearing artifacts

### Requirement: Documentation explains the deployment-specific BucketFS workflow

The system SHALL document default cloud bucket access using the credentials in `secrets.json`, including the service, bucket, HTTPS endpoint, and write credential, and SHALL identify filesystem access as the local deployment workflow.

#### Scenario: Cloud user uploads a file

- **WHEN** a cloud deployment user follows the BucketFS instructions
- **THEN** the instructions show how to select the default bucket's credentials, locate the HTTPS endpoint, and upload a file, and link to Exasol's documentation for additional buckets and database access

#### Scenario: Local user accesses BucketFS

- **WHEN** a local deployment user follows the BucketFS instructions
- **THEN** the instructions direct the user to the deployment's filesystem-backed storage workflow
