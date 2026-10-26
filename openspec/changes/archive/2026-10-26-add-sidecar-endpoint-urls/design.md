## Context

See proposal.md for motivation.

Three constraints shape the approach. A sidecar's saved definition is a subset
of a Kubernetes Container, which carries no scheme or path. Saved definitions
are authoritative once a sidecar is enabled, so anything status needs after the
catalog changes has to live in `sidecars.yaml`. The macOS runtime forwards a
declared host port to a runtime-assigned one, so the address a user can reach
is the observed one rather than the declared one.

## Goals / Non-Goals

**Goals:**

- One place that answers "what URL does this endpoint accept connections on",
  usable from both text and JSON status.
- Endpoint reporting that stays correct on a runtime that forwards ports.

**Non-Goals:**

- Changing which ports are published, or how they are published.
- Validating that anything actually answers at the reported URL.

## Decisions

**Declarations sit beside the container definition, not inside it.** Kubernetes
puts its application protocol hint on `ServicePort` and keeps `ContainerPort` to
addressing alone, so keeping the launcher's container subset faithful means
putting scheme and path elsewhere. The declaration's shape follows
`readinessProbe.httpGet`, which already names a container port and carries a
scheme and a path, so the idiom is familiar to anyone reading the catalog.

**One definition type carries the declarations.** A catalog entry and a saved
sidecar already share a `Definition` holding the container, so `endpoints` goes
on that definition and appears under the same `sidecars.<name>` key in both
files. Enabling copies the resolved definition across as it stands, and
validation reaches the declarations through the same path as the container.

**A declaration references its port by name.** `Port.Name` is already parsed and
validated as a unique DNS label, so naming gives it a purpose and keeps the
declaration stable when a port number changes. Validation requires the named
port to exist on the container, which catches the common typo.

**The URL resolves against the observed host address.** `sidecarEndpoints`
receives the declarations keyed by host port, which is the one address a
forwarding runtime and a directly binding one both report, and the value
reconciliation already compares against the saved definition. Resolving at
observation time is what keeps the reported URL reachable on macOS.

**Status reports one derived `url` field.** Scheme and path stay inputs in the
saved file; the endpoint object carries the single assembled value that a user
pastes or a script consumes. The URL is built with `url.URL` over
`net.JoinHostPort`, so an IPv6 loopback renders as `http://[::1]:18080/mcp`.

**A declaration on an internal port is inert.** Kubernetes treats a port
declaration as a hint rather than a publication request, and an unpublished port
has no host address a URL could name. Accepting it keeps a hand-edited
`sidecars.yaml` parseable while a user temporarily removes a `hostPort`.

**The container fingerprint stays over the container definition alone.** The
fingerprint is a label on a created container, so it exists to detect a gap
between what a user saved and what is running. Status reads declarations from
`sidecars.yaml` on every call and nothing bakes them into the container, so
hashing them in would report `pending` after a path edit and recreate a running
container to change a string that only status prints.

## Risks / Trade-offs

[A container running from an older definition whose host ports have since
changed cannot be matched to a declaration] → it reports its host and port
without a URL. Reconciliation already reports `pending` for that container, so
the condition is visible.
