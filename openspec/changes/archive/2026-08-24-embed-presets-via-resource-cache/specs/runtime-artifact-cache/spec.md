## ADDED Requirements

### Requirement: A statically defined local-path artifact may omit a checksum
A statically defined `file://` or bare local-path artifact SHALL NOT be required to declare a
checksum, since its integrity comes from being part of the same versioned repository commit as
the launcher itself, not from a hand-authored value.

#### Scenario: Local-path artifact without a checksum is accepted
- **WHEN** a statically defined resource declares a `file://` or bare local-path artifact with
  no checksum
- **THEN** the specification is accepted

#### Scenario: Non-local artifact still requires a checksum
- **WHEN** a statically defined resource declares an artifact whose source is neither a git
  repository nor a local path, and specifies no checksum
- **THEN** the specification is rejected as invalid, exactly as before this capability existed
