# Glossary

The Exasol Launcher creates and manages deployments. One deployment is one Exasol database together
with the local or cloud resources that run it, built from a pair of presets and tracked in a
deployment directory on your computer. The terms below are listed alphabetically.

## Adapter

The component that lets Exasol read an external data source through a
[virtual schema](#virtual-schema). JDBC adapters run as Java UDFs and require a Java
[script language container](#script-language-container-slc). See
[Virtual schemas](virtual-schemas.md).

## BucketFS

The database's file store, used to stage files such as adapter and driver JARs where SQL statements
can reach them under `/buckets`. See
[Virtual schemas](virtual-schemas.md#where-local-deployments-keep-database-files).

## Deployment

One launcher-managed Exasol database and the resources that run it. A local deployment runs on your
macOS, Linux, or Windows computer; a cloud deployment runs in your own AWS, Azure, Exoscale, or
STACKIT account. See [Manage deployments](manage-deployments.md).

## Deployment directory

The local directory holding a deployment's configuration, state, credentials, and selected presets.
Keep it until the deployment is destroyed, and do not commit it to version control. See
[Select a deployment](manage-deployments.md#select-a-deployment).

## Deployment lock

A marker in the deployment directory that stops two launcher processes from operating on the same
deployment at once. `exasol status` reports `operation_in_progress` while it is held. See
[Troubleshooting](troubleshooting.md#status-is-operation_in_progress-but-nothing-is-running).

## Exasol Admin

The browser-based administration interface for a deployment, available on cloud deployments. See
[Use Exasol Admin](connect.md#use-exasol-admin).

## Exasol Launcher

The `exasol` command-line tool that installs and manages Exasol Personal. See
[Install the launcher](getting-started.md).

## Exasol Personal

A full Exasol database for personal use, deployed locally or into your own cloud account and managed
with the Exasol Launcher.

## Infrastructure preset

A [preset](#preset) that provisions the local or cloud environment the database runs on. The
built-in ones are `aws`, `azure`, `exoscale`, `stackit`, and `local`. See [Presets](presets.md).

## Installation preset

A [preset](#preset) that installs and configures Exasol on provisioned infrastructure. See
[Presets](presets.md).

## Node

One compute host in a deployment. Cloud deployments can run several; a local deployment is always a
single node. See
[Deploy to the cloud](cloud-deployment.md#choose-the-cluster-size-and-compute-type).

## Podman machine

Podman's virtual machine, used by local deployments on Windows. The launcher creates or starts the
shared default machine when needed and leaves it running when a deployment stops. See
[Windows host preparation](local-deployment.md#windows-host-preparation).

## Preset

A self-contained set of templates and configuration used to provision infrastructure or install
Exasol. Each deployment combines an [infrastructure preset](#infrastructure-preset) with an
[installation preset](#installation-preset), taken from a built-in name, a path, a Git repository,
or an archive. See [Presets](presets.md).

## Runtime cache

The launcher's local store of downloaded tools and runtime files, shared by all deployments and
maintained automatically. See [Runtime cache](launcher-cache.md).

## Script language container (SLC)

A container supplying the runtime for [user-defined functions](#user-defined-function-udf) in a
language such as Python, Java, R, or Rust. Cloud deployments include the standard containers; local
deployments install them on request. See
[UDFs and script language containers](udfs.md).

## Secrets file

The `secrets.json` file in the deployment directory, holding the database and Exasol Admin
credentials that clients need. See [Connect to the database](connect.md).

## User-defined function (UDF)

A function written in a script language and executed inside the database, inside a
[script language container](#script-language-container-slc). See
[UDFs and script language containers](udfs.md).

## Virtual schema

A schema that presents another database's objects as if they were Exasol objects, backed by an
[adapter](#adapter). See [Virtual schemas](virtual-schemas.md).
