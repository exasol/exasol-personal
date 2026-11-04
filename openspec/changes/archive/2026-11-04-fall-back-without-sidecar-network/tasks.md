Each implementation task below maps to exactly one commit that passes the full
check suite. Group 3 is validation and produces no commit.

## 1. Fall back without the service network

- [x] 1.1 Start the database without the deployment network when creating it on
  the network fails with the nftables error, label that container as lacking
  the service network, refuse sidecar enable without saving the sidecar, and
  report enabled sidecars failed at start and in status with the WSL or kernel
  guidance while the label is set. Verify `task tests-unit` covers the
  fallback (including a fresh install keeping its initial password), no label
  after a later start on the network, other network errors surfacing
  unchanged, the enable error leaving the sidecar unsaved, the start failure,
  sidecar status, no restart-required report for a fallback database without
  sidecars, and the WSL and non-WSL guidance.

## 2. Documentation

- [x] 2.1 Rewrite the nftables troubleshooting section for sidecars, the
  Windows launcher, other Linux kernels and Docker, including the restart after
  either fix; add the WSL 2.7 requirement for sidecars on Podman to the system
  requirements; and update the sidecar changelog entry.

## 3. Validation

- [x] 3.1 Run `task all` and confirm formatting, linting, unit tests and the
  launcher tests pass on the finished branch.
- [x] 3.2 On WSL 2.6, with the Linux and Windows launchers, check install
  without sidecars, the enable error, start with an enabled sidecar, and
  sidecars serving after `wsl --update` or the iptables fix followed by stop
  and start.
- [x] 3.3 On WSL 2.7 or later, check install, sidecar enable without a
  restart, and stop and start.
