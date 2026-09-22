# Cloud resources and costs

Cloud deployments create resources in your own provider account. The default is one node, with a
100 GB operating-system disk and a separate 100 GB database data disk. Increasing the cluster size
creates another compute instance, network interface or address where applicable, and both disks for
each additional node.

Every provider also gets a small bootstrap object-storage area used to deliver installation files.
A separate remote archive is optional but enabled by default; disable the provider's archive option
when you do not want it.

## Resources created by provider

### Amazon Web Services

The AWS preset creates a VPC, public subnet, internet gateway, route table, security group, S3
gateway endpoint, bootstrap S3 bucket, and an SSH key whose private part is stored in Parameter
Store. Each node has an EC2 instance with an EBS root volume, a separate EBS data volume, and an
automatically assigned public address.

When S3 archive support is enabled, it also creates an archive bucket and a scoped IAM role and
instance profile.

### Microsoft Azure

The Azure preset creates a resource group, virtual network, subnet, network security group,
bootstrap storage account and Blob container, and a Key Vault for the SSH private key. Each node has
a Linux virtual machine, network interface, static Standard public IP, managed OS disk, and separate
managed data disk.

When Blob archive support is enabled, it also creates a separate storage account and private Blob
container for the archive.

### Exoscale

The Exoscale preset creates a private network, security group, SSH key, and bootstrap Simple Object
Storage (SOS) bucket. Each node has a Compute instance with its root disk, an automatically assigned
public IP, and a separate Block Storage data volume.

When SOS archive support is enabled, it also creates an archive bucket and scoped IAM role and API
key.

### STACKIT

The STACKIT preset creates a private network, security group, SSH key, bootstrap Object Storage
bucket, and scoped bootstrap credentials. Each node has a server, network interface, public IP,
operating-system volume, and separate data volume.

When Object Storage archive support is enabled, it also creates a separate archive bucket and
scoped credentials.

## Stop or destroy a deployment

`exasol stop` preserves the deployment so that it can be started again. It stops or deallocates the
compute nodes, but retained storage and some networking resources can continue to incur charges:

- AWS continues to bill retained EBS volumes and may bill other retained resources; AWS documents
  the [costs associated with stopped instances](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-lifecycle.html#stopped-instances).
- Azure deallocates the virtual machines, while disks and networking can remain billable; see
  [Azure virtual-machine states and billing](https://learn.microsoft.com/en-us/azure/virtual-machines/states-billing).
- Exoscale continues to charge for volumes attached to powered-off instances and for stored SOS
  data; see [Exoscale billing](https://community.exoscale.com/platform/billing/).
- STACKIT storage is billed independently of a linked server's state, and public IP and Object
  Storage are separate billed services; see the
  [STACKIT price list](https://www.stackit.de/en/prices/).

Archive and bootstrap object storage persists while the deployment is stopped and can therefore
keep accruing storage charges. Public IP charges depend on the provider; the Azure and STACKIT
presets allocate dedicated public IP resources, while AWS and Exoscale use addresses assigned to
their compute instances.

Run `exasol destroy` when you no longer need the deployment. Destroy removes the launcher-managed
cloud resources and their data; stopping does not. Keep the deployment directory until destruction
has succeeded, as explained in [Manage deployments](manage-deployments.md#destroy-a-deployment).
