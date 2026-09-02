## Context

The custom-SLC command currently normalizes language values against a hard-coded list of
Python, Java, and R. The Exasol UDF protocol is implemented by the client executable inside the
SLC; the `lang` query parameter is part of the client contract. The Rust language container
demonstrates a compatible third-party client using `lang=rust` without database changes.

## Goals / Non-Goals

**Goals:**

- Allow custom SLC clients to declare identifiers beyond the official SLC flavors.
- Keep identifiers safe for activation URLs, persisted state, and SQL construction.
- Preserve normalization and comparison behavior for existing Python, Java, and R installations.

**Non-Goals:**

- Validate that an archive actually implements the declared language; archive validation remains
  focused on the required executable.
- Add a registry of supported third-party languages.
- Change the database or SLC protocol.

## Decisions

### Validate syntax, not a closed vocabulary

Replace the closed language enum validation with a non-empty, normalized identifier validation.
The launcher trims leading and trailing whitespace, converts the identifier to lowercase, and
requires it to start with an ASCII letter or digit and contain only ASCII letters, digits, dots,
hyphens, or underscores. This lets the launcher pass `rust` or another
client-defined value through while preventing whitespace, control characters, URI delimiters, or
SQL injection characters from changing the generated URI.

A closed vocabulary is rejected because the launcher cannot own the list of clients implemented by
custom SLC authors.

### Keep the language value in activation and state

The normalized identifier remains part of the activation URI and persisted state. This preserves
the existing update no-op comparison and allows `slc list` to report the client identifier.

### Keep the existing public option

Retain `--language`; it is the correct concept for the value passed to the SLC client. Update its
help text to say that it identifies the runtime/client supplied by the custom SLC.

## Risks / Trade-offs

- [A custom archive may not support its declared identifier] → Keep the existing archive and
  activation verification, and report runtime activation failures clearly.
- [The accepted syntax may be narrower than a future client protocol] → Use a documented safe
  identifier grammar and extend it only when a real client requires it.
- [Existing state uses only the three official values] → Preserve lowercase normalization so
  existing state remains compatible.
