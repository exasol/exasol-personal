## 1. Report a stopped container host and guide database recovery

- [x] 1.1 Extend the direct-host preparer seam with a read-only question about whether
      whatever hosts the containers is running, answered from the Podman machine state on
      Windows and unconditionally on Linux
- [x] 1.2 Resolve a failed container probe against the container host in the direct-host
      runtime's status, so a stopped host reports a not-running container instead of failing
- [x] 1.3 Give `database_connection_failed` a recovery message, so every status `exasol
      status` reports carries guidance
- [x] 1.4 Cover both statuses with unit tests, including that reading the container host's
      state never starts it
- [x] 1.5 Document recovering a Windows deployment after a host reboot, and add changelog
      entries for both statuses

## 2. Verification

- [x] 2.1 Run repository validation (`task fmt`, `task lint`, `task all`)
