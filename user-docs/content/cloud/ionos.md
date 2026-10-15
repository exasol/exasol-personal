# Set up IONOS Cloud

Prepare an IONOS Cloud contract and API token before deploying Exasol Personal.

## Prerequisites

You need an active IONOS Cloud contract with billing configured and a user who may create data
centers and reserve IP blocks.
See the [IONOS Cloud documentation](https://docs.ionos.com/cloud) for background information.

Each node is a dedicated core server with its own reserved public IP. Check that your contract's
resource limits cover the cluster you plan to deploy: cores and RAM per server, cores and RAM in
total, and reservable IPs. Run `ionosctl contract get` to show them, and ask IONOS support to raise
limits that are too low.

## Create an API token

In the Data Center Designer (DCD):

1. Open **Management** > **Token Manager**.
2. Generate a token and set its validity to cover the whole session, including `exasol destroy`.
3. Copy the token. It is shown only once.

The launcher authenticates with the token only. Username and password authentication is not
supported.

## Configure your computer

Set `IONOS_TOKEN` to the token.

On Linux or macOS:

```bash
export IONOS_TOKEN="<your-token>"
```

In Windows PowerShell:

```powershell
$env:IONOS_TOKEN = "<your-token>"
```

In Windows Command Prompt:

```batch
set IONOS_TOKEN=<your-token>
```

## Install

The default is a single node with 4 dedicated AMD EPYC cores and 16 GB RAM in `de/fra`, with
100 GB SSD Premium volumes for the operating system and the database:

```bash
exasol install ionos
```

Servers are sized as `<cores>x<ram_gb>` with `--instance-type`. Use `--cluster-size` for more
nodes:

```bash
exasol install ionos --instance-type 8x32 --data-volume-size 200
exasol install ionos --cluster-size 2 --instance-type 8x32 --data-volume-size 200
```

Use `--location` and `--cpu-family` to select another location and CPU family, and `--disk-type`
for `SSD Premium`, `SSD Standard`, or `HDD` volumes:

```bash
exasol install ionos --location de/txl --cpu-family INTEL_ICELAKE --disk-type "SSD Standard"
```

The launcher checks the instance type against the limits of the selected location and CPU family
before creating resources. Contract limits are enforced by IONOS during provisioning.

By default each deployment creates its own data center, which `exasol destroy` removes. To deploy
into an existing data center, pass its ID. The location of that data center is used, and
`exasol destroy` leaves the data center itself in place:

```bash
exasol install ionos --datacenter-id "<your-datacenter-uuid>"
```

Run `exasol install ionos --help` for all options.

Reserved IPs and volumes remain billable while a deployment is stopped. See the
[IONOS Cloud price list](https://cloud.ionos.com/prices) for current prices.
