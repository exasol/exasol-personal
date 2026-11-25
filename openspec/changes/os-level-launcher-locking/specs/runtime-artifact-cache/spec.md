## MODIFIED Requirements

### Requirement: Cache unlocking SHALL support stale-lock recovery

The launcher SHALL provide a way to clear a runtime artifact cache lock that an older launcher version left behind, and SHALL refuse to clear the cache while a running launcher of this version uses it.

#### Scenario: User clears stale cache lock

- **WHEN** an older launcher version left a runtime artifact cache lock
- **AND** a user requests cache unlocking
- **THEN** the launcher removes that lock
- **AND** the launcher reports on standard error that it removed the lock

#### Scenario: No stale cache lock exists

- **WHEN** no runtime artifact cache lock of an older launcher version exists
- **AND** a user requests cache unlocking
- **THEN** the launcher reports on standard error that there was no lock to remove

#### Scenario: Cache in use by this launcher version

- **WHEN** a running launcher of this version uses the runtime artifact cache
- **AND** a user requests cache unlocking
- **THEN** the launcher fails with an in-use error naming that launcher command

### Requirement: Runtime artifact cache operations SHALL coordinate concurrent access
The launcher SHALL coordinate operations that read or mutate runtime artifact cache state so concurrent launcher processes do not corrupt cached artifacts or cache metadata.

#### Scenario: Concurrent cache mutation is serialized
- **WHEN** multiple launcher processes attempt to mutate runtime artifact cache state concurrently
- **THEN** only one mutation proceeds at a time

#### Scenario: Cache operation reports lock contention
- **WHEN** a cache operation cannot proceed because another process holds the cache
- **THEN** the launcher reports that the cache is in use
- **AND** the error names each running launcher command holding the cache with its process ID and start time
