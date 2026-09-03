## Context

`init` and `install` take the preset selection as positional arguments rather than as subcommands, so a single Cobra command serves every preset. Both build their `Long` text at package scope from shared fragments and append the embedded preset list plus the compatibility matrix lazily, the first time help is actually requested, because resolving presets needs a request context that package initialization does not have.

The preset selection is already known before Cobra parses: the launcher scans the raw arguments up front so it can register only the selected preset's variable flags. Contextual help therefore needs no new resolution mechanism, only a reliable record of what that scan found.

## Goals / Non-Goals

- Goal: help for a selected preset describes that preset instead of every preset.
- Goal: help without a resolvable selection keeps its current content, since the top-level help points at `install --help` for the compatibility matrix.
- Non-Goal: changing preset resolution, flag registration, or deployment behavior.
- Non-Goal: restructuring the introduction or the deployment directory help.

## Decisions

### Record the selection separately from flag registration

The existing preset label annotation is set while registering a preset's variable flags, and that registration returns early for a preset that declares no variables. Reusing the annotation would therefore leave presets without variables looking unselected. The argument scan records the resolved selection in its own state instead, so help sees the selection regardless of whether the preset contributes flags.

### Compose the long description from named fragments

The generic help is the concatenation of an introduction, the deployment directory help, the positional argument explanation, and the preset discovery tip. Splitting the last two apart lets preset-specific help substitute the preset description for the positional argument explanation while keeping the discovery tip, and lets the generic text stay a plain concatenation whose output does not change.

### Treat a defaulted installation preset as unselected

When only one preset argument is given, the launcher resolves a default installation preset for the deployment. Presenting that default as if the user had chosen it would misreport the command line, so the installation preset is described only when a second argument selects it explicitly. The infrastructure preset's compatible installation presets are listed regardless, which is what a user needs in order to choose one.

### Derive compatibility from the manifests already read for the matrix

Compatible installation presets come from the same provided/required capability comparison that builds the compatibility matrix, so a preset pair is reported consistently by both forms of help. A preset selected by directory path is compared against the embedded installation presets the same way.

## Risks / Trade-offs

Usage and example lines are rewritten for the selected preset while help renders. This is confined to the help path, which prints and exits, so no command execution observes the rewritten values.

Help formatting is asserted by tests that compare against preset metadata reported by the preset listing command rather than against literal descriptions, so preset wording can change without breaking them.
