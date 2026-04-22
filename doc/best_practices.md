# Best Practices

## Avoid `init()` Functions

**Rule:** Prefer explicit initialization by directly wiring functions and objects instead of using `init()`.

### Explanation

While `init()` functions are a supported Go language feature that may seem convenient, they introduce several problems:

1. They act as "magic" behind-the-scenes initialization
2. They create hard-to-track side effects
3. They reduce code maintainability due to poor locality
4. They make testing more difficult due to lack of isolation

The Go community at large discourages the use of `init()` functions in established best practices. This aligns with best practices from many other programming languages where similar features and patterns are universally discouraged for the same reasons (e.g., static initialization in C++).

Popular linters like `golangci-lint` report these issues for good reason, and many large Go projects avoid using `init()` functions. We should follow this practice as well.

### Further Reading

For more detailed information, consider these resources:
- [Understanding init Functions in Go](https://www.bytesizego.com/blog/init-function-golang)
- [Cobra Issue: Discourage init()](https://github.com/spf13/cobra/issues/1862)
- [Reddit Discussion on init() Best Practices](https://www.reddit.com/r/golang/comments/prtpqy/best_practices_regarding_init_function_and_small/)
- [StackOverflow: Is it bad to use init() functions?](https://stackoverflow.com/questions/56039154/is-it-really-bad-to-use-init-functions-in-go)

## Only print to the terminal in `cmd` packages

**Rule:** Do not use `fmt.Printf`, `fmt.Println`, or other functions that print directly to the terminal inside packages outside of your `cmd` layer.

### Explanation

The Go standard library’s `fmt` package provides functions like `Print`, `Println`, and `Printf` that write formatted output to standard output (`os.Stdout`) by default. These are useful in CLI entry points, but they hard-code a specific output destination (the terminal).

When code in lower-level packages prints directly to the terminal, it couples those packages to a particular execution context. Code that prints to stdout cannot be easily reused in environments where terminal output is inappropriate, such as servers, tests, background jobs, libraries, or GUI applications.

By restricting direct terminal output to the `cmd` package (or other designated boundary layers), you preserve separation between **core logic** and **presentation**. Other packages should return values or accept an `io.Writer` for output instead of printing directly. This makes logic testable (you can capture or mock output) and reusable in contexts beyond a terminal. A common idiom in Go is to use `fmt.Fprintf(w, ...)` with an `io.Writer` parameter when output must be produced programmatically, rather than writing straight to stdout. :contentReference[oaicite:1]{index=1}

## Keep backend-specific behavior behind backend interfaces

**Rule:** If behavior depends on the deployment backend, resolve the backend once and delegate that behavior to the backend implementation. Do not branch on backend identifiers in command code, and do not read backend-private deployment artifacts outside the backend implementation.

### Explanation

Backend-specific conditionals such as `if backend == "local"` in command orchestration code leak runtime concerns out of the runtime boundary. That makes the codebase harder to extend because every new backend-specific behavior gets copied into more commands instead of staying localized.

The `cmd` layer and the deployment orchestration layer should deal in stable backend capabilities and interfaces: lifecycle operations, diagnostic payloads, shell behavior, connection metadata, and similar contracts. Backend implementations may still use different files or schemas internally, but those details should stay hidden behind the backend abstraction.

This keeps backend behavior coherent, reduces duplication, and makes future backend additions or changes cheaper because most command code does not need to know how a specific backend stores or derives its state.

Backends should return data, not preformatted CLI output. JSON encoding, text rendering, and terminal printing belong in common launcher or `cmd` code, not in backend implementations.

## Make preset compatibility explicit in user-facing CLI output

**Rule:** When preset compatibility is constrained, CLI help and preset discovery commands must surface those constraints directly instead of forcing users to infer them from validation errors.

### Explanation

Compatibility metadata in manifests is necessary for correctness, but it is not sufficient for usability. If a preset is special, or if only certain infrastructure and installation presets can be combined, the launcher should say so in `--help` output and in preset discovery commands such as `exasol presets list`.

This is especially important for `local`, which is not just another infrastructure preset row. It selects a different backend, has different platform constraints, and only works with compatible installation presets. Making that relationship visible up front reduces failed attempts and keeps the command UX honest.


