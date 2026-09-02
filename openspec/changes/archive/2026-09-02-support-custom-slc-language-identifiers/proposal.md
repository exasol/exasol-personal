## Why

Custom SLCs provide their own `exaudfclient` and can implement the Exasol UDF protocol for
additional runtimes, such as Rust. The current custom-SLC command rejects every language except
Python, Java, and R, preventing users from registering compatible custom clients.

## What Changes

- Accept syntactically valid custom SLC language identifiers instead of a fixed language list.
- Use the supplied identifier in activation URLs and persisted custom-SLC state.
- Update CLI help and documentation to describe the language as the client runtime identifier.
- Preserve clear validation for empty, unsafe, or malformed identifiers.

## Capabilities

### New Capabilities

- `custom-slc-language-identifiers`: Support custom SLCs identified by compatible runtime names.

### Modified Capabilities

## Impact

- Changes custom-SLC validation, CLI help, documentation, and tests.
- Does not change the Exasol database or SLC protocol.
- A supplied identifier remains subject to the custom SLC's client implementation; accepting it
  does not guarantee that the archive can execute successfully.
