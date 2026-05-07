# NAuth CRDs

This Helm chart contains only the Custom Resource Definitions (CRDs) for the NAuth operator.

It is intended to be used alongside the main [nauth chart](../nauth) with `crds.install=false`.

The chart uses Helm 3's native [`crds/`](./crds) folder to ensure CRDs are installed before any templates.
