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




