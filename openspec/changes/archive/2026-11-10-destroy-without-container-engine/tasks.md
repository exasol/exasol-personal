## 1. Destroy Without the Engine

- [x] 1.1 Add destroy options carrying a confirmation for a local deployment whose container engine command does not resolve, presented by the `destroy` command only in a terminal with the deployment's container and network and the commands that remove them. Have the local backend ask before its engine-backed destroy, and on acceptance remove the deployment's data, runtime files, and local artifacts without the engine. Add unit tests for accepting, declining, unattended runs, and a resolvable engine, and a launcher test marked `openspec("docker-host-runtime")` for the unattended failure. Document the prompt in the troubleshooting page and add the changelog entry. Run `task all`. Commit as `fix: Destroy a local deployment whose container engine was uninstalled - SPOT-33896`.

## 2. Validation

- [ ] 2.1 On Windows, destroy a Docker deployment after uninstalling Docker Desktop, accepting and declining the prompt, and with `--auto-approve`.

## 3. Archive

- [x] 3.1 Archive the change into `openspec/specs`. Commit as `docs(openspec): Archive destroy without container engine change`.
