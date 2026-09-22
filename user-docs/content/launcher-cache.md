# Runtime cache

The Exasol Launcher downloads tools and other runtime files on demand and reuses them across
deployments. It manages these files automatically; installing, using, and managing a database does
not require manual cache maintenance.

## Inspect the cache

When troubleshooting a reported download or integrity problem, inspect the cache without changing
it:

```bash
exasol diag cache
```

List the downloaded runtime artifacts with `exasol cache list`. See the
[CLI reference](cli-reference.md#exasol-cache) for the complete cache command surface.

## Clean a failed download

Preview and remove only artifacts that failed integrity checks:

```bash
exasol cache clean --invalid --dry-run
exasol cache clean --invalid
```

For a download that was interrupted before completion, use:

```bash
exasol cache clean --partial-downloads --dry-run
exasol cache clean --partial-downloads
```

The launcher downloads a removed artifact again when it is next required. Do not clear cache locks
while another launcher process might be running.
