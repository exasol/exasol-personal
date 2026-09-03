## Why

`exasol install local --help` and `exasol init local --help` render the generic introduction, the overview of every embedded preset, and the full compatibility matrix, so a user who already selected a preset reads mostly information about other presets instead of the one they chose.

## What Changes

- Describe the selected infrastructure preset and the installation presets compatible with it when a preset argument is present.
- Describe an explicitly selected installation preset as well.
- Show usage and examples for the selected presets.
- Keep the generic preset overview and compatibility matrix for `exasol install --help`, `exasol init --help`, and preset arguments that cannot be resolved.

## Capabilities

### New Capabilities

- `preset-contextual-help`: Defines how command help adapts to the preset selected on the command line.

### Modified Capabilities

None.

## Impact

This changes the help output of `exasol install` and `exasol init`, their tests, and the changelog entry. Deployment behavior, flag parsing, and other commands are unaffected.
