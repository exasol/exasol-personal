## 1. Public connection information (SPOT-33857)

- [x] 1.1 Introduce shared endpoint resolution and optional installation metadata; expose eligible endpoints in info and connection instructions with compatibility tests and endpoint documentation. Commit: `feat: Report public database node endpoints - SPOT-33857`.

## 2. Multi-node connections (SPOT-33856)

- [x] 2.1 Bound per-node transport attempts within the caller deadline while preserving login rejection behavior and connector compatibility, with focused deadline and real-driver rejection tests. Commit: `fix: Bound per-node database connection attempts - SPOT-33856`.
- [x] 2.2 Opt the Ubuntu preset into multi-node support, configure driver hosts with legacy fallback and shared-port validation, and add transport, TLS, older-launcher, and CLI evidence plus connection documentation. Commit: `feat: Connect through eligible database nodes - SPOT-33856`.

## Specification closeout

After the implementation tasks pass validation and review, archive the change and synchronize
permanent capability specifications in a separate `docs(openspec): Archive multi-node connections
change` commit.
