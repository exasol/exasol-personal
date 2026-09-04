## Source namespace

Revising released documentation needs a source that accumulates commits, which no immutable
reference provides. Accepting any branch would allow publication from development branches, so
branches are accepted only under `docs/`. The prefix states the intent of the branch, keeps the
existing guarantee that publication cannot be requested from ordinary development work, and can be
widened later without changing published state.

Reserving `docs/` for branches also removes an existing conflict. Release version derivation
selects the most recent version tag, so a documentation tag sharing the version namespace is
indistinguishable from a release tag and has to be filtered out explicitly. Branch names never
participate in that selection.

A source name that exists as both a branch and a tag resolves silently to the branch during
checkout, which would let an unchanged request publish different content once a branch appears.
Such a name is therefore rejected, and a fully qualified reference selects one deliberately.

## Validation placement

Source classification stays in the workflow and runs before the selected snapshot supplies any
executable content, so a rejected source never reaches the publishing environment. Version
derivation stays in the helper script, where it is covered by the documentation tooling tests.

## Provenance

A branch tip moves, so the published version no longer identifies its source by name alone. The
resolved commit is recorded in the run summary and in the published version's commit message,
which keeps every published version traceable to exact content.
