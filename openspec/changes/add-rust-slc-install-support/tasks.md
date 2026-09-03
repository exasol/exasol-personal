## 1. Latest release resolution

- [ ] 1.1 Resolve the latest `exasol-labs/language-container-rs` release tag from the GitHub
  releases API, with a bounded timeout.
- [ ] 1.2 Build the asset download URL from the tag and the host architecture (`amd64` plain,
  `arm64` with the `-aarch64` suffix), rejecting other architectures before any network call.
- [ ] 1.3 Cover resolution, architecture mapping, and lookup failures with unit tests against a
  local test server.

## 2. Alias dispatch

- [ ] 2.1 Dispatch the reserved alias `rust` in `slc install` and `slc update` to the Rust install
  path before the official catalog lookup, keeping the existing flag set unchanged.
- [ ] 2.2 Install through the existing custom-SLC path with the fixed alias `RUST` and language
  `rust`, so state, activation, restart handling, and the no-op comparison are inherited.
- [ ] 2.3 Cover the dispatch, the absence of a `--source` flag, and the unchanged catalog behavior
  for other aliases with command-level tests.

## 3. User-facing contract

- [ ] 3.1 Add `exasol slc install rust` to the README SLC examples and point at
  `slc custom install --alias RUST --language rust --source <path-or-url>` for a pinned build.
- [ ] 3.2 Add a CHANGELOG entry with a command example.

## 4. Verification

- [ ] 4.1 Run the focused SLC tests and the full Go test suite.
- [ ] 4.2 Validate the OpenSpec change.
