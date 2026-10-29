## ADDED Requirements

### Requirement: Podman command failures state Podman's reason
The system SHALL include Podman's reported error line in the error returned for a failed Podman command, with or without `--verbose`, and SHALL record the failure and Podman's error output in the deployment log when a deployment fails.

#### Scenario: Container start fails with a Podman error
- **WHEN** Podman exits with a non-zero status while starting the Nano container and prints an error
- **THEN** the command's error shows the exit status followed by Podman's error line

#### Scenario: Deployment fails because of a Podman error
- **WHEN** a deployment fails because a Podman command failed
- **THEN** the deployment log contains the failure and Podman's error output

#### Scenario: Podman prints no error output
- **WHEN** a Podman command fails without printing any error output
- **THEN** the command's error shows the exit status
