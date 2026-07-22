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

## Keep CLI output in the command layer

**Rule:** Command implementations own terminal presentation. Library packages return data and errors; they do not print user-facing output directly.

### Explanation

Terminal output is part of the CLI contract, not part of the deployment, configuration, or runtime libraries. Keeping it in the command layer preserves a clean separation between core logic and presentation, makes library code reusable outside a terminal, and keeps command output testable.

The command layer should build text and JSON output from the same returned domain objects. Library packages should accept and return objects, not pre-rendered CLI representations whose only purpose is to be printed. Text output may intentionally be shorter or more human-oriented, but JSON output must be complete enough for automation and agent workflows.

Use stdout only for primary command output. When `--json` is selected, stdout must contain only valid JSON for successful command output so callers can parse it directly without filtering. Human guidance, progress notes, current-directory notices, prompts, and call-to-action messages belong on stderr.

Expected errors are not successful command output. Unsupported features, invalid platform/backend combinations, validation failures, and other expected failures should be returned through the command error path so they are written to stderr and never mixed into stdout.

Route queued terminal output through the command helpers: use `addTerminalOutput` for primary stdout output and `addTerminalNotice` for stderr notices. This preserves ordering and keeps JSON stdout parseable when root-level notices are added.

