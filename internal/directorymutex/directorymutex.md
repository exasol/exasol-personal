# Directory mutex

## Overview

This package provides cross-process directory locking with one marker file in the directory. Marker file contents are always empty; lock state is encoded only in the marker filename.

## Marker protocol

Exactly one marker file is allowed at any time:

- unlocked: no marker file
- exclusive lock: `.dirmutex.exclusive`
- shared lock with count `n`: `.dirmutex.shared.<n>` where `n >= 1`

Any malformed `.dirmutex.*` marker name or multiple markers are treated as invalid state.

## Locking semantics

Shared acquire:
- if unlocked: create `.dirmutex.shared.1` atomically (`O_CREATE|O_EXCL`)
- if shared `n`: atomically rename to `.dirmutex.shared.<n+1>`
- if exclusive: wait and retry

Exclusive acquire:
- if unlocked: create `.dirmutex.exclusive` atomically (`O_CREATE|O_EXCL`)
- if shared or exclusive: wait and retry

Acquisition retries every 500 ms until one of these happens:
- acquisition succeeds
- context is canceled or reaches deadline
- a default acquire timeout expires (when caller context has no deadline)

## Unlock semantics

Exclusive release:
- remove `.dirmutex.exclusive`

Shared release:
- if `n == 1`: remove `.dirmutex.shared.1`
- if `n > 1`: atomically rename `.dirmutex.shared.<n>` to `.dirmutex.shared.<n-1>`

Unlock operations retry every 500 ms on racing state transitions (`ENOENT`/`EEXIST`). Unlock is bounded by a default timeout and returns `ErrUnlockTimeout` if it cannot complete in time.

## Consistency goals

- updates are linearizable for cooperating processes using this protocol
- counter updates are atomic rename transitions
- timeout during acquire never applies a partial counter update
- timeout during unlock is treated as serious and returns an explicit error
