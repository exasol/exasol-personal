# Presets

Exasol Personal uses presets to provision infrastructure and install the database. A preset is a
self-contained directory of templates and configuration files.

Each deployment combines:

- an infrastructure preset, which provisions a cloud or local environment; and
- an installation preset, which installs and configures Exasol on that environment.

Release 2.2 includes the `aws`, `azure`, `exoscale`, `stackit`, and `local` infrastructure presets.
The built-in `ubuntu` installation preset is selected by default where applicable.

List the built-in presets with:

```bash
exasol presets list
```

## Select a preset source

The `install` command accepts built-in names, filesystem paths, Git repositories, and archives:

```bash
exasol install <infrastructure-preset> [installation-preset]

exasol install aws
exasol install ./my-preset
exasol install https://github.com/org/preset.git
exasol install https://github.com/org/preset.git@v1.0
exasol install https://example.com/preset.tar.gz
exasol install file:///path/to/preset-dir
exasol install file:///path/to/preset.tar.gz
```

A local path starts with `.`, `/`, or `~`, or otherwise contains a path separator. Local directory
URIs are used directly. Local archives are extracted again on each run.

Git sources support HTTPS and SSH URLs. Append `@<branch-or-tag>` to select a ref. The launcher
caches a Git source by commit and reuses it on later runs.

Remote `.tar.gz`, `.tgz`, and `.zip` archives can use HTTP or HTTPS. They are downloaded on each run
because the source does not supply a checksum.

## Develop a preset

The preset manifest schemas, required output artifacts, caching rules, and reference implementations
are documented in the
[2.2 preset development guide](https://github.com/exasol/exasol-personal/blob/v2.2.0/doc/presets.md).
