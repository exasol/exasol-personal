# Limitations

Local deployments are intended for development and exploration. Cloud deployments support UDFs,
virtual schemas, Exasol Admin, and multiple database nodes; the differences below apply to local
deployments only.

## Available after a setup step

- **Script languages:** UDFs are supported, but no script language container is installed by
  default. Install the one you need with `exasol slc install <language>`. Cloud deployments include
  the standard containers. See [UDFs and script language containers](udfs.md).
- **Virtual schemas:** Supported once the adapter runtime and the adapter's dependencies are in
  place. JDBC adapters need a Java SLC and staged adapter and driver files. See
  [Virtual schemas](virtual-schemas.md).

## Not available

- **Administration UI:** Exasol Admin is available on cloud deployments only. See
  [Use Exasol Admin](connect.md#use-exasol-admin).
- **Multiple nodes:** A local deployment always runs as a single node.
- **Shell access:** `exasol shell host` and `exasol shell container` work for local deployments only
  on macOS.
