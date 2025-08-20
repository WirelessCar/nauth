---
title: Getting Started
description: Get started with NAuth
---

A Kubernetes operator for managing [decentralized authentication & authorization for NATS](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt)

NAuth allows platform teams to provide easy multi-tenancy support for development teams by providing `Account` & `User` CRD:s that conveniently packages:

- Account creation & updates
- Account exports & imports
- User creation & credentials delivery

## Installation
NAuth supports installation through packaged [Helm](https://helm.sh) charts.

```
helm install nauth oci://ghcr.io/wirelesscar/nauth --create-namespace --namespace nauth
```

### Pre-requisites
NAuth requires [NATS](https://nats.io) to be installed in the cluster, since NAuth integrates with NATS (over NATS) to provide the account JWT:s.
See examples of how to setup NATS with JWT auth together with NAuth in the [examples](https://github.com/WirelessCar/nauth/tree/main/examples) directory.


## Getting Started
Running a large NATS cluster requires that the operator is secured properly. If you do not already have an operator, try
out:
- the [operator-bootstrap](https://github.com/WirelessCar/nauth/tree/main/operator-bootstrap) utility which comes with NAuth to create your own operator
- check out our [examples](https://github.com/WirelessCar/nauth/tree/main/examples) which contains a static operator and
  corresponding NATS configuration

You can also use [`nsc`](https://github.com/nats-io/nsc) directly to create a throw-away operator & system account.

## More on decentralized JWT Auth
It is recommended to have an understanding of how [decentralized authentication & authorization for
NATS](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt) works before using NAuth.
However, setting up your local NAuth & NATS bundle to experiment is also a good way to learn! Check out our
[CONTRIBUTING guide](https://github.com/WirelessCar/nauth/blob/main/CONTRIBUTING.md), which describes how to setup a
local testing environment with a bundled NATS cluster & configuration.

Check out this video for a comprehensive description on how decentralized JWT Auth works. In order to work with NAuth,
it's important to have an understanding of how the basics work.

[![NATS decentralized JWT Auth](https://i3.ytimg.com/vi/5pQVjN0ym5w/hqdefault.jpg)](https://youtu.be/5pQVjN0ym5w)

