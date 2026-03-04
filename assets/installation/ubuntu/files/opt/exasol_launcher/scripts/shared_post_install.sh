#!/usr/bin/env bash
# Common variables for scripts that run post-installation.

# Path to c4 executable for the DB
# Needed to hardcode this because we need to use exactly this c4 from this path
# It looks for its config in ../etc/c4.yaml or rather $HOME/.ccc/ccc/etc/c4.yaml
# We use the synlink in $HOME/.local/bin here though instead to abstract from these internals.
# Unfortunately, it appears the installation process doesn't update PATH in .bashrc
# until some point later during the init (we should probably fix that)
# Otherwise we could have simply sourced $HOME/.bashrc
C4_PATH="$HOME/.local/bin/c4"

# Play ID (or deployment ID)
# Needed for subsequent c4 operations as most ops need to target a specific deployment
# We have only one deployment here, but it is better to make this explicit
PLAY_ID="$("$C4_PATH" config -e .play.id)"
