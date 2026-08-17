## 1. Spec And Default Resolution

- [x] 1.1 Remove the hardcoded local preset memory default that forces 2048 MB
- [x] 1.2 Compute the macOS local VM memory default as approximately 50% of total host memory in the local backend

## 2. Verification

- [x] 2.1 Update local backend tests for the new default resolution behavior
- [x] 2.2 Update local install integration coverage that currently expects 2048 MB by default

## 3. Memory Validation

- [x] 3.1 Reject macOS host memory below 8192 MB with a short user-facing error
- [x] 3.2 Reject user-configured local VM memory below 4096 MB
- [x] 3.3 Add focused tests for host-memory and configured-memory validation
