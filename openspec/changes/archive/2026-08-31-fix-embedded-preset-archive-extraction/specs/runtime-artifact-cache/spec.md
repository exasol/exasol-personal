## ADDED Requirements

### Requirement: Runtime artifact cache SHALL extract directory entries within a tar.gz archive
The runtime artifact cache SHALL extract an explicit directory entry within a
tar.gz archive as a directory, in addition to the regular file and symlink
entries it already extracts.

#### Scenario: Directory entry is created
- **WHEN** a tar.gz archive being extracted contains an explicit directory
  entry
- **THEN** the cache SHALL create that directory in the extraction target
- **AND** extraction of the remaining entries in the archive SHALL continue

#### Scenario: Entries nested under an extracted directory entry are extracted
- **WHEN** a tar.gz archive contains a directory entry followed by regular
  file or symlink entries nested under that directory
- **THEN** the cache SHALL extract all of those nested entries into the
  created directory
