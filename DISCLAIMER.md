# Disclaimer

## Independent Project

This tool is an **independent personal project** created by kubanto.

It is **not** an official Veeam or Kasten product, is **not** affiliated with,
endorsed by, or supported by Veeam Software or any of its subsidiaries.

## No Warranty

This software is provided **"as is"**, without warranty of any kind, express or
implied, including but not limited to the warranties of merchantability, fitness
for a particular purpose, and non-infringement.

In no event shall the author be liable for any claim, damages, or other
liability — whether in contract, tort, or otherwise — arising from, out of, or
in connection with the software or the use or other dealings in the software.

## Use at Your Own Risk

- The author assumes **no responsibility** for any damage to systems, data loss,
  or security incidents resulting from the use of this tool.
- The tool operates in **read-only** mode (only `get` and `list` RBAC verbs are
  used), but it is the user's responsibility to review the RBAC manifest before
  applying it to any cluster.
- Always validate output against your actual cluster state before using it for
  operational decisions.

## No Maintenance Commitment

The author provides **no commitment** to maintain, update, fix bugs, or provide
support for this tool. Issues and pull requests are welcome but may not receive
a response.

## Data Privacy

This tool collects Kubernetes cluster metadata (versions, configuration,
namespace names, policy names) and writes it to local files only. It does not
transmit any data to external services. The user is solely responsible for
handling the generated reports in accordance with their organization's data
classification and privacy policies.
