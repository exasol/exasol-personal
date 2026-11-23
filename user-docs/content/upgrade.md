# Upgrade the launcher

Stop your local deployments with `exasol stop` before upgrading, and start them with the new launcher
afterwards. Upgrade the Exasol Launcher by running the installer again:

```bash
curl https://downloads.exasol.com/exasol-personal/installer.sh | sh
exasol version
```

A launcher upgrade applies to new deployments right away. Existing deployments keep the database
version and configuration they were installed with, so some releases need a few manual steps for
existing deployments. Follow the section of each release between your previous launcher version and
the new one, oldest first. The [release notes](release-notes.md) list all changes.

## Upgrade to 2.5

### Launcher files move to a new location

The first launcher command after the upgrade moves deployments, SQL history, and cache settings
from `~/.exasol/personal` to platform-specific locations. A running or locked deployment stays at
the old location until you stop or unlock it. See [Launcher files](manage-deployments.md#launcher-files)
for the new locations and how to complete the move.

After the move, launcher versions before 2.5 no longer find your deployments.

### Remove the session limit from existing deployments

New deployments accept any number of database sessions. Deployments installed with an earlier
launcher version keep their limit of 20 concurrent sessions until you remove it. Removing the limit
restarts the database, which closes open sessions.

#### Local deployments

1. Stop the deployment:

    ```bash
    exasol stop
    ```

2. Find the deployment directory:

    ```bash
    exasol info --json | jq -r '.deploymentDir'
    ```

3. In `local/runtime/exa/exasol.conf` below that directory, delete this line from the
   `[DB_PARAMS]` section and save the file:

    ```ini
    maxConnectionsLicenseLimit=20
    ```

4. Start the deployment:

    ```bash
    exasol start
    ```

#### Cloud deployments

Cloud deployments take the session limit from their database license. Replace it with the Exasol
Personal license without a session limit:

1. Open a shell on the deployment host, fetch the license, and copy it to the database container's
   `/exa/tmp` directory, then leave the host shell:

    ```bash
    exasol shell host
    ~/.local/bin/c4 fetch -r @license:personal-unlimited
    cp ~/.ccc/x/u/branchr/v8-personal-unlimited-*/license.xml ~/.ccc/play/local/*/main/*/data/tmp/
    exit
    ```

2. Open a shell in the database container, upload the license, then leave the container shell:

    ```bash
    exasol shell container
    confd_client license_upload license: "\"{< /exa/tmp/license.xml}\""
    exit
    ```

    The upload prints the new license. It shows `max_session_limit: Unlimited`.

3. Restart the deployment so that the database applies the new license:

    ```bash
    exasol stop
    exasol start
    ```

Pass the same `--deployment` or `--deployment-dir` selector to each `exasol` command when you
upgrade a named or explicitly selected deployment.
