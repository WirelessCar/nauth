# Updating CRDs-catalog

NAuth CRDs should be published to the community CRD schema catalog at <https://github.com/datreeio/CRDs-catalog> when a release changes the CRD schema. The catalog is maintained in a separate repository, so generated schema files are committed to a PR there, not to this repository.

## Prerequisites

Fork `datreeio/CRDs-catalog` before editing it. With the GitHub CLI, create the fork and clone your fork outside this repository:

```bash
gh repo fork datreeio/CRDs-catalog --clone=false
gh repo clone CRDs-catalog ../CRDs-catalog
```

If you do not use the GitHub CLI, fork <https://github.com/datreeio/CRDs-catalog> in GitHub and clone your fork into `../CRDs-catalog`.

Install this repository's toolchain with mise:

```bash
mise install
```

The catalog update task uses the `python` and `uv` versions from `mise.toml`. Python package requirements are isolated with `uv run --no-project --with pyyaml==6.0.2`, so you do not need to install `pyyaml` globally or create a repository venv.

## Update The Catalog

Regenerate NAuth CRDs before exporting schemas if the release changes API types or controller-gen markers:

```bash
make manifests
```

Run the catalog update task with the local catalog checkout path:

```bash
mise nauth:update-crds-catalog ../CRDs-catalog
```

The task converts `charts/nauth-crds/crds/*.yaml` with the catalog's `Utilities/openapi2jsonschema.py` and replaces `../CRDs-catalog/nauth.io/*.json`.

Inspect the external catalog diff:

```bash
git -C ../CRDs-catalog diff -- nauth.io
```

The expected schema files are:

- `nauth.io/account_v1alpha1.json`
- `nauth.io/accountexport_v1alpha1.json`
- `nauth.io/accountimport_v1alpha1.json`
- `nauth.io/natscluster_v1alpha1.json`
- `nauth.io/user_v1alpha1.json`

Commit those changes in the CRDs-catalog checkout and open a pull request against `datreeio/CRDs-catalog`.

## Troubleshooting

If the task reports a missing catalog converter, update the local CRDs-catalog checkout from upstream and try again.

If `uv` needs to download `pyyaml` on the first run, allow network access. The package is cached by `uv` and is not installed into global Python site-packages.
