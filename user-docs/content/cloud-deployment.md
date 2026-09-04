# Deploy to the cloud

Exasol Personal 2.2 can provision a database in your own AWS, Azure, Exoscale, or STACKIT account.
Cloud deployments are useful when you need more capacity, multiple nodes, or a shared database.

## Prepare your account

Follow the setup guide for your provider before starting the installation:

- [Amazon Web Services](cloud/aws.md)
- [Microsoft Azure](cloud/azure.md)
- [Exoscale](cloud/exoscale.md)
- [STACKIT](cloud/stackit.md)

Cloud resources incur charges in your account until you destroy them.

## Install the database

Run the command for your provider:

```bash
exasol install aws
exasol install azure --location westeurope
exasol install exoscale
exasol install stackit --project-id "<your-project-uuid>"
```

The launcher generates the deployment files, provisions the infrastructure, starts it, and installs
Exasol Personal. A cloud installation normally takes 10 to 20 minutes.

When installation finishes, the command prints connection instructions. Run `exasol info` later to
display them again.

## Choose the cluster size and compute type

The default is a single-node cluster on a memory-optimized compute instance. Default types include
`r6i.xlarge` on AWS, `Standard_E4s_v3` on Azure, `standard.extra-large` on Exoscale, and `m2i.4` on
STACKIT.

Use `--cluster-size` and `--instance-type` to override the defaults:

```bash
exasol install <provider> --cluster-size <number> --instance-type <type>
```

If an installation is interrupted, resources already created are not removed automatically. Follow
the cleanup guidance in [Troubleshooting](troubleshooting.md) to avoid ongoing charges.
