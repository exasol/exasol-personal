## 1. Tar.gz directory entry extraction

- [x] 1.1 Clean tar entry paths before extracting, so a directory entry's
      trailing separator no longer makes `os.Root` reject it
      (`internal/runtimeartifacts/targz_extractor.go`)
- [x] 1.2 Cover directory-entry extraction, including regular files nested
      under a directory entry, with a test
      (`internal/runtimeartifacts/targz_extractor_test.go`)
- [x] 1.3 Add a `CHANGELOG.md` entry under `Fixed` for the fix
