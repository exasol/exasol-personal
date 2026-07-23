## 1. State and pure logic

- [x] 1.1 Add an `InstalledCustomSLC` state list, separate from the official one, keyed by alias and content digest.
- [x] 1.2 Build the activation URI and SCRIPT_LANGUAGES read-merge-write (set/remove one alias, preserve the rest) as pure, unit-tested logic.

## 2. Custom install / update / remove

- [x] 2.1 `slc custom install --file|--url --alias --language`: acquire the container (with digest), unpack it into the default BucketFS bucket, activate via `ALTER SYSTEM`, record state — no restart.
- [x] 2.2 Download for a URL happens on the host and is removed after unpack (no lingering copy on disk); a local file is used in place.
- [x] 2.3 `slc custom update` replaces the container behind a custom alias; identical content and language is a no-op.
- [x] 2.4 `slc custom remove` drops the SCRIPT_LANGUAGES entry and deletes the BucketFS files.
- [x] 2.5 Require a running database and a local deployment; reject clearly otherwise.

## 3. Alias mutual-exclusivity (both directions)

- [x] 3.1 Custom install: block reuse of an alias owned by an installed official SLC; confirm before overriding a built-in/official alias that is not installed; confirm before replacing an installed custom SLC.
- [x] 3.2 Official install/update: block when a custom SLC already owns one of the official SLC's aliases.

## 4. CLI surface

- [x] 4.1 Custom SLCs live under `slc custom install`/`update`/`remove`; the top-level `install`/`update`/`remove` stay official-only, and the top-level `remove` points a custom alias at `slc custom remove`.
- [x] 4.2 `list` (text and `--json`) covers custom SLCs alongside official ones, distinguished by type.

## 5. Delivery and container validation

- [x] 5a.1 Deliver the container in pure Go over SFTP (no remote shell or remote archiving tool); a cloud deployment reaches BucketFS over its HTTP service and is a separate backend implementation.
- [x] 5a.2 Validate the archive on the host before any deployment write: verify gzip integrity, reject entries that escape the container (path traversal, escaping symlinks/hardlinks), and require the standard SLC client to be present.
- [x] 5a.3 Confirm activation took effect (read back `SCRIPT_LANGUAGES`) before reporting success.

## 6. Tests and validation

- [x] 6.1 Unit tests for the activation URI, SCRIPT_LANGUAGES merge, alias/source validation, and the collision rules in both directions.
- [x] 6.2 Regression test: the start path never includes a custom SLC in the runner's mount flags.
- [x] 6.3 Unit tests for archive validation (integrity, escape rejection, client presence) and SFTP extraction.
- [x] 6.4 Formatting, focused tests, full-repo tests, and OpenSpec validation for this change.
