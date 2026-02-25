# Version checking

Exasol personal makes calls to a REST API once per day, per repository.

To determine if 24 hours has passed, the time of the last version check is written to the `.exasolLauncherState.json` file in the deployment directory.

Every subcommand of `exasol`, with the exception of `version`, attempts to perform the version check.

If a command is run from outside of a deployment directory, the version check is skipped. Note that `init` and `install` always run the version check directly. This is because these commands are expected to be run from outisde of a deployment directory.

If the call to the REST API fails or times out, the attempt is still recorded as a version check and another attempt will not be made for the same deployment directory for 24 hours.

The environment variable EXASOL_VERSION_CHECK_URL can be used to override the default API endpoint. This is used in the tests.
