## 1. Statement splitting

- [x] 1.1 Terminate `CREATE ... SCRIPT` / `CREATE ... FUNCTION` on a lone `/` line; ignore `;` in the body.
- [x] 1.2 Keep `;` termination for all other statements, unchanged (quote- and comment-aware).
- [x] 1.3 Make script-DDL detection whitespace- and comment-aware (a `SCRIPT`/`FUNCTION` word inside a comment is not a keyword).
- [x] 1.4 Buffer a recognized script with no `/` yet; flush the whole definition at end of input.

## 2. Tests

- [x] 2.1 Java/R script with body semicolons terminated by `/` parses as one statement.
- [x] 2.2 Normal statements, `ALTER`/`DROP`/`TRUNCATE`, CTAS, and `CREATE TABLE script_log` stay `;`-terminated.
- [x] 2.3 `CREATE /* a script */ TABLE ...` is not misclassified as a script.
- [x] 2.4 Unterminated script is buffered (parser) and flushed whole at EOF (interactive and non-interactive).

## 3. Validation

- [x] 3.1 Formatting, focused tests, and full-repo tests pass; OpenSpec validation for this change.
