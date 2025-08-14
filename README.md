<p align="center">
    <img src="./assets/nauth.png" alt="NAUTH" width="280" height="200">
</p>

# NAUTH
A Kubernetes operator for managing [decentralized authentication & authorization for NATS](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt)

NAuth allows platform teams to provide easy multi-tenancy support for development teams by providing `Account` & `User` CRD:s that conveniently packages:

- Account creation & updates
- Account exports & imports
- User creation & credentials delivery

> [!WARNING]
> This project is in early development. There will likely be breaking changes before getting to a stable release.
> Instructions might be lacking, but will be built as we go.

### Installation
NAuth supports installation through packaged [Helm](https://helm.sh) charts.

## Getting Started
Running a large NATS cluster requires that the operator is secured properly. If you do not already have an operator, try
out the [operator-bootstrap](./operator-bootstrap) utility which comes with NAuth.

You can also use [`nsc`](https://github.com/nats-io/nsc) directly to create a throw-away operator & system account.

## Nauth Development
Check out the [CONTRIBUTING](./CONTRIBUTING.md) guide.
