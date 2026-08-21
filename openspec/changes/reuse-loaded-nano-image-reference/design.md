## Context

The shared Podman installer parses the reference reported by `podman load`, creates a deployment-specific local tag for it, and uses that alias in `podman run`. The parsed reference is already valid for both direct host and VM-backed Podman execution.

## Goals / Non-Goals

**Goals:**

- Use the loaded Nano image name or ID directly in the container run command.
- Keep the existing archive loading and output validation behavior.

**Non-Goals:**

- Remove deployment-specific aliases created by earlier launcher versions.
- Change Nano artifact resolution, caching, or cleanup.

## Decisions

1. Pass the parsed loaded-image reference directly to `podman run`. This handles both names and IDs while removing the redundant `podman tag` operation. Inspecting the archive separately was rejected because Podman already supplies the authoritative runtime reference.
2. Retain the current parser and its single-reference requirement. Broadening accepted Podman output is unrelated to removing the alias and would add compatibility risk.
3. Verify the command contract with the existing fake Podman executable for both a name and an ID. A deployment-level test was rejected because the unit boundary records the exact commands without requiring platform-specific Podman infrastructure.

## Risks / Trade-offs

- Existing redundant aliases remain after upgrading → avoid destructive image cleanup and stop creating new aliases.
- A loaded image may be reported only by ID → pass the ID through unchanged and cover it explicitly in tests.
