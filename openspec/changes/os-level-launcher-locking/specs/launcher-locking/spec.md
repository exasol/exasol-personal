## ADDED Requirements

### Requirement: Launcher commands SHALL coordinate access to a deployment
A launcher command that changes a deployment SHALL run only while no other launcher command uses that deployment. Commands that only read a deployment SHALL run alongside each other. A command that cannot get access within its waiting time SHALL fail with an in-use error, except that `exasol status` and `exasol deployments list` SHALL report the deployment as `operation_in_progress`.

#### Scenario: Concurrent changing commands exclude each other
- **WHEN** a launcher command that changes a deployment runs
- **AND** another launcher command that changes the same deployment starts
- **THEN** the second command fails with an in-use error without changing the deployment

#### Scenario: Reading commands run alongside each other
- **WHEN** an `exasol shell` session uses a deployment
- **AND** `exasol info` runs on the same deployment
- **THEN** `exasol info` reports the deployment information

#### Scenario: Status during a changing command
- **WHEN** a launcher command that changes a deployment runs
- **AND** `exasol status` runs on the same deployment
- **THEN** `exasol status` reports `operation_in_progress`

### Requirement: Deployment access SHALL end with the launcher process
A deployment or cache that a launcher process used SHALL be available to the next launcher command as soon as that process has ended, including after a crash or forced termination, without an unlock command.

#### Scenario: Forcefully terminated launcher
- **WHEN** a launcher command that changes a deployment is terminated with `SIGKILL`
- **THEN** the next launcher command on that deployment is not reported as in use

#### Scenario: Interrupted launcher
- **WHEN** a user interrupts a launcher command with Ctrl-C
- **THEN** the deployment stays in use until the launcher and the processes it waits for have exited
- **AND** the next launcher command after that is not reported as in use

### Requirement: In-use errors SHALL identify the launcher commands holding access
An in-use error for a deployment or the runtime artifact cache SHALL list each running launcher command that holds it, with its command, process ID, and start time, and SHALL tell the user to wait for those commands to finish or to stop them.

#### Scenario: Deployment used by a shell session
- **WHEN** `exasol shell` with process ID 4711 uses a deployment
- **AND** `exasol deploy` on that deployment fails with an in-use error
- **THEN** the error names `exasol shell`, process ID 4711, and the time the shell started
- **AND** the error tells the user to wait for it to finish or to stop it

### Requirement: Locking SHALL be optional per invocation
The `--no-lock` option and the `EXASOL_LAUNCHER_NO_LOCK` environment variable set to `1` or `true` SHALL run a command without waiting for, honoring, or taking deployment, runtime artifact cache, or startup migration locks. Such a command SHALL print a warning on standard error that names the option or variable and states that concurrent launcher commands can then corrupt deployment state.

#### Scenario: Command runs while another command holds the deployment
- **WHEN** `exasol deploy` uses a deployment
- **AND** `exasol status --no-lock` runs on the same deployment
- **THEN** `exasol status` reports the deployment status without an in-use error
- **AND** standard error contains a warning naming `--no-lock`

#### Scenario: Locking disabled through the environment
- **WHEN** `EXASOL_LAUNCHER_NO_LOCK=1` is set
- **AND** a user runs `exasol info --json`
- **THEN** standard output contains only the JSON result
- **AND** standard error contains a warning naming `EXASOL_LAUNCHER_NO_LOCK`

### Requirement: Filesystems without lock support SHALL produce actionable errors
When the filesystem holding launcher locks refuses file locks, a command that needs a lock SHALL fail with an error that names the affected path, explains that the filesystem does not support file locks, and describes `--no-lock` as a workaround for when no other launcher command uses the deployment.

#### Scenario: Lock service unavailable
- **WHEN** the filesystem holding launcher locks refuses file locks
- **AND** a user runs `exasol status`
- **THEN** the command fails with an error naming the path and the missing lock support
- **AND** the error describes `--no-lock` and its condition

### Requirement: Selected directories SHALL contain only their own content
Launcher commands SHALL keep coordination files out of deployment directories and any other directory selected with `--deployment-dir`.

#### Scenario: Status on a directory that is not a deployment
- **WHEN** a user runs `exasol status --deployment-dir` on an existing empty directory
- **THEN** the directory is still empty afterwards

#### Scenario: Removed deployment leaves no directory
- **WHEN** a user removes a deployment with `exasol remove`
- **THEN** the deployment directory no longer exists

### Requirement: Locks of older launcher versions SHALL be honored and clearable
A deployment or cache lock that an older launcher version created SHALL make commands of this version report the deployment or cache as in use, with an error that names the lock file and the matching unlock command. `exasol diag unlock` SHALL remove such locks of the selected deployment and report whether it removed one. While a running launcher of this version uses the deployment, `exasol diag unlock` SHALL fail with an in-use error. Its help SHALL state that it cannot detect whether an older launcher version is still running.

#### Scenario: Lock left by an older launcher version
- **WHEN** an older launcher version left a lock on a deployment
- **AND** a user runs `exasol status`
- **THEN** the result names the lock file and `exasol diag unlock`

#### Scenario: Clearing an older-version lock
- **WHEN** an older launcher version left a lock on a deployment
- **AND** a user runs `exasol diag unlock`
- **THEN** the command reports that it removed the lock
- **AND** the next command on that deployment is not reported as in use

#### Scenario: Nothing to clear
- **WHEN** no lock of an older launcher version exists on a deployment
- **AND** a user runs `exasol diag unlock`
- **THEN** the command reports that there was no lock to remove

#### Scenario: Unlock while this version uses the deployment
- **WHEN** `exasol shell` of this version uses a deployment
- **AND** a user runs `exasol diag unlock`
- **THEN** the command fails with an in-use error naming `exasol shell`
