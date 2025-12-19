# Local development & E2E test setup

This directory contains local-only Helm values and manifests used by the KUTTL
end-to-end suite.

## Prerequisites

- mise (installs toolchain via `mise install`)
- Docker (for building/loading images)

## Quick start

From the repo root (uses `mise` tasks):

```sh
mise run nauth:e2e-test
```

This runs `kubectl kuttl test` using `kuttl-test.yaml`, which creates a Kind
cluster (default name `kind`), installs the local NATS chart from `local/nats`,
builds and loads the controller image, and installs the `charts/nauth` chart
with overrides from `local/nauth`. The KUTTL run deletes the Kind cluster after
the tests finish.

## Local dev stack

To bring up a local Kind cluster with NATS, NAUTH, and example scenarios:

```sh
mise run nauth:install
```

The task scripts live under `.mise-tasks/nauth` and can be run individually.

## Local overrides

- `local/nats/values.yaml`: NATS chart overrides for the test environment.
- `local/nauth/values.yaml`: Nauth chart overrides for the test environment.
- `local/nauth/manifests/operator.yaml`: extra manifests applied during setup. Do not modify as it is also used by KUTTL tests. 
- `local/prometheus/values.yaml`: Prometheus chart overrides (if used).

## Cleanup

```sh
kind delete kind
```
