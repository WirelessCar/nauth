# Contributing

Contributions are welcome. Open an issue for bugs or ideas, and discuss larger changes before starting work.
Everyone participating in the project must follow the [Code of Conduct](./CODE_OF_CONDUCT.md).
This guide applies to everyone submitting changes, including maintainers.
For triage and release procedures, see [MAINTENANCE.md](./MAINTENANCE.md).

## Developer setup

Use `mise` to install the repository's toolchain:

```sh
mise install
```

NAuth uses Kubebuilder controller-gen conventions for API types and generated code.
The `Makefile` provides the common build and verification commands; `make help` lists them.
For a local Kind cluster with NATS and NAuth, follow the [local development guide](./local/README.md).
For sample resources and publishing/subscribing, see the [examples guide](./examples/README.md).

## Tests and generated files

Before submitting a change, run:

```sh
make fmt lint test
```

`make test` runs unit and integration tests and prepares the envtest binaries.
Add or update tests for changed behavior. For CRD changes, include regression coverage under `test/e2e`.
Run the KUTTL end-to-end suite with `make test-e2e`; it creates and deletes a Kind cluster.
See the [local development guide](./local/README.md) for prerequisites and setup.
For breaking-change labeler changes, also run the [action tests](./.github/actions/label-breaking-change/README.md#development).

When changing API types or controller-gen markers, regenerate the derived files with
`make generate manifests`. Avoid editing generated Go code or CRD YAML directly.

## Commits and pull requests

1. Fork the repository and create a branch for your change.
1. Keep changes focused, explain the problem being solved, and link the relevant issue in the PR.
1. Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages and the PR title:
   `<type>(<scope>): <description>` or `<type>: <description>`, for example `feat(api): add token validation`.
1. Sign every commit with a verified signature and include a DCO `Signed-off-by` trailer:
   ```sh
   git commit -S -s
   ```
   The trailer certifies compliance with the [Developer Certificate of Origin](https://developercertificate.org).
1. Push your branch and open a pull request. Draft PRs are welcome for early feedback.

### Breaking changes and deprecations

Mark breaking changes with `!` in the PR or commit header, or a `BREAKING CHANGE:` / `BREAKING-CHANGE:` commit footer.
The PR title and every commit message are checked to maintain the `breaking-change` label automatically.
Include migration guidance in the PR or documentation.
For deprecations, explain the replacement and ask a maintainer to apply the `deprecation` label.

## Documentation

When updating Helm values, follow the [helm-docs](https://github.com/norwoodj/helm-docs) format.
Regenerate Helm chart and CRD reference documentation with:

```sh
mise nauth:generate-docs
```

### Version labels

The documentation website follows `main`. Mark release-sensitive features with:

- `Unreleased` when documented before their release.
- `Since vX.Y.Z` when available from a published release.
