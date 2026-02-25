# Exasol Cleanup (standalone)

A small, standalone utility to discover and clean up Exasol Personal deployments on AWS.

This internal tool is written and maintained almost exclusively using agentic AI. It's not part of any official release, not officially supported and purely internal.
Feel free to use if you find it useful though.

## Usage

Build:

```
task build
```

Discover deployments:

```
./bin/exasol-cleanup discover --owner=*
./bin/exasol-cleanup discover --legacy --owner=*
```

Show details:

```
./bin/exasol-cleanup show exasol-123ad553
```

Run cleanup (dry-run by default):

```
./bin/exasol-cleanup run exasol-123ad553 
# To actually delete resources (dangerous):
./bin/exasol-cleanup run exasol-123ad553 --execute
```
