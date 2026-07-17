## ADDED Requirements

### Requirement: Script and function definitions terminate on a slash line

`exasol connect` SHALL terminate `CREATE ... SCRIPT` and `CREATE ... FUNCTION` definitions at
a line whose content is only `/`, rather than at `;`, so that semicolons inside a script or
function body are not treated as statement terminators. This applies to interactive input and
to `--command`/`--file`. All other statements SHALL continue to terminate at `;`.

#### Scenario: Script body containing semicolons

- **WHEN** a `CREATE JAVA SCALAR SCRIPT ... AS <body>` whose body contains `;` is submitted and terminated by a `/` on a line by itself
- **THEN** the whole definition is executed as a single statement, with the body semicolons preserved

#### Scenario: Normal statements still split on semicolons

- **WHEN** semicolon-separated non-script statements are submitted
- **THEN** they are split and executed on `;` exactly as before

#### Scenario: A script or function keyword inside a comment is not misdetected

- **WHEN** a non-script statement such as `CREATE /* a script */ TABLE t (id INT)` is submitted
- **THEN** it is treated as a normal `;`-terminated statement, not as a script definition

#### Scenario: Unterminated script definition is not split on body semicolons

- **WHEN** a recognized script/function definition is submitted without a closing `/`
- **THEN** it is not split at its body semicolons
- **AND** it stays buffered until a `/` arrives, and any buffered remainder is executed as one statement at end of input
