# Deployment directory compatibility

The deployment directory is a long-lived artifact. To protect users from accidentally running an incompatible launcher against an existing deployment directory (which can corrupt state, break workflow guarantees, or leave infrastructure in an undefined state), the launcher enforces a **mandatory compatibility check** before executing any deployment-directory-dependent command.

This compatibility mechanism is distinct from release/update notifications. For update checking, see [Version checking](version_checking.md).

## Concepts

### Deployment version

When a deployment directory is created, the launcher persists the launcher version as part of the deployment’s persistent state.

This stored version is the **deployment version**:

- It is the authoritative version marker for that deployment directory.
- It is treated as part of the deployment contract.
- It is **immutable** for the lifetime of the deployment directory.

### Launcher version

The **launcher version** is the version of the currently running `exasol` binary.

### Command compatibility range

Each command that operates on a deployment directory defines the range of deployment versions it supports.

At minimum, a command declares a **minimum supported deployment version**.

This allows compatibility to evolve gradually:

- Read-only commands can remain compatible with a wider range of deployments.
- Lifecycle and mutating commands can require stricter compatibility.

## Compatibility rules (enforced for every command)

Before doing any mutation or external interaction, commands that operate on a deployment directory must validate compatibility using a centralized mechanism.

This check is implemented in an abstract, generic fashion and is mandatory for **all commands** that interact with a deployment directory.

The decision rules are:

1. **Deployment newer than launcher:**
   - If $\text{deployment version} > \text{launcher version}$, the command fails immediately.
   - This is rejected **on principle**: an older launcher cannot reliably claim compatibility with a deployment created by a newer launcher, because future changes to state, contracts, or workflow semantics are not knowable to the older binary.
   - The required action is to **upgrade** the launcher.

2. **Deployment older than launcher:**
   - If $\text{deployment version} < \text{command minimum supported deployment version}$, the command fails immediately.
   - The required action is to **use a compatible launcher version** (typically by downgrading).

3. **Supported:**
   - Otherwise, the command is allowed to proceed.

## Failure behavior and messaging

On incompatibility, the launcher:

- Exits with a non-zero status.
- Produces a clear, actionable message that includes:
  - Deployment version
  - Current launcher version
  - The reason execution is blocked
  - The required action (upgrade or use a compatible older launcher)

## Stability of the version marker

The compatibility mechanism depends on being able to read the deployment version reliably.

For that reason, the deployment version is recorded in a dedicated plain-text marker file (`.exasolLauncher.version`).

This marker’s presence, identity, and location are treated as a long-term contract and must remain stable over time.

This ensures that even as the launcher evolves, it can still detect an incompatible deployment directory early and fail fast rather than proceeding with undefined behavior.

## Forward evolution

This model supports safe evolution of the launcher and deployment contract:

- Breaking changes can be introduced without silently corrupting existing deployments.
- Individual commands can tighten their supported range over time.
- A future migration workflow can be added explicitly (for example, a dedicated migration command), without weakening the “fail fast on incompatibility” guarantee.
