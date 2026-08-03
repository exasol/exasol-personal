## ADDED Requirements

### Requirement: External preset URIs support an optional subdirectory fragment

An external preset URI MAY include a trailing `#<subpath>` fragment that selects a
subdirectory within the resolved source. When present, the fragment SHALL identify the
preset directory (containing the required `infrastructure.yaml` or `installation.yaml`
manifest) within the cloned repository or extracted archive.

The fragment SHALL be supported on git URLs, remote archive URLs (`http://`, `https://`
ending in a supported archive extension), and local archive URIs (`file://` ending in a
supported archive extension). It SHALL be rejected on `file://` URIs that point to a
directory.

When a URI contains both an `@<ref>` suffix and a `#<subpath>` fragment, the ref SHALL come
before the fragment (e.g. `https://host/repo.git@v1#infra/aws`), and both SHALL be applied.

The fragment SHALL NOT contain traversal segments (`..`). Subpath resolution SHALL error out
when the referenced directory does not exist inside the resolved source, and SHALL surface
the existing "does not contain the expected … manifest" error when the referenced
subdirectory lacks the required manifest.

#### Scenario: Git URL with fragment resolves preset from subdirectory

- **WHEN** the user passes `https://host/repo.git#infra/aws`
- **THEN** the system SHALL clone the repository and resolve the preset from the
  `infra/aws` subdirectory

#### Scenario: Git URL combines ref and fragment

- **WHEN** the user passes `https://host/repo.git@v1#infra/aws`
- **THEN** the system SHALL check out ref `v1` and resolve the preset from the `infra/aws`
  subdirectory

#### Scenario: Local archive with fragment extracts and resolves subdirectory

- **WHEN** the user passes `file:///tmp/presets.tar.gz#installation/ubuntu`
- **THEN** the system SHALL extract the archive and resolve the preset from the
  `installation/ubuntu` subdirectory of the extracted contents

#### Scenario: Remote archive with fragment downloads and resolves subdirectory

- **WHEN** the user passes `https://host/presets.tar.gz#infra/aws`
- **THEN** the system SHALL download and extract the archive and resolve the preset from
  the `infra/aws` subdirectory

#### Scenario: Fragment on file:// directory URI is rejected

- **WHEN** the user passes `file:///tmp/my-presets#infra/aws`
- **THEN** the system SHALL return an error stating that the subdirectory fragment is not
  supported on `file://` directory URIs and instructing the user to pass the subdirectory
  path directly

#### Scenario: Fragment with traversal is rejected

- **WHEN** the user passes a preset URI whose fragment resolves outside the source root
  (e.g. contains `..`)
- **THEN** the system SHALL return an error and SHALL NOT resolve any preset content

#### Scenario: Fragment pointing to non-existent subdirectory returns a clear error

- **WHEN** the user passes a preset URI whose fragment names a subdirectory that does not
  exist in the resolved source
- **THEN** the system SHALL return an error that includes the requested subpath

#### Scenario: Fragment pointing to a subdirectory without the required manifest fails preset verification

- **WHEN** the user passes a preset URI whose fragment names a subdirectory that exists but
  lacks the required manifest for the preset type
- **THEN** the system SHALL return the "does not contain the expected … manifest" error,
  and the error SHALL include the resolved subpath
