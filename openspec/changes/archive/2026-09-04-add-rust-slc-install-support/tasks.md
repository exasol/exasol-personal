## 1. Latest release resolution

- [x] 1.1 Resolve the latest `exasol-labs/language-container-rs` release tag from the GitHub
  releases API, with a bounded timeout.
- [x] 1.2 Build the asset download URL from the tag and the host architecture (`amd64` plain,
  `arm64` with the `-aarch64` suffix), rejecting other architectures before any network call.
- [x] 1.3 Cover resolution, architecture mapping, and lookup failures with unit tests against a
  local test server.

## 2. Alias dispatch

- [x] 2.1 Dispatch the reserved alias `rust` in `slc install` and `slc update` to the Rust install
  path before the official catalog lookup, keeping the existing flag set unchanged.
- [x] 2.2 Install through the existing custom-SLC path with the fixed alias `RUST` and language
  `rust`, so state, activation, restart handling, and the no-op comparison are inherited.
- [x] 2.3 Cover the dispatch, the absence of a `--source` flag, and the unchanged catalog behavior
  for other aliases with command-level tests.

## 3. User-facing contract

- [x] 3.1 Add `exasol slc install rust` to the user documentation's SLC examples
  (`user-docs/content/udfs.md`) and point at
  `slc custom install --alias RUST --language rust --source <path-or-url>` for a pinned build.
- [x] 3.2 Add a CHANGELOG entry with a command example.

## 4. Verification

- [x] 4.1 Run the focused SLC tests and the full Go test suite.
- [x] 4.2 Validate the OpenSpec change.
- [x] 4.3 Live-verify against a real Exasol Personal Local deployment: `exasol slc install rust`
  resolves the actual latest GitHub release (`v0.23.1`, after `exasol-labs/language-container-rs#91`
  fixed a `build_info/language_definitions.json` schema mismatch that made the database's
  `InitProcess` abort), installs cleanly, appears in `slc list` as active, is a no-op on
  reinstall/update, and `slc remove rust` removes it — all through the actual shipped alias
  dispatch, not a mocked path.
