# runtime-artifact-cache Specification

## MODIFIED Requirements

### Requirement: Cache unlocking SHALL support stale-lock recovery

The launcher SHALL provide a way to clear a stale runtime artifact cache lock.

#### Scenario: User clears stale cache lock

- **WHEN** a user requests cache unlocking
- **THEN** the launcher clears the cache lock if one exists
- **AND** the launcher reports the unlock result as an operational notice on standard error
