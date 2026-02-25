# Overview

This package implements the `exasol` executable.

Packages within the `cmd/` directory are responsible for:

- Cobra configuration
- Logging configuration
- Argument type conversion and simple validation

Any code that is not specific to setting up a CLI executable, or
is otherwise suitable for unit testing, likely belongs in `internal/` or `pkg/`.

## Preset selection + infrastructure variables

Some commands (notably `init` and `install`) accept infrastructure variables that depend on the selected infrastructure preset.

Infrastructure variables are registered as regular Cobra flags (prefixless) for the selected preset only, e.g.:

- `--cluster-size 2`
- `--instance-type r6i.xlarge`

This keeps parsing simple (Cobra handles `--flag value` and `--flag=value`) while ensuring Cobra never sees variables from multiple infrastructure presets at the same time.

Preset selection uses explicit flags:

Preset selection uses positional arguments:

- `exasol init <infra preset name-or-path> [install preset name-or-path]`
- `exasol install <infra preset name-or-path> [install preset name-or-path]`

The infrastructure preset is required.
The installation preset is optional and defaults to the embedded installation preset.

Each preset argument can be either an embedded preset name (e.g. `aws`) or a preset directory path.
To force path selection, pass a path-like value such as `./my-preset` or `/abs/path/to/preset`.

