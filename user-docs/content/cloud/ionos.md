# Set up IONOS Cloud

Prepare an IONOS Cloud contract and API token before deploying Exasol Personal.

## Prerequisites

You need an active IONOS Cloud contract with billing configured and a user who may reserve IP
blocks, create internet access, and either edit a shared data center or create data centers. The
user does not need to be an administrator.
See the [IONOS Cloud documentation](https://docs.ionos.com/cloud) for background information.

Each node is a dedicated core server with its own reserved public IP. With the default settings, a
node uses 4 cores, 16 GB RAM, 200 GB of SSD storage, and one IP. Check that your contract's
resource limits cover the cluster you plan to deploy: cores and RAM per server, cores, RAM, and
storage in total, and reservable IPs. The **Resource Overview** in the DCD shows them. Ask IONOS
support to raise limits that are too low.

## Configure access

Skip this section if you are a contract administrator. Otherwise, an administrator grants access
through a group in the Data Center Designer (DCD), under **Management** > **Users & Groups**.

### Share one data center

Recommended when several people in an organization use Exasol Personal, so that each deployment
does not create its own data center:

1. Create a data center for Exasol Personal deployments in the location you want to use. The DCD
   provisions a data center only when it contains at least one element, so add a LAN before you
   select **Provision Changes**. Deployments create their own LANs and leave this one untouched.
2. Create a group. On its **Privileges** tab, enable **Reserve IP** and **Create Internet Access**.
3. On the group's **Resources** tab, give the group **Edit** access to the data center.
4. On the group's **Members** tab, add the users.
5. Give the users the data center ID, which is the **UUID** on the data center's **View** tab. They
   pass it with `--datacenter-id` when installing a new deployment, as described in
   [Install](#install). The deployment keeps the ID, so to retry a failed installation, run
   `exasol install ionos` without options.

Members of this group cannot create data centers, so `exasol install ionos` without
`--datacenter-id` fails with `403 Forbidden` and the message that `CREATE_DATA_CENTER_PRIVILEGE` is
required. Every member can change all resources in the
shared data center, including other members' deployments.

### Create a data center per deployment

For a single user, each deployment can create and remove its own data center:

1. Create a group. On its **Privileges** tab, enable **Create Data Center**, **Reserve IP**, and
   **Create Internet Access**.
2. On the group's **Members** tab, add the user.

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

The default is a single node with 4 dedicated cores and 16 GB RAM in `de/fra`, with
100 GB SSD Premium volumes for the operating system and the database:

```bash
exasol install ionos
```

Servers are sized as `<cores>x<ram_gb>` with `--instance-type`. Use `--cluster-size` for more
nodes:

```bash
exasol install ionos --instance-type 4x16 --data-volume-size 200
exasol install ionos --cluster-size 2 --instance-type 4x16 --data-volume-size 200
```

Multi-node deployments are not yet verified on IONOS Cloud.

Use `--location` and `--cpu-family` to select another location and CPU family, and `--disk-type`
for `SSD Premium`, `SSD Standard`, or `HDD` volumes:

```bash
exasol install ionos --location de/txl --cpu-family INTEL_ICELAKE --disk-type "SSD Standard"
```

Without `--cpu-family`, the launcher uses the first CPU family that the data center offers. The
launcher checks the instance type against the limits of the
location and CPU family before creating servers. Contract limits are enforced by IONOS during
provisioning.

By default each deployment creates its own data center, which `exasol destroy` removes. To deploy
into an existing data center, such as one shared by your organization, pass its ID. The location
of that data center is used, and `exasol destroy` leaves the data center itself in place:

```bash
exasol install ionos --datacenter-id "<your-datacenter-uuid>"
```

Run `exasol install ionos --help` for all options.

Reserved IPs and volumes remain billable while a deployment is stopped. See the
[IONOS Cloud price list](https://cloud.ionos.com/prices) for current prices.
