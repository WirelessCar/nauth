<p align="center">
    <img src="./www/public/nauth.svg" alt="NAUTH" width="280" height="200">
</p>

# NAUTH
A Kubernetes operator for managing [decentralized authentication & authorization for NATS](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt)

NAuth allows platform teams to provide easy multi-tenancy support for development teams by providing `Account` & `User` CRD:s that conveniently packages:

- Account creation & updates
- Account exports & imports
- User creation & credentials delivery

> [!NOTE]
> This project is still in early and active development. There will likely be breaking changes before getting to a stable release.

## Installation
NAuth supports installation through packaged [Helm](https://helm.sh) charts.

```
helm install nauth oci://ghcr.io/wirelesscar/nauth --create-namespace --namespace nauth
```

A [`nauth-crds`](./charts/nauth-crds) chart is also available for installing CRDs separately, which works 
alongside the main chart with `crds.install=false`.

### Pre-requisites
NAuth requires [NATS](https://nats.io) to be installed in the cluster, since NAuth integrates with NATS (over NATS) to provide the account JWT:s.
See examples of how to setup NATS with JWT auth together with NAuth in the [examples](./examples) directory.

NAuth requires the **system account user credentials** and the [**operator signing key nkey seed**](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/nkey_auth) to be provided as Kubernetes Secrets.

NAuth resolves these credentials through a `NatsCluster`. Choose one of these reference patterns:

**A.** For single-cluster deployments, set `NATS_CLUSTER_REF` on the NAuth controller (`namespace/name`, for example `nats/my-nats-cluster`) and define the secrets in that referenced `NatsCluster` (`spec.operatorSigningKeySecretRef` and `spec.systemAccountUserCredsSecretRef`).
   - Default behavior (`NATS_CLUSTER_REF_OPTIONAL=false`) is strict mode: account-level `spec.natsClusterRef` must match `NATS_CLUSTER_REF`.
   - `NATS_CLUSTER_REF_OPTIONAL=true` is explicit opt-in default mode: accounts without `spec.natsClusterRef` use `NATS_CLUSTER_REF`, while accounts may override with their own ref.
   - Recommended migration to per-account explicit refs:
     1) Set `NATS_CLUSTER_REF` with `NATS_CLUSTER_REF_OPTIONAL=false`.
     2) Add the same `spec.natsClusterRef` to all existing `Account` resources.
     3) Remove `NATS_CLUSTER_REF` and rely on explicit `spec.natsClusterRef` in each `Account`.

**B.** Define an explicit `spec.natsClusterRef` reference in each `Account` CR to a specific `NatsCluster`.

For an example that defines a `NatsCluster`, explicit `spec.natsClusterRef`, and the required credential Secret references, see the [cluster reference scenario](./examples/nauth/manifests/scenarios/cluster-ref/README.md).

## Getting Started
Running a large NATS cluster requires that the operator is secured properly. If you do not already have an operator, try
out the [operator-bootstrap](./operator-bootstrap) utility which comes with NAuth.

You can also use [`nsc`](https://github.com/nats-io/nsc) directly to create a throw-away operator & system account.

## More on decentralized JWT Auth
Check out this video for a comprehensive description on how decentralized JWT Auth works. In order to work with NAuth,
it's important to have an understanding of how the basics work.

[![NATS decentralized JWT Auth](https://i3.ytimg.com/vi/5pQVjN0ym5w/hqdefault.jpg)](https://youtu.be/5pQVjN0ym5w)

## Observe existing NATS accounts
NAuth can observe an existing NATS account without taking ownership of its JWT. Use this when migrating accounts into NAuth or recovering desired state from an existing cluster.

See the [observe existing accounts guide](https://nauth.io/guides/observe-existing-accounts/) for the required Secret labels and `Account` resource example.

## Nauth Development
Check out the [CONTRIBUTING](./CONTRIBUTING.md) guide.

### Join our Slack
We co-locate with the NATS Slack in our [own channel](https://natsio.slack.com/archives/C0AFYH76KPD)
