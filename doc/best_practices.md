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

### The output contract

Classify every piece of terminal output as one of three kinds and route it accordingly:

- **Primary output** — the result the user asked for. Goes to stdout. Under `--json` it is the only content on stdout and must be valid JSON, so callers can parse it without filtering.
- **Operational notice** — context a user needs to interpret the result, such as which deployment directory was used or that the license was accepted. Goes to stderr, and is shown even when stdout is piped or `--json` is selected. It never appears on stdout.
- **Call-to-action (CTA)** — decorative next-step guidance that nudges the user toward a follow-up command (for example "run `exasol deploy` to apply", an available-update hint, or a "Next steps" block). Goes to stderr, and is shown only to interactive users.

Use the delete-test to tell a CTA from a notice: if removing the message changes neither the result nor its correctness reporting, it is a CTA.

### Routing rules

- Send primary output to stdout only. Never mix prose, prompts, or guidance into stdout, and especially not into `--json` output.
- Send notices, prompts, progress, and CTAs to stderr. Placing human-facing messaging on stderr keeps stdout parseable when it is piped, and keeps `--json` stdout clean.
- Suppress CTAs only when `--json` is selected. CTAs are textual guidance that any reader benefits from — including a non-interactive agent driving the CLI in a workflow — so do not gate them on an interactive terminal. Under `--json`, consumers want structured output and should branch on structured state fields instead of prose, so CTAs are suppressed there. Keep CTAs on stderr rather than stdout: the stderr placement is a structural guarantee that stdout stays pure. (Ephemeral rendered output such as progress indicators, spinners, or color is different: gate that on an interactive terminal, since it corrupts logs and pipes.)
- Return expected failures — unsupported features, invalid platform/backend combinations, validation failures — through the command error path so they are written to stderr with a non-zero exit status and never mixed into stdout.

Route queued terminal output through the command helpers so ordering is preserved and JSON stdout stays parseable: primary output through the stdout helper, operational notices through the stderr-notice helper, and CTAs through the dedicated call-to-action helper that applies the suppression rules above.

