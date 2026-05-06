# basic-test

This KUTTL suite verifies the basic happy path for operator bootstrap resources, `example-account`, and `example-user`.

It verifies existing bootstrap resources:

- `operator-op-sign` in the `nats` namespace
- `operator-sau-creds` in the `nats` namespace
- `local-nats` as the default `NatsCluster`

It creates:

- `example-account` in the test namespace
- `example-user` in the test namespace, pointing at `example-account`

The assertions verify that:

- the operator bootstrap secrets and `local-nats` exist and reference each other correctly
- `example-account` reconciles successfully and receives an account ID and signing key
- the generated account root and signing secrets exist for `example-account`
- `example-user` reconciles successfully and is linked to `example-account`
- the generated user credentials secret exists and is owned by `example-user`
