# Account Controller Integration Tests

This package is the middle ground between unit tests and KUTTL e2e tests.

It runs:
- `envtest` for Kubernetes
- embedded NATS
- real controller-runtime manager
- real `AccountReconciler`
- real `core` managers
- real outbound `k8s` and `nats` adapters

It does not use mocks.

## Running

Run the whole package:

```bash
go test ./test/integration/controller/account
```

Run a single scenario:

```bash
go test ./test/integration/controller/account -run TestAccountControllerIntegration/create-basic -count=1
```

That is the intended IDE workflow too.

## Scenario format

Scenarios live in `approvals/` as `*.input.yaml`.

Current top-level fields:

- `config`: per-scenario operator/controller config
- `resources`: Kubernetes objects to apply before waiting for reconciliation
- `collect`: Kubernetes objects to snapshot into the approval output

The approval output currently contains:

- normalized Kubernetes resources
- normalized NATS account claims uploaded for collected `Account` resources

## Intent

Keep these tests:
- blackbox
- YAML-first
- controller-specific
- approval-driven

Keep cross-controller/full-operator flows in `test/e2e`.
