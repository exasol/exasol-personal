## Context

Saved sidecar definitions describe what a deployment wants to run. Turning them
into running services needs a component that owns containers, networking, and
published endpoints. That component is platform-specific: Podman runs directly
on the user's machine on Linux and Windows, and inside a VM guest on macOS,
where the host port a user connects to is forwarded rather than bound.

Deployment backends differ more than their runtimes do. A backend that reaches
an external service to run sidecars shares no container handling with one that
drives Podman locally, so the launcher gains more from a narrow capability both
can answer than from a shared runtime both feed.

## Goals / Non-Goals

**Goals:**

- Keep one declarative entry point that turns a desired set of sidecars into
  running services.
- Keep the backend interface free of container-runtime vocabulary.
- Let each local runtime own the publication its platform requires.
- Keep reconciliation verdicts in one place, so status has one definition.
- Report sidecars as unsupported where a backend does not run them.

**Non-Goals:**

- Run sidecars on cloud deployments.
- Report sidecar readiness or health beyond process state.

## Decisions

1. Give the deployment backend a sidecar capability rather than extending the
   shared backend interface.

   Sidecars are answered by local deployments only. The entry points resolve
   the deployment's backend kind and report any other kind as unsupported, so
   no sidecar vocabulary reaches the shared deployment interface or the cloud
   backend.

2. Apply the whole desired set in one call.

   Applying receives every sidecar the deployment wants and makes reality
   match: it creates what is missing, replaces what changed, and removes what is
   no longer desired. Published endpoints follow the same set, so withdrawing an
   endpoint needs no separate call. Disable becomes a document edit followed by
   one apply, and an interrupted operation converges on the next one.

3. Report observations as backend-neutral facts.

   `InspectSidecars` reports, per sidecar, whether it runs, its exit code,
   whether it was created from the definition that was applied, and the
   endpoints a user can reach. Two of those need backend knowledge: matching
   reads the salted definition label, and endpoints resolve whatever the runtime
   published on top of what the container bound. The remaining comparison
   against saved definitions stays in the deployment layer, which gives status
   one definition of `complete`, `pending`, and `unknown`.

4. Let each local runtime open its own ports.

   A runtime whose containers bind on the user's machine has nothing to open. A
   runtime behind a VM boundary forwards the declared host port into the guest
   and withdraws forwards it no longer serves. The local backend asks its
   runtime and reports the resulting endpoints, so the platform difference stays
   inside the runtime that has it.

5. Report a short cause for a failed start and point at status.

   A sidecar that fails to start reports its name and a short cause, with a call
   to action to run `exasol sidecar status`. The full diagnostic is recorded
   with the sidecar and surfaced there. This gives every startup failure the
   same handling, whether the cause is an occupied port, an unavailable image,
   or a rejected entrypoint.
