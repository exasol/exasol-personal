#!/usr/bin/env bash
# Common variables for scripts that run post-installation.

# Path to c4 executable for the DB
# Needed to hardcode this because we need to use exactly this c4 from this path
# It looks for its config in ../etc/c4.yaml or rather $HOME/.ccc/ccc/etc/c4.yaml
# We use the symlink in $HOME/.local/bin here though instead to abstract from these internals.
C4_PATH="$HOME/.local/bin/c4"
