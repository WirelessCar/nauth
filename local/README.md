# Local development & E2E test setup

This directory contains local-only Helm values and manifests used by the KUTTL
end-to-end suite.

## Prerequisites

- mise (installs toolchain via `mise install`)
- Docker (for building/loading images)
- kubectl (for interacting with the local cluster)
- make (for the `Makefile` targets)

## Quick start

From the repo root (uses `mise` tasks):

```sh
mise nauth:e2e-test
```

This runs `kubectl kuttl test` using `kuttl-test.yaml`, which creates a Kind
cluster (default name `kind`), installs the local NATS chart from `local/nats`,
builds and loads the controller image, and installs the `charts/nauth` chart
with overrides from `local/nauth`. The KUTTL run deletes the Kind cluster after
the tests finish.

## Local dev stack

To bring up a local Kind cluster with NATS and NAuth:

```sh
mise nauth:install
```

This task recreates the default Kind cluster and installs NATS, NAuth, and a local operator fixture.
Apply the [example scenarios](../examples/README.md) separately to create Accounts and Users.
The task scripts live under `.mise-tasks/nauth` and can be run individually.

## Local overrides

- `local/nats/values.yaml`: chart overrides for NATS in the test environment.
- `local/nauth/values.yaml`: chart overrides for NAuth in the test environment.
- `local/nauth/manifests/operator.yaml`: shared bootstrap manifests applied during setup, including `NatsCluster` (`local-nats`) and referenced secrets.
- `local/prometheus/values.yaml`: chart overrides for Prometheus (if used).

## Cleanup

```sh
kind delete cluster
```
