## Alias selection

Publication order and version order diverge whenever documentation for an older release is
published after a newer one. Version order is the property readers rely on, so automatic alias
selection compares the requested version against the published catalog and grants `latest` only
when no higher stable version exists. Comparison is limited to stable versions, whose precedence
is the numeric order of their release components, so no version-parsing dependency is required.

Automatic selection cannot express publishing documentation ahead of its announcement, or
restoring `latest` to an earlier version after a withdrawn release. A publication input therefore
overrides the comparison in either direction. Forcing `latest` onto a pre-release is rejected,
because the site root resolves through `latest` and has to reach a stable version.

## Alias exposure

The version selector reads its configuration from the page it is served on, so the alias only
becomes visible on versions built after the configuration changes. Existing versions show it once
they are republished, which is a normal publication and needs no migration.

Aliases stay redirects rather than copies. A `latest` URL therefore remains linkable but resolves
to the versioned URL, which keeps one canonical address per page and avoids storing a second copy
of the site for every stable release.
