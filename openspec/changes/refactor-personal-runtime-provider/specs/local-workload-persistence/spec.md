## ADDED Requirements

### Requirement: Runtime destruction preserves Personal-owned data

Adapter stop, destroy, workload replacement, provider replacement, and image reload
SHALL preserve data below the deployment's Personal-owned local data directory.

#### Scenario: Provider state is recreated

- **WHEN** the macOS provider is destroyed and initialized again
- **THEN** the Personal-owned host data directory remains unchanged

#### Scenario: The deployment is explicitly destroyed

- **WHEN** the user invokes Personal's deployment destroy operation
- **THEN** Personal removes the exact deployment-owned data directory after runtime cleanup
