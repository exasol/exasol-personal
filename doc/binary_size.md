# Binary Size Optimization

This project optimizes raw launcher binary size without removing end-user features. The default build and release paths apply only low-effort techniques with acceptable operational tradeoffs.

## Decisions

Release and Task builds use:

- `CGO_ENABLED=0` to keep builds statically linked and consistent across release targets.
- `-trimpath` to remove local filesystem paths from the executable.
- `-ldflags="-s -w"` to omit symbol table and DWARF debug data.
- `-gcflags=all=-l` to disable inlining and reduce code size.

Higher-risk options are intentionally not used by default. UPX-style executable packing, alternative compilers, dependency replacement, and platform-specific build rewrites add more release, signing, debugging, or maintenance risk than their expected benefit justifies for this project.

## Technique Summary

| Technique | Effort | Risk | Tradeoff | Decision |
|---|---:|---:|---|---|
| `CGO_ENABLED=0` | Low | Low | Keeps release binaries static; may be unsuitable for future features requiring cgo. | Use by default. |
| `-trimpath` | Low | Low | Removes local build paths; stack traces use module/import paths instead. | Use by default. |
| `-ldflags="-s -w"` | Low | Low | Reduces debugger visibility by removing symbol and DWARF data. Panic stack traces remain usable. | Use by default. |
| `-gcflags=all=-l` | Low | Medium | Reduces size by disabling inlining; may affect performance and stack shape. | Use by default. |
| UPX or similar executable packing | Medium | Medium-High | Can reduce raw binary size further, but complicates signing, security tooling, startup behavior, and debugging. | Do not use. |
| Alternative compilers or libc/linker strategies | High | High | Platform-specific compatibility and toolchain risk. | Do not use. |
| Dependency replacement for size only | Medium-High | Medium | Requires behavioral replacement and broad testing for modest gains. | Do not pursue for size alone. |

## References

- Go build flags: [`go build`](https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies)
- Go linker flags: [`cmd/link`](https://pkg.go.dev/cmd/link)
- Go compiler flags: [`cmd/compile`](https://pkg.go.dev/cmd/compile)
- Stripping and executable packing overview: [Shrink your Go binaries with this one weird trick](https://words.filippo.io/shrink-your-go-binaries-with-this-one-weird-trick/)
- Executable packing tradeoffs: [UPX](https://upx.github.io/)
