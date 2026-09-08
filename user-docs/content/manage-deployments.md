# Manage deployments

Each local or cloud deployment keeps its configuration, state, and credentials in a deployment
directory. Keep this directory until you have destroyed the deployment resources.

## Select a deployment

The default deployment is stored in `default` under the managed deployments
directory listed in [Launcher files](#launcher-files). When you run a command
from an existing deployment directory, the launcher selects that deployment
automatically. When the launcher selects the default directory, it reports the
selected path on stderr before the command starts, including for interactive
sessions. The notice appears once and leaves stdout free for command output.

To maintain multiple deployments, give each one a case-sensitive name with `--deployment` or `-d`:

```bash
exasol install local -d demo
exasol install aws -d staging
exasol status -d demo
exasol connect -d demo -c "SELECT 1"
```

Named deployments live in that same managed deployments directory, under `<name>`. Names that
differ only by case may collide on the case-insensitive filesystems commonly used by macOS and
Windows.

Alternatively, select an arbitrary path with `--deployment-dir <path>`. You cannot combine
`--deployment-dir` and `--deployment`.

List the launcher-managed default and named deployments with:

```bash
exasol deployments list
```

Example output:

```text
default status=database_ready preset=local/ubuntu path=/Users/me/Library/Application Support/exasol/launcher/deployments/default
staging status=stopped preset=aws/ubuntu path=/Users/me/Library/Application Support/exasol/launcher/deployments/staging
```

The list does not include deployments selected through an arbitrary `--deployment-dir` path.

## Launcher files

The launcher keeps its own files in these platform-specific locations:

| Purpose | Linux | macOS | Windows |
| --- | --- | --- | --- |
| Managed deployments | `~/.exasol/launcher/deployments` | `~/Library/Application Support/exasol/launcher/deployments` | `%LOCALAPPDATA%\exasol\launcher\deployments` |
| SQL history | `~/.exasol/launcher/history/exasol_history` | `~/Library/Application Support/exasol/launcher/history/exasol_history` | `%LOCALAPPDATA%\exasol\launcher\history\exasol_history` |
| Cache settings | `${XDG_CONFIG_HOME:-~/.config}/exasol/launcher/resources.yaml` | `~/Library/Application Support/exasol/launcher/resources.yaml` | `%APPDATA%\exasol\launcher\resources.yaml` |
| [Runtime cache](launcher-cache.md) | `${XDG_CACHE_HOME:-~/.cache}/exasol/launcher/resources` | `~/Library/Caches/exasol/launcher/resources` | `%LOCALAPPDATA%\exasol\launcher\resources` |

Earlier launcher versions kept deployments and cache settings below
`~/.exasol/personal`. The first command after an upgrade moves deployments,
SQL history, and cache settings to the locations above, reports each move,
deletes the old runtime cache, and removes the old folders it leaves empty. The
move waits until no deployment at the old location is in use. Stop a running
local deployment at its old location first:

```bash
exasol stop --deployment-dir ~/.exasol/personal/deployments/<name>
```

When a deployment name exists at both locations, the launcher names both paths
and stops. Rename or remove one of them, then run the command again.

## Inspect or change configuration

An initialized deployment directory is tied to its infrastructure and installation presets. Retry a
failed installation with the same presets, or use these commands to inspect and change their
parameters:

```bash
exasol config get
exasol config set
exasol config reset
```

To select different presets in the same directory, first run `exasol destroy --remove`. If the
deployment resources have already been removed outside the launcher, run `exasol remove` instead.

## Stop and start

Stop a deployment when you do not need its compute resources:

```bash
exasol stop
```

For cloud deployments, networking and data volumes can continue to incur costs while compute
instances are stopped. See [Cloud resources and costs](cloud-resources.md#stop-or-destroy-a-deployment)
for provider-specific details.

Restart the deployment with:

```bash
exasol start
```

Node IP addresses can change after a restart. Check the command output or run `exasol info` before
connecting again.

## Destroy a deployment

Destroying removes the deployment resources and their data:

```bash
exasol destroy
```

The launcher asks for confirmation first. Pass `--auto-approve` to skip the prompt; a command with
no terminal attached proceeds without asking, so a script that reaches `exasol destroy` or
`exasol remove` destroys the deployment. See
[Unattended approvals](scripting.md#unattended-approvals-version-23-and-later) before automating
destructive commands.

By default, the launcher retains the deployment directory for inspection or recreation. Remove it
after successful destruction with:

```bash
exasol destroy --remove
```

If the resources were already deleted manually or are no longer accessible, remove only the local
deployment directory with:

```bash
exasol remove
```

`exasol remove` does not destroy resources. Deleting a deployment directory or the launcher itself
also does not remove provisioned resources. If the directory is lost before destruction, you must
remove the remaining resources directly through the target environment.

For local deployments, `exasol destroy` removes the launcher-managed runtime and database data for
the selected deployment. On Windows, it leaves Podman's shared default machine running.
