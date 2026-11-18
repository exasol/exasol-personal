## 1. Add Secret Volumes

- [x] 1.1 Add `volumes` beside the container and `volumeMounts` on it to the sidecar definition format, with validation naming offending fields and resolution of secret items into files. Add creation without start, copying into a container, and start to the Podman and Docker runtimes, and have the sidecar driver copy a definition's volume files into the created container before starting it. Compute the applied-definition label without sensitive values, keep an owner-only record of the sensitive values a container received, treat a container whose record differs as changed, and remove the record with its container. Add unit tests for parsing, validation, resolution, the copied archive, the label, rotation, and cleanup, a deployment test for start, and a live sidecar test marked `openspec("sidecar-configuration")` that serves the password from a volume file and inspects the container. Document secret volumes, the migration to a newer template, and the label in the user and contributor sidecar documentation, and add the changelog entries. Run `task all`. Commit as `feat: Deliver sidecar connection values through secret volumes - SPOT-34050`.

## 2. Validation

- [x] 2.1 On Linux with Podman and Docker, and on Windows, enable a sidecar with a secret volume, inspect the container, rotate the password, and start a deployment created by the previous launcher version.

## 3. Archive

- [x] 3.1 Archive the change into `openspec/specs`. Commit as `docs(openspec): Archive sidecar secret volumes change`.
