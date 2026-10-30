# Example setup

This directory provides sample NAuth resources and scenarios for local testing.
They are designed to work with the `mise` tasks (scripts live under
`.mise-tasks/nauth`).

## Quick start

First install the [local NATS and NAuth stack](../local/README.md#local-dev-stack), then apply the example scenarios:

```sh
mise nauth:install-examples
```

If you prefer to run it manually:

```sh
kubectl apply -f examples/nauth/manifests/scenarios/ --recursive
```

## Publish and subscribe

The simple-account scenario creates `example-account` and `example-user` in the `simple-account` namespace.
The publish and subscribe tasks read credentials from the current context's namespace.
Select that namespace and wait for the User to become ready:

```sh
kubectl config set-context --current --namespace=simple-account
kubectl wait --for=condition=Ready user/example-user --timeout=120s
```

Run these commands from the repository root in three separate terminals:

```sh
# Terminal 1: forward the NATS client port.
mise nats:port-forward
```

```sh
# Terminal 2: subscribe using the example User's credentials.
mise nats:sub -- example-user 'foo.>'
```

```sh
# Terminal 3: publish a message.
mise nats:pub -- example-user foo.test 'hello there'
```

## Notes

The manifests include example secrets for local development. Do not use these
in production. Instead, follow the
[operator-bootstrap](../operator-bootstrap/README.md) instructions to generate
your own credentials.
