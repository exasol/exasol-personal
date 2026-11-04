## Context

Netavark sets up Podman networks with nftables rules that need the kernel's
`NFT_FIB_INET` and `NFT_FIB_IPV6` support. The WSL 2.6 kernel (6.6.87.2) lacks
it and fails with `netavark: nftables error`; WSL 2.7.0 (6.6.114.1) and later
have it. Both were verified on the same machine with the Linux and Windows
launchers. Docker uses iptables and is unaffected. Rootless Podman cannot
connect a running container in its default `pasta` network mode to a network
(`"pasta" is not supported: invalid network mode`), while a container started
on a bridge network can join further networks live. This is why the database
starts on the deployment network before any sidecar is enabled.

## Goals / Non-Goals

**Goals:** the database always starts on hosts without nftables support;
sidecars are refused there with actionable guidance; supported hosts behave as
today.

**Non-Goals:** detecting the WSL version, restarting the database from
`sidecar enable`, and attaching the network only when a sidecar is enabled,
which would need a restart on every first enable under Podman.

## Decisions

- **Fall back on the specific error only.** When creating the database on the
  network fails and the runtime's error contains `netavark: nftables error`,
  the database is created again without the network. Exit codes differ between
  Podman commands, so the text is matched. Any other failure fails the start as
  today.
- **Fall back inside the existing create.** The retry reuses the same run
  spec without the network, after the bootstrap password is written and before
  the existing port-bind classification and failure cleanup, so a retry failure
  is reported as any start failure is today. The error text comes from the
  runtime's captured stderr (`CommandStderr`).
- **Record the outcome on the database container.** The fallback container
  carries a label marking the service network as unsupported. Each start that
  is not already running recreates the container, so the label always reflects
  how the running database was created, needs no new state file, and is gone
  after a start on the network. Sidecar enable and status read it through the
  existing container attachment inspection instead of probing; it cannot go
  stale while the database runs, because fixing the host (updating WSL or
  switching Podman to iptables) only takes effect for a database created after
  the fix.
- **Unsupported is not restart required.** While the label is set, the
  database restart check reports no restart, so status does not show the
  stop/start call to action for a deployment without sidecars, and sidecar
  apply returns a typed unsupported error that enable reports instead of
  swallowing it as it does the restart requirement. Start-time reconcile records
  that error against each enabled sidecar through the existing per-sidecar
  failure recording.
- **A refused enable saves nothing.** The failure is a lasting property of the
  host, unlike a sidecar's own start failure, so keeping the sidecar would make
  every later start report it until the host is fixed.
- **Start reports sidecars as today.** With an enabled sidecar, the database
  starts and start reports the sidecar failure as it does for any sidecar,
  which only happens after enabling on a stopped deployment or rolling WSL
  back.
- **Quiet without sidecars.** A fallback with no enabled sidecar only logs at
  debug level, because the deployment does not need the network.
- **Start succeeds after a fallback.** Returning an error would leave the
  database down and, on macOS, stop the VM, because the start path is shared.
- **WSL-specific guidance.** The Windows launcher, or a Linux kernel release
  containing `microsoft`, gets the `wsl --update` guidance; other hosts get the
  generic kernel guidance.

## Compatibility

Supported hosts never hit the fallback, so the database starts on the network
and sidecars attach live as today, on Linux, Windows and macOS, with Podman or
Docker. Disable, destroy, the restart-required flow for a database without its
alias, port recovery, and SLC restarts are unchanged.

## Risks / Trade-offs

- Each start on an unsupported host makes one failed create attempt before the
  fallback, adding about a second.
- Supported hosts keep an unused network until a sidecar is enabled, as in the
  v2.4.0 release candidates.
- The stderr match depends on netavark's wording; unit tests pin the reported
  text.
