# Limitations

Local deployments are intended for development and exploration. They have these differences from
cloud deployments:

- **Script languages:** UDFs are supported, but no script language container is installed by
  default. Install one with `exasol slc install <language>`. Cloud deployments include the standard
  containers.
- **Virtual schemas:** Virtual schemas require an adapter runtime and dependencies. JDBC adapters
  need a Java SLC and staged adapter and driver files. See [Virtual schemas](virtual-schemas.md).
- **Administration UI:** Exasol Admin is not available on local deployments.
- **Cluster size:** A local deployment always runs as a single node.
- **Shell access:** The runtime-managed `shell` commands for local deployments are available only on
  macOS.

Cloud deployments support UDFs, virtual schemas, Exasol Admin, and multiple database nodes.
