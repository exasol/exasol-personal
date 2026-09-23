# Scripting the launcher

Exasol Launcher commands are designed to be usable from scripts and agents. Select the deployment
explicitly with `--deployment` or `--deployment-dir` when a script must not depend on its working
directory. See [Manage deployments](manage-deployments.md) for selection rules.

## Handle output and exit status

Successful primary output is written to standard output. Progress, operational notices, prompts,
and text guidance are written to standard error, so redirect or parse the streams separately.

A non-interactive invocation of a command that supports `--json` writes exactly one parseable JSON
value to standard output when it succeeds. Operational notices can still appear on standard error.
Check each command's help to see whether JSON output is available.

Expected failures, including invalid input and unsupported operations, are reported on standard
error and exit with status `1`; successful commands exit with status `0`. A status reported inside a
successful JSON result is data, not the process exit status. `exasol connect --json` is a special
case: a SQL failure produces one structured JSON result on standard output and also exits with
status `1`.

For example, keep the JSON stream clean while preserving diagnostics:

```bash
if status_json=$(exasol status --deployment staging --json); then
  printf '%s\n' "$status_json" | jq -r '.status'
else
  printf 'Could not inspect staging\n' >&2
fi
```

## Interpret deployment status

`exasol status --json` reports one of these values in its `status` field:

| Status | Meaning |
| --- | --- |
| `not_initialized` | The selected deployment directory has not been initialized. |
| `initialized` | Configuration exists and is ready to deploy. |
| `operation_in_progress` | Another deployment operation holds the deployment lock. |
| `interrupted` | A deployment operation was interrupted before completion. |
| `stopped` | The deployment exists but its database is stopped. |
| `running` | The deployment infrastructure is running; database readiness was not established. |
| `deployment_failed` | Provisioning or installation failed. |
| `database_connection_failed` | The deployment is running, but the database did not accept the status check. |
| `database_ready` | The database is running and accepts connections. |

Treat the value as an explicit state instead of relying on its order in this table. The JSON result
can also contain `message` and `error` fields with recovery context.

## Execute SQL non-interactively

Pass SQL directly, read it from a file, or pipe it through standard input:

```bash
exasol connect --json --command "SELECT 1; SELECT 2"
exasol connect --json --file report.sql
printf 'SELECT 1;\nSELECT 2;\n' | exasol connect --json
```

`--command` and `--file` are mutually exclusive. These non-interactive JSON invocations execute
statements in order, stop at the first failed statement, and emit one JSON document for the whole
invocation. That document retains earlier successful statement records and adds a structured error
to the failed record. See [Connect to the database](connect.md) for result formats, row limits, and
interactive use.

## Unattended approvals (version 2.3 and later)

Use the global `--auto-approve` option when a script intentionally permits actions that normally
ask for confirmation.

Without an attached terminal, confirmation-based deployment and script-language-container actions
proceed automatically instead of waiting for input. This includes destructive actions such as
`destroy` and `remove`, so do not invoke them as a way to test whether approval is available.
Input validation and other safety checks still apply.

Host runtime preparation is the exception because it can change shared host state, for example by
installing Podman. An unattended command that needs such a change fails unless `--auto-approve` is
present; the error explains how to approve the change or perform it manually. See
[Run Exasol locally](local-deployment.md#windows-host-preparation) for the host preparation flow.

## Use agent skills

Exasol publishes agent skills for compatible coding agents. Follow
[Install with an AI agent](getting-started.md#install-with-an-ai-agent) to install the skills and let
an agent guide the setup workflow.
