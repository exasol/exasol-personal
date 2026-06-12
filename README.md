<div align="center">

<picture>
  <source srcset="static/Exasol_Logo_2025_Bright.svg" media="(prefers-color-scheme: dark)">
  <img src="static/Exasol_Logo_2025_Dark.svg" alt="Exasol Logo" width="300">
</picture>

# Exasol Personal

**The Analytics Database for Agentic AI — Free for Personal Use**

*Deploy a full-scale Exasol database on your own infrastructure in minutes*

[![Documentation](https://img.shields.io/badge/docs-exasol.com-blue)](https://docs.exasol.com/db/latest/home.htm)
[![Community](https://img.shields.io/badge/community-exasol-green)](https://community.exasol.com)
[![Downloads](https://img.shields.io/badge/downloads-exasol.com-orange)](https://downloads.exasol.com/exasol-personal)

</div>

## 🔥 Key Features

- 🤖 **Built for Agentic AI** — Connect AI agents and LLM tools directly through a scriptable CLI
- 🧠 **Built-in AI Functions** — Leverage native AI/ML capabilities with GPU acceleration, right where your data lives
- 🏎️ **In-Memory Performance** — Run complex analytics at in-memory speed with Exasol's industry-leading analytics engine
- 🏢 **Full Enterprise Features** — Access all enterprise-scale capabilities of the Exasol database, completely free for personal use
- ♾️ **Unlimited Data** — Store and analyze unlimited amounts of data with no artificial limits
- 📈 **Scalable Architecture** — Scale up to any number of nodes using Exasol's MPP (Massively Parallel Processing) architecture
- ⚙️ **Simple Deployment** — Spin up Exasol on AWS, Azure, Exoscale, STACKIT, or your local system with just a few commands
- 🖥️ **Cross-Platform CLI** — Install and manage your cluster using the Exasol Launcher on Linux, macOS, or Windows


## ✅ Prerequisites

A cloud account on one of the supported platforms with permission to provision compute instances, or a macOS Apple Silicon system for local deployment:

- **AWS** — [Set up an AWS account for Exasol Personal](./HOWTO_SETUP_AWS_ACCOUNT.md)
- **Azure** — [Set up an Azure account for Exasol Personal](./HOWTO_SETUP_AZURE_ACCOUNT.md)
- **Exoscale** — [Set up an Exoscale account for Exasol Personal](./HOWTO_SETUP_EXOSCALE_ACCOUNT.md)
- **STACKIT** — [Set up a STACKIT account for Exasol Personal](./HOWTO_SETUP_STACKIT_ACCOUNT.md)
- **Local** — local deployment on macOS Apple Silicon only


## 🏎️ Quick Start (macOS / Linux)


1. Download the launcher
```bash
curl https://downloads.exasol.com/exasol-personal/installer.sh | sh
```

2. Install on a cloud provider or your local system

```bash
exasol install aws        # Amazon Web Services
```

```bash
exasol install azure      # Microsoft Azure
```

```bash
exasol install exoscale   # Exoscale
```

```bash
exasol install stackit    # STACKIT
````

```bash
exasol install local      # local system, macOS Apple Silicon only
```

Read on for Windows instructions and full details.


## 🚀 Deploy Exasol Personal

1. Download and install the Exasol Launcher for your platform.

   On Linux and macOS, run:
   ```bash
   curl https://downloads.exasol.com/exasol-personal/installer.sh | sh
   ```

   This installs the `exasol` binary to `~/.local/bin`. On most Linux distributions this directory is already in your `PATH`. On macOS, or if the installer reports that `~/.local/bin` is not in your `PATH`, follow its instructions.

   On Windows: download the Exasol Launcher from the [Exasol Download Portal](https://downloads.exasol.com/exasol-personal) and copy the `exasol` binary to a directory in your `PATH`.

2. For cloud presets, configure authentication for your provider. See the relevant account setup guide in [Prerequisites](#-prerequisites) for the environment variables and credentials required.

3. To install Exasol Personal, run the following command with the preset for your cloud provider or local system:
   ```bash
   exasol install aws        # Amazon Web Services
   exasol install azure      # Microsoft Azure
   exasol install exoscale   # Exoscale
   exasol install stackit    # STACKIT
   exasol install local      # local system, macOS Apple Silicon only
   ```
   The `exasol install` command does the following:
   - Generates backend files in the deployment directory
   - Provisions the necessary deployment resources
   - Starts the deployment
   - Downloads and installs Exasol Personal on that infrastructure

   The whole process normally takes about 10 to 20 minutes to complete.

   When the deployment process has finished, you will see instructions on how to connect to your Exasol database using a client of your choice. You can also find this information at any time by using `exasol info` in the terminal.

By default, Exasol Personal stores deployment state in `~/.exasol/personal/deployments/default`. If you run a command from an existing deployment directory, Exasol Personal uses that directory instead. Pass `--deployment-dir <path>` to choose a different deployment directory explicitly.

Keep the deployment directory until deployment resources have been destroyed. Deleting the directory does not remove those resources and can make cleanup harder.

An initialized deployment directory is tied to the selected infrastructure and installation presets. Rerun `exasol install <preset>` with the same presets to retry a failed deployment safely, or use `exasol config get`, `exasol config set`, and `exasol config reset` to inspect or change parameters for the existing presets without deleting local state. To switch presets in the same deployment directory, run `exasol destroy --remove` before initializing again, or run `exasol remove` if the deployment resources are already gone.

Runtime tools such as OpenTofu are downloaded on demand and reused from a per-user runtime artifact cache. Use `exasol cache list` to inspect cached artifacts, `exasol cache clean` to remove stale artifacts, `exasol cache clean --invalid` to remove artifacts that fail integrity checks, `exasol cache clean --partial-downloads` to remove staged partial downloads, `exasol cache clean --all` to wipe cached artifacts, and `exasol diag cache` to inspect cache health without changing it. Add `--dry-run` to a cleanup command to preview what would be removed.

## 📊 Load Sample Data

To get started quickly, Exasol provides two sample datasets hosted on S3 that you can import directly using SQL.

You can load it directly by executing this command:

```bash
exasol connect < sample.sql
```

Alternatively, connect with a SQL client of your choice and paste the statements below:

```sql
CREATE OR REPLACE TABLE PRODUCTS (
    PRODUCT_ID        DECIMAL(18,0),
    PRODUCT_CATEGORY  VARCHAR(100),
    PRODUCT_NAME      VARCHAR(2000000),
    PRICE_USD         DOUBLE,
    INVENTORY_COUNT   DECIMAL(10,0),
    MARGIN            DOUBLE,
    DISTRIBUTE BY PRODUCT_ID);

IMPORT INTO PRODUCTS
FROM PARQUET AT 'https://exasol-easy-data-access.s3.eu-central-1.amazonaws.com/sample-data/'
FILE 'online_products.parquet';
```

```sql
CREATE OR REPLACE TABLE PRODUCT_REVIEWS (
    REVIEW_ID          DECIMAL(18,0),
    PRODUCT_ID         DECIMAL(18,0),
    PRODUCT_NAME       VARCHAR(2000000),
    PRODUCT_CATEGORY   VARCHAR(100),
    RATING             DECIMAL(2,0),
    REVIEW_TEXT        VARCHAR(100000),
    REVIEWER_NAME      VARCHAR(200),
    REVIEWER_PERSONA   VARCHAR(100),
    REVIEWER_AGE       DECIMAL(3,0),
    REVIEWER_LOCATION  VARCHAR(200),
    REVIEW_DATE        VARCHAR(200),
    DISTRIBUTE BY PRODUCT_ID);

IMPORT INTO PRODUCT_REVIEWS
FROM PARQUET AT 'https://exasol-easy-data-access.s3.eu-central-1.amazonaws.com/sample-data/'
FILE 'product_reviews.parquet';
```

| Table | Rows | Size |
|---|---|---|
| `PRODUCTS` | 1,000,000 | 27.3 MiB |
| `PRODUCT_REVIEWS` | 1,822,007 | 154.5 MiB |

Both tables are distributed by `PRODUCT_ID`, enabling efficient joins between them.

## ⏯️ Start and stop Exasol Personal

To save costs, you can temporarily stop Exasol Personal by using the following command:
```bash
exasol stop
```
This stops the compute instance(s) that Exasol Personal is running on.

Networking and data volumes that the database data is stored on will continue to incur costs when compute instances are stopped.

To start Exasol Personal again, use the following command:
```bash
exasol start
```
The IP addresses of the nodes will change when you restart Exasol Personal. Check the output of the `start` command to know how to connect to the deployment after a restart.

For local deployments, which currently require macOS Apple Silicon, the launcher manages a local VM runtime and an internal deployment share inside the deployment directory. The initial local database credentials are `sys` / `exasol`. `exasol shell host` opens the local VM shell, and `exasol shell container` opens a shell inside the local database container.

## 🗑️ Remove Exasol Personal

To completely remove an Exasol Personal deployment, use `exasol destroy`. This command deletes the deployment resources and all associated data.

To learn more about this command, use `exasol destroy --help`.

By default, `exasol destroy` keeps the local deployment files so you can inspect the deployment or recreate the same preset. To also remove the local deployment directory after deployment resources have been destroyed, run:
```bash
exasol destroy --remove
```

If deployment resources were already deleted manually or you no longer have access to destroy them through the launcher, use the local recovery command:
```bash
exasol remove
```
This removes the local deployment directory. It does not destroy deployment resources.

Deleting the deployment directory and the Exasol Launcher will not remove the resources that were created in the target environment. To completely remove a deployment, you must use the `exasol destroy` command before deleting the deployment directory.

If you have already deleted the deployment directory and the Exasol Launcher, you must remove the resources manually in the target environment.

For local deployments, `exasol destroy` deletes the local VM disk/data and launcher-managed share for that deployment.

## ⚙️ Cloud: Choosing cluster size and compute instance types

By default the launcher deploys a single-node cluster on a memory-optimized instance in the cloud (e.g. `r6i.xlarge` on AWS, `Standard_E4s_v3` on Azure, `standard.extra-large` on Exoscale, `m2i.4` on STACKIT). To change the number of nodes or the instance type, use the `--cluster-size` and `--instance-type` options:
```bash
exasol install <preset> --cluster-size <number> --instance-type <string>
```
If the deployment process is interrupted, resources that were already created will not be removed automatically and cloud resources may continue to accrue cost. In that case, use `exasol destroy` to clean up the deployment, or remove the resources manually in the target environment.


## 🔜 Next steps

Once the deployment process is complete, use `exasol info` for information about how to connect to your Exasol database. The credentials for connecting to the database from a client are stored in the file `secrets.json` in the deployment directory.

You can also use the built-in SQL client in Exasol Launcher to connect directly to the database from the command line:
```bash
exasol connect
```

To run SQL without entering the interactive shell, pass it directly. Both flags accept multiple `;`-separated statements and exit when done, which is convenient for scripting and automation:
```bash
exasol connect -c "SELECT 1; SELECT 2"   # run inline statement(s)
exasol connect -f script.sql             # run statements from a file
```
`--command` and `--file` are mutually exclusive. In this non-interactive mode, execution stops at the first failing statement and `connect` exits with a non-zero status so scripts can detect errors. Combine with `--json` for machine-readable output.

In an interactive session, query output is capped at 100 rows by default so a large `SELECT` doesn't flood the terminal; a note is printed when output is truncated. Piped or `--command`/`--file` (non-interactive) execution returns the full result set. Use `--max-rows N` to set the cap explicitly, or `--max-rows 0` for unlimited:
```bash
exasol connect --max-rows 0        # return all rows, even interactively
echo "SELECT * FROM PRODUCTS;" | exasol connect --max-rows 1000
```

See also...
- To learn more about how you can connect to your Exasol database and start loading data using the many supported tools and integrations, see [Connect to Exasol](https://docs.exasol.com/db/latest/connect_exasol.htm) and [Load Data](https://docs.exasol.com/db/latest/loading_data.htm).
- To learn how to use the SQL statements, data types, functions, and other SQL language elements that are supported in Exasol, see [SQL reference](https://docs.exasol.com/db/latest/sql_reference.htm).

## 🧑‍💻 Exasol Admin

Exasol Admin is an easy-to-use web interface that you can use to administer your new Exasol database. Instructions for how to access Exasol Admin is shown in the terminal output at the end of the install process.

- To find the Exasol Admin URL after the installation has completed, use `exasol info`.
- The credentials for connecting to Exasol Admin are stored in the file `secrets.json` in the deployment directory.

Your browser may show a security warning when connecting to Exasol Admin because of the self-signed certificate. Accept this warning and continue.

Currently, Exasol Admin is only available on cloud deployments.

## 🔒 Connect using SSH

To connect with SSH to your deployment use one of the following commands:

```bash
# Connect to the compute instance your database is running on:
exasol shell host
```

```bash
# Connect to the COS container your node is running on:
exasol shell container
```

## 📦 Presets

Exasol Personal uses **presets** — self-contained directories of templates and config files — to provision infrastructure and install Exasol. Each deployment combines two presets:

- **Infrastructure preset** — provisions deployment resources for a cloud provider or local system. Built-in: `aws`, `azure`, `exoscale`, `stackit`, `local`.
- **Installation preset** — installs and configures Exasol on the provisioned nodes. Built-in: `ubuntu` (used by default).

```bash
exasol install <infra-preset> [install-preset]

exasol install aws            # built-in preset by name
exasol install ./my-preset    # local preset by path (starts with . / ~ or contains /)
exasol install ./my-infra ./my-install   # both presets from local paths
```

List all available built-in presets, including cloud and local targets:

```bash
exasol presets
```

### Local presets

You can store your own preset directories anywhere on your filesystem and pass the path directly to `exasol install`. This lets you target additional infrastructure platforms or customize provisioning without modifying the launcher.

### Building your own preset

See [doc/presets.md](doc/presets.md) for the full preset contract: manifest schema, required output artifacts, variable channels, and the reference implementations in `assets/infrastructure`.

## ⚖️ Licensing

The Exasol Launcher source code in this repository is open-source software licensed under the [MIT License](./LICENSE). You are free to use, modify, and distribute it. Contributions are made under the same terms.

The launcher installs the **Exasol Database**, which is proprietary software provided by Exasol AG, free for personal use. By deploying it with `exasol install`, you accept the [Exasol Personal End User License Agreement (EULA)](https://www.exasol.com/terms-and-conditions/#h-exasol-personal-end-user-license-agreement).

## 📚 Resources & Documentation

Get the most out of Exasol with these resources:

- [Exasol Documentation](https://docs.exasol.com/db/latest/home.htm) — Complete database documentation
- [Connect to Exasol](https://docs.exasol.com/db/latest/connect_exasol.htm) — Driver downloads and client setup
- [Load Data](https://docs.exasol.com/db/latest/loading_data.htm) — Import data into your database
- [SQL Reference](https://docs.exasol.com/db/latest/sql_reference.htm) — Complete SQL syntax reference
- [Exasol Community](https://community.exasol.com) — Ask questions and share knowledge (use tag `exasol-personal`)
