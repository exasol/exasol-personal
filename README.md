<div align="center">

<picture>
  <source srcset="static/Exasol_Logo_2025_Bright.svg" media="(prefers-color-scheme: dark)">
  <img src="static/Exasol_Logo_2025_Dark.svg" alt="Exasol Logo" width="300">
</picture>

# Exasol Personal

**The High-Performance Analytics Engine — Free for Personal Use**

*Deploy a full-scale Exasol cluster on your own AWS infrastructure in minutes*

[![Documentation](https://img.shields.io/badge/docs-exasol.com-blue)](https://docs.exasol.com/db/latest/home.htm)
[![Community](https://img.shields.io/badge/community-exasol-green)](https://community.exasol.com)
[![Downloads](https://img.shields.io/badge/downloads-exasol.com-orange)](https://downloads.exasol.com/exasol-personal)

</div>

## 🔥 Key Features

- 🏎️ **In-Memory Performance** — Run complex analytics at in-memory speed with Exasol's industry-leading analytics engine
- 🏢 **Full Enterprise Features** — Access all enterprise-scale capabilities of the Exasol database, completely free for personal use
- ♾️ **Unlimited Data** — Store and analyze unlimited amounts of data with no artificial limits
- 📈 **Scalable Architecture** — Scale up to any number of nodes using Exasol's MPP (Massively Parallel Processing) architecture
- 🤖 **Built-in AI Functions** — Leverage native AI/ML capabilities with GPU acceleration
- ⚙️ **Simple Deployment** — Spin up a distributed cluster on your own AWS infrastructure with just a few commands
- 🖥️ **Cross-Platform CLI** — Install and manage your cluster using the Exasol Launcher on Linux, macOS, or Windows


## ✅ Prerequisites

- An AWS account that can provision large type instances. Exasol launcher by default uses the r6i.xlarge EC2 instance type.
- An AWS user with enough permissions to create resources such as EC2 instances.

To learn how to set up an AWS account with an instance profile and configure your local environment to use that profile when installing Exasol Personal, see [Set up an AWS account for Exasol Personal](./HOWTO_SETUP_AWS_ACCOUNT.md).


## 🏎️ Quick Start for macOS 🍎 and Linux 🐧

Download the Exasol launcher:
```bash
curl https://downloads.exasol.com/exasol-personal/installer.sh | sh
```
Create a deployment directory:
```bash
mkdir deployment && cd deployment
```
Install Exasol Personal:
```bash
../exasol install aws
```
Read on for how to install on Windows and for more detailed instructions.


## 🚀 Deploy Exasol Personal

1. Download Exasol Launcher for your platform.

   On Linux and macOS, run:
   ```bash
   curl https://downloads.exasol.com/exasol-personal/installer.sh | sh
   ```

   On all platforms including Windows:
   Download Exasol Launcher from the [Exasol Download Portal](https://downloads.exasol.com/exasol-personal).

   Copy the `exasol` binary into your PATH.


2. Create a new directory “deployment” and change into the directory:
   ```bash
   mkdir deployment
   cd deployment
   ```
3. Configure the `AWS_PROFILE` environment variable to use the instance profile that you created in your AWS account for Exasol Personal.

   If the profile name is `exasol`:
   ```bash
   # Linux / macOS (Bash)
   export AWS_PROFILE=exasol
   ```
   ```powershell
   # Windows (PowerShell)
   $env:AWS_PROFILE = "exasol"
   ```
      ```powershell
   # Windows (cmd)
   set AWS_PROFILE=exasol
   ```

4. To install Exasol Personal, run the following command:
   ```bash
   exasol install aws
   ```
   The `exasol install` command does the following:
   - Generates Terraform files in the deployment directory
   - Provisions the necessary AWS infrastructure with Terraform
   - Starts up the AWS infrastructure
   - Downloads and installs Exasol Personal on that infrastructure

   The whole process normally takes about 10 to 20 minutes to complete.

   When the deployment process has finished, you will see instructions on how to connect to your Exasol database using a client of your choice. You can also find this information at any time by using `exasol info` in the terminal.

Most `exasol` commands must be run from the context of the deployment directory. Change into the deployment directory before you run any `exasol` commands, and prepend the command with the relative path to the binary. For example: `../exasol <command>`.

To avoid having to prepend all exasol commands with the path to the binary, you can add the path to your PATH environment variable. For more information about how to set environment variables, refer to the documentation for your operating system.

## 📊 Load Sample Data

To get started quickly, Exasol provides two sample datasets hosted on S3 that you can import directly using SQL.

Connect to your database (e.g. via `exasol connect` or any SQL client) and run:

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


## ⚙️ Choosing cluster size and instance types

The launcher will by default generate Terraform files to install one Exasol node on one Amazon EC2 instance of the type r6i.xlarge. To change the number of nodes and the EC2 instance type to use in the deployment, use the `--cluster-size` and `--instance-type` options:
```bash
exasol install aws --cluster-size <number> --instance-type <string>
```
If the deployment process is interrupted, EC2 instances that were created by the process will not be terminated and may therefore continue to accrue cost. In case of an aborted deployment you must log in to the AWS console and manually terminate those instances.

## ⏯️ Start and stop Exasol Personal

To save costs, you can temporarily stop Exasol Personal by using the following command (in the deployment directory):
```bash
exasol stop
```
This stops the EC2 instance(s) that Exasol Personal is running on.

Networking and data volumes that the database data is stored on will continue to incur costs when instances are stopped.

To start Exasol Personal again, use the following command:
```bash
exasol start
```
The IP addresses of the nodes will change when you restart Exasol Personal. Check the output of the `start` command to know how to connect to the deployment after a restart.

## 🗑️ Remove Exasol Personal

To completely remove an Exasol Personal deployment, use `exasol destroy`. This command will terminate the EC2 instance and delete it and all associated resources in AWS.

To learn more about this command, use `exasol destroy --help`.

Deleting the deployment directory and the Exasol Launcher will not remove the resources that were created in your AWS account. To completely remove a deployment, you must use the `exasol destroy` command.

If you have already deleted the deployment directory and the exasol binary, you must log in to the AWS console and manually terminate the EC2 instances and associated resources.

## 🔜 Next steps

Once the deployment process is complete, use `exasol info` for information about how to connect to your Exasol database. The credentials for connecting to the database from a client are stored in the file `secrets.json` in the deployment directory.

You can also use the built-in SQL client in Exasol Launcher to connect directly to the database from the command line:
```bash
../exasol connect
```
See also...
- To learn more about how you can connect to your Exasol database and start loading data using the many supported tools and integrations, see [Connect to Exasol](https://docs.exasol.com/db/latest/connect_exasol.htm) and [Load Data](https://docs.exasol.com/db/latest/loading_data.htm).
- To learn how to use the SQL statements, data types, functions, and other SQL language elements that are supported in Exasol, see [SQL reference](https://docs.exasol.com/db/latest/sql_reference.htm).

## 🧑‍💻 Exasol Admin

Exasol Admin is an easy-to-use web interface that you can use to administer your new Exasol database. Instructions for how to access Exasol Admin is shown in the terminal output at the end of the install process.

- To find the Exasol Admin URL after the installation has completed, use `exasol info`.
- The credentials for connecting to Exasol Admin are stored in the file `secrets.json` in the deployment directory.

Your browser may show a security warning when connecting to Exasol Admin because of the self-signed certificate. Accept this warning and continue.

## 🔒 Connect using SSH

To connect with SSH to the EC2 instance that your Exasol database is running on, use `exasol diag shell`.

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