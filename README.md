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

> [!IMPORTANT]
> Nauth requires the **system account user credentials** and the [**operator signing key nkey seed**](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/nkey_auth) to be provided as a k8s secret with the proper labels.
> - *nauth.io/secret-type: system-account-user-creds*
> - *nauth.io/secret-type: operator-sign*

You can see a full [operator example setup here](./examples/nauth/manifests/operator.yaml).

## Getting Started
Running a large NATS cluster requires that the operator is secured properly. If you do not already have an operator, try
out the [operator-bootstrap](./operator-bootstrap) utility which comes with NAuth.

You can also use [`nsc`](https://github.com/nats-io/nsc) directly to create a throw-away operator & system account.

## More on decentralized JWT Auth
Check out this video for a comprehensive description on how decentralized JWT Auth works. In order to work with NAuth,
it's important to have an understanding of how the basics work.

[![NATS decentralized JWT Auth](https://i3.ytimg.com/vi/5pQVjN0ym5w/hqdefault.jpg)](https://youtu.be/5pQVjN0ym5w)

## Import an existing NATS Account
Use this to "observe" an account that already exists in the NATS cluster. NAuth will fetch the JWT from NATS and update 
the account status, but it will never push a new JWT.

#### Required Secrets
 - Account root seed secret labeled: `account.nauth.io/id=$ACCOUNT_PUBKEY`, `nauth.io/secret-type=account-root` and 
`nauth.io/managed=true`. 
 - Account signing seed secret labeled: `account.nauth.io/id=$ACCOUNT_PUBKEY`, `nauth.io/secret-type=account-sign` 
and `nauth.io/managed=true`. 

#### Account CR example
```yaml
apiVersion: nauth.io/v1alpha1
kind: Account
metadata:
  name: my-acc
  labels:
    account.nauth.io/id: $ACCOUNT_PUBKEY
    nauth.io/management-policy: observe        # observe-only
```

Note: the System Account required by NAuth itself can only be observed and reconciliation of such an Account CRD without
`nauth.io/management-policy: observe` label will fail.

## Nauth Development
Check out the [CONTRIBUTING](./CONTRIBUTING.md) guide.

### Join our Slack
We co-locate with the NATS Slack in our [own channel](https://natsio.slack.com/archives/C0AFYH76KPD)
