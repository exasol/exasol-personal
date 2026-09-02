## ADDED Requirements

### Requirement: Custom SLC language identifiers are client-defined

The custom SLC install and update commands SHALL accept any non-empty language identifier that
matches the launcher's safe identifier grammar, rather than limiting values to the official SLC
flavors. The launcher SHALL trim leading and trailing whitespace, normalize the identifier to
lowercase, and require it to start with an ASCII letter or digit and contain only ASCII letters,
digits, dots, hyphens, and underscores. The launcher SHALL use the normalized identifier in the
SLC activation URI and persisted custom-SLC state.

#### Scenario: Install a custom Rust SLC

- **WHEN** the user installs a valid custom SLC with `--language rust`
- **THEN** the launcher accepts the language, persists `rust`, and activates the SLC with
  `lang=rust`

#### Scenario: Existing official custom languages remain supported

- **WHEN** the user installs a custom SLC with `--language " PYTHON "`
- **THEN** the launcher persists and activates it with the normalized identifier `python`

#### Scenario: Invalid language identifier is rejected

- **WHEN** the user supplies an empty language identifier, or one that contains whitespace after
  trimming leading/trailing spaces, or delimiter characters
- **THEN** the launcher rejects the command before staging or activating the SLC

#### Scenario: Language identifier is not treated as archive validation

- **WHEN** the user supplies a syntactically valid language identifier whose custom client does
  not implement the Exasol UDF protocol
- **THEN** the launcher may accept and stage the archive, but activation or execution reports
  the client failure
