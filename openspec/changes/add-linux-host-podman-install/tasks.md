## 1. Configuration Boundaries

- [x] 1.1 Add common `RuntimeConfig` embedding while retaining VM-only sizing in `VMConfig`
- [x] 1.2 Add typed `localinstall.StartConfig` and pass it through `LinuxHostRuntime`
- [x] 1.3 Parse and validate the Linux host DB port override without applying VM resource limits

## 2. Podman Lifecycle

- [x] 2.1 Detect an already-running deployment container before resolving the Nano artifact
- [x] 2.2 Load, identify, and deterministically tag the Nano image with contextual errors
- [x] 2.3 Start Nano with persistent `/exa`, the fixed container port, baseline options, and fresh-only init parameters
- [x] 2.4 Make stop and destroy container cleanup idempotent

## 3. Verification

- [x] 3.1 Add fake-Podman unit tests covering success, repeat start, configuration errors, and command failures
- [ ] 3.2 Run targeted Go tests, repository unit tests, lint, and build
- [x] 3.3 Update user-visible change documentation and validate the OpenSpec change
