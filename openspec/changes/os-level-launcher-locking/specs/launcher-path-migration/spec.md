## MODIFIED Requirements

### Requirement: Concurrent invocations SHALL each complete migration
When several CLI invocations start migration at the same time, each SHALL proceed to its command once every item is at its new location. An invocation that waits for another invocation's migration SHALL say so on standard error. After concurrent migrations, every migrated deployment SHALL be usable without an unlock command.

#### Scenario: Two invocations migrate at the same time
- **WHEN** two CLI invocations start migration of the same legacy items at the same time
- **THEN** both proceed without a migration warning
- **AND** every legacy item is at its new location
- **AND** no migrated deployment is reported as in use afterwards

#### Scenario: Invocation waits for another migration
- **WHEN** a CLI invocation starts while another invocation migrates legacy deployments
- **THEN** standard error states that it waits for the other launcher to finish migrating
- **AND** the invocation proceeds to its command after that migration
