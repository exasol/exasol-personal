# Limitations

Local deployments are intended for development and exploration. Release 2.2 has these differences
from cloud deployments:

- **Script languages:** UDFs are supported, but no script language container is installed by
  default. Install one with `exasol slc install <language>`. Cloud deployments include the standard
  containers.
- **Virtual schemas:** Virtual schemas are not available on local deployments.
- **Administration UI:** Exasol Admin is not available on local deployments.
- **Cluster size:** A local deployment always runs as a single node.
- **Host platforms:** A local deployment requires a supported Apple Silicon Mac. The launcher itself
  can run on the other supported platforms to manage cloud deployments.

Cloud deployments support UDFs, virtual schemas, Exasol Admin, and multiple database nodes.
