## Source namespace

A release line already needs a branch to carry patches once the default branch has moved on. A
separate documentation branch for the same line duplicates that role, keeps documentation away
from the behavior it describes, and prevents reviewing a patch together with its documentation.
Branch sources are therefore accepted under `release/` and nowhere else, which keeps publication
from being requested against ordinary development work.

Release publication selects a tag and never inspects a branch, so a release branch changes no
release automation. Which commits reach a release branch is a release-process decision and is not
settled here.

## Version granularity

A release branch covers a line rather than a single release, so its name yields a version such as
`2.2`. Publishing per line also matches what readers select: consecutive patch versions differ
little, and listing each one makes the version selector harder to use. Full versions stay valid,
because mapping historical content to an exact version is still useful and pre-release
documentation still needs its own entry.

Ordering compares release components after padding a line to a full version, so `2.2` and `2.2.0`
occupy one position and cannot both claim to be the higher version. Documentation for a line that
is not released yet is published without the alias, which the explicit alias decision already
covers.
