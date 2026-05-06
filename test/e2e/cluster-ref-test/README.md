# cluster-ref-test

This KUTTL suite verifies the `natsClusterRef` flow using a dedicated namespace and secrets that do not use legacy `nauth.io/secret-type` labels.

It creates:

- the namespace `cluster-ref-test`
- `nats-url-config` in `cluster-ref-test`
- `my-operator-signing-key` in `cluster-ref-test`
- `my-system-account-creds` in `cluster-ref-test`
- `test-cluster` as a `NatsCluster` in `cluster-ref-test`
- `cluster-ref-account` in `cluster-ref-test`
- `cluster-ref-user` in `cluster-ref-test`

The assertions verify that:

- the setup secrets and ConfigMap exist without relying on legacy secret labels
- `test-cluster` reconciles successfully and loads its URL and secret references from the dedicated namespace
- `cluster-ref-account` reconciles successfully through `test-cluster`
- `cluster-ref-account` gets generated account root and signing secrets in `cluster-ref-test`
- no legacy operator bootstrap secret copies are created in `cluster-ref-test`
- `cluster-ref-user` reconciles successfully and receives a generated user credentials secret
