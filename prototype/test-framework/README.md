# Pytest Test-Framework Prototype

This isolated prototype demonstrates the proposed fixture and annotation behavior without calling Exasol Personal or any cloud service.

The tests choose an explicit execution-boundary fixture. The internal deployment manager creates controlled or isolated deployments, reuses the shared deployment, and cleans up everything it owns. Short deterministic random delays make those lifecycle events visible without making test outcomes random.

Run from the `tests` directory:

```shell
# Default AWS demonstration
poetry run pytest --log-cli-level=INFO ../prototype/test-framework

# Critical path on Azure
poetry run pytest --log-cli-level=INFO -m smoke --provider=azure ../prototype/test-framework

# Provider filtering on Exoscale
poetry run pytest --log-cli-level=INFO --provider=exoscale ../prototype/test-framework

# Local macOS behavior
poetry run pytest --log-cli-level=INFO --provider=local --platform=macos-arm64 ../prototype/test-framework

# Recovery behavior with an isolated deployment
poetry run pytest --log-cli-level=INFO -m chaos ../prototype/test-framework

# Collection without execution
poetry run pytest --collect-only -q ../prototype/test-framework

# Three concurrent workers; shared tests stay together
poetry run pytest -n 3 --dist loadgroup -rP --log-cli-level=INFO ../prototype/test-framework

# Two isolated recovery cases running concurrently
poetry run pytest -n 2 --dist load -rP -m chaos --log-cli-level=INFO ../prototype/test-framework
```

Expected behavior:

- The first shared test creates one `shared-live` deployment.
- Later shared tests print that the same deployment is reused.
- Controlled, isolated, and local fixtures receive separate deployments.
- Function-owned deployments are cleaned immediately after their tests.
- The shared deployment is checked after every consumer and cleaned at session end.
- Each xdist worker owns a manager and uses worker-prefixed deployment IDs.
- Shared-deployment tests are grouped on one worker when using `--dist loadgroup`.
- Isolated tests can run concurrently because every test owns its deployment.
- Provider and platform restrictions appear as normal pytest skips.
- `-m smoke` and `-m chaos` use standard pytest marker selection.
- The pytest header reports the simulated target and the temporary STACKIT omission.

STACKIT is intentionally unavailable as a selectable provider in this prototype because its real deployment path is currently not working correctly.
