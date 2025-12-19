# Example setup

This directory provides sample NAuth resources and scenarios for local testing.
They are designed to work with the `mise` tasks (scripts live under
`.mise-tasks/nauth`).

## Quick start

Apply the example scenarios to a local cluster:

```sh
mise run nauth:install-examples
```

If you prefer to run it manually:

```sh
kubectl apply -f examples/nauth/manifests/scenarios/ --recursive
```

## Notes

The manifests include example secrets for local development. Do not use these
in production. Instead, follow the
[operator-bootstrap](../operator-bootstrap/README.md) instructions to generate
your own credentials.
