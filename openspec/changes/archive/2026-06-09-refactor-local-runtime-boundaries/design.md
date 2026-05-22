## Context

The Exasol Local VM backend was added inside `internal/deploy` because it needed to participate in the existing lifecycle commands quickly. That package now contains both deployment workflow orchestration and local runtime implementation details: runner staging, VM work directories, managed share preparation, SSH key generation, runner state parsing, and local deployment artifact generation.

This makes the package boundary misleading. The deploy workflow should coordinate lifecycle operations and state transitions, while the local runtime implementation should own the mechanics of managing the macOS Exasol Local runner. It also keeps local `deployment.json` shaped like cloud deployments by writing `nodes`, even though local exposes loopback endpoints and does not model deployable nodes.

## Goals / Non-Goals

**Goals:**

- Move local runner and VM runtime mechanics out of `internal/deploy`.
- Keep the local backend in the deployment workflow as a thin adapter.
- Make new local deployment artifacts endpoint-based and omit `nodes`.
- Keep cloud/tofu artifact shape and node-based shell behavior unchanged.
- Keep old node-derived metadata readable for existing deployments.

**Non-Goals:**

- Redesign the full backend interface.
- Move cloud/tofu implementation out of `internal/deploy` in this change.
- Remove node support from config models.
- Change the macOS runner command contract.
- Change user-visible lifecycle commands.

## Decisions

1. Create a dedicated local runtime package.

   The package should own local runtime paths, embedded/configured runner staging, VM initialization, start/stop/destroy commands, SSH key/share preparation, runner state parsing, and local runtime cleanup. It should expose a small API such as `Deploy`, `Start`, `Stop`, and `Destroy`, returning endpoint/runtime state needed by the backend.

   The deploy package should not know how the runner is staged or how the managed share is prepared.

2. Keep artifact mapping in the local backend.

   The local runtime package should return data, not write launcher artifacts. The local backend should map runtime endpoints into `config.DeploymentInfo`, `config.Secrets`, and connection instructions. This keeps launcher artifact ownership near the deployment workflow and avoids making the runtime package depend on workflow state decisions.

3. Local deployment artifacts use `connection`, not `nodes`.

   New local `deployment.json` files should contain loopback SQL, Admin UI, SSH, and shell metadata under `connection`. They should omit `nodes` because local does not have a node model.

   Existing config normalization remains node-aware so cloud deployments and older local artifacts continue to work.

4. Local shell access resolves SSH from local connection metadata.

   The tofu backend can continue using node-derived SSH details. The local backend should build its SSH connection from local `connection.sshPort`, loopback host, and the local runtime key path. This removes local shell dependence on `nodes`.

5. Keep the first refactor narrow.

   Connection instruction rendering can stay in `internal/deploy` for now. It is another cleanup candidate, but moving it in the same change would increase risk without being necessary for removing local runtime implementation from deploy.

## Risks / Trade-offs

- Moving code without changing behavior can still break local lifecycle paths -> keep function-level tests around runner staging, start/stop/destroy, and artifact generation.
- Removing local `nodes` can break hidden local dependencies -> add tests for local connect/info/status and shell metadata without `nodes`.
- Existing local deployments may already contain `nodes` -> preserve config normalization and shell fallback compatibility where practical.
- Package extraction can create import cycles -> keep local runtime independent of deploy workflow types; use config only for deployment paths where needed, or pass plain paths/options.
