## 1. Docker-first engine selection

- [x] 1.1 Prefer Docker over Podman for `auto` on Linux and Windows while keeping Podman first for installation, with selection tests, CLI reference, documentation, and changelog. Commit: `feat: Prefer Docker over Podman for auto engine selection`.

## Specification closeout

After the implementation task passes validation and review, archive the change and synchronize
permanent capability specifications in a separate `docs(openspec): Archive Docker-first engine
selection change` commit.
