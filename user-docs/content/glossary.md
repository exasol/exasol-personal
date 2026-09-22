# Glossary

## Cloud deployment

A [deployment](#deployment) whose infrastructure runs in a supported cloud account. See
[Deploy to the cloud](cloud-deployment.md).

## Deployment

One launcher-managed Exasol database and the local or cloud resources that support it. See
[Manage deployments](manage-deployments.md).

## Deployment directory

The local directory containing a deployment's configuration, state, credentials, and selected
presets. Keep it until the deployment is destroyed, and do not commit it to version control. See
[Select a deployment](manage-deployments.md#select-a-deployment).

## Exasol Launcher

The `exasol` command-line tool that installs and manages Exasol Personal. See
[Install the launcher](getting-started.md).

## Exasol Personal

A full Exasol database for personal use, deployed locally or into the user's cloud account and
managed with the Exasol Launcher.

## Infrastructure preset

A [preset](#preset) that defines and provisions the local or cloud environment in which Exasol is
installed. See [Presets](presets.md).

## Installation preset

A [preset](#preset) that installs and configures Exasol on provisioned infrastructure. See
[Presets](presets.md).

## Local deployment

A [deployment](#deployment) that runs on the user's macOS, Linux, or Windows computer, directly or
through a locally managed virtual machine. See [Run Exasol locally](local-deployment.md).

## Node

One compute host in a deployment. A deployment can contain one node or a multi-node database
cluster.

## Preset

A self-contained set of templates and configuration used to provision infrastructure or install
Exasol. Each deployment combines an infrastructure preset with an installation preset. See
[Presets](presets.md).

## Script language container

A container that supplies the runtime for user-defined functions in a language such as Python,
Java, R, or Rust. It is also called an SLC. See
[UDFs and script language containers](udfs.md).
