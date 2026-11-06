# Maintenance

This guide is the entry point for NAuth maintainers. When submitting changes, follow
[CONTRIBUTING.md](./CONTRIBUTING.md), including its commit, release-impact, and documentation conventions.

## Related guides

- [Issue triage](./docs/triage.md): assess community issues and apply triage labels.
- [CRDs-catalog](./docs/crds-catalog.md): publish changed CRD schemas to the external catalog.
- [Operator bootstrap](./operator-bootstrap/README.md): provision NATS operator credentials and rotate keys.
- [Upgrading NAuth](./www/src/content/docs/guides/upgrading.mdx): publish user-facing migration steps for breaking releases.

## Release classification

Release notes use GitHub's built-in generator. Each PR appears once.
The sections are Breaking Changes ⚠️, Deprecations, and Other Changes, in that order.
A PR carrying both release-impact labels appears under Breaking Changes.
Migration explanations belong in the linked PR or documentation.

The `Release Labels` workflow checks the PR title and every commit message for
[breaking-change declarations](./CONTRIBUTING.md#breaking-changes-and-deprecations).
It adds or removes only `breaking-change`; all other labels remain manually controlled.
Apply `deprecation` to PRs that deprecate supported behavior.

The labeler executes trusted base-branch code and reads PR metadata through the GitHub API.
Do not check out or run PR-head code in this privileged workflow.
The [action guide](./.github/actions/label-breaking-change/README.md) describes its input and tests.

Labeling failures appear in GitHub Actions. They do not block merges or release publication.
Maintainers review the labels and generated notes before merging each release PR.
For PRs merged before the labeler was installed, review and apply release-impact labels manually.
After correcting labels, rerun the `Release Please` workflow to refresh the release PR and changelog.

## Releases

Release Please creates a PR updating `CHANGELOG.md` and `.release-please-manifest.json`.
Release PRs are never auto-merged. A maintainer decides when to merge them and create a release.
Version bumps come from Conventional Commits; release notes list merged PRs using GitHub's built-in generator.

While the release line is below `1.0.0`, breaking changes bump the minor version and every other change bumps
only the patch version, including `feat`, `docs`, `chore`, `test`, and `ci` commits.
Release Please keeps releases on the `0.x` line. Breaking changes remain breaking even though they bump only the minor version.

The `Release Please` workflow runs on pushes to `main` and can also be started manually on `main` with
`workflow_dispatch`. Release Please PRs are titled `chore(release): release ${version}`.
The workflow uses the repository's `GITHUB_TOKEN`.
After Release Please creates the GitHub Release and tag, it calls the `Operator Release` workflow.
That workflow builds the operator image and both Helm charts from the release tag.
The leading `v` is stripped before publishing artifacts.

### Stable release checklist

Before merging a stable release PR:

- Review the proposed version, release notes, and migration guidance for breaking changes or deprecations.
- Update the [Upgrading NAuth guide](./www/src/content/docs/guides/upgrading.mdx) with breaking-release migration steps,
  including any required intermediate NAuth versions.
- Confirm that breaking-change labels are correct and apply deprecation labels where appropriate.
  Resolve labeling failures and refresh the release PR after label changes.
- If the release changes NAuth CRDs, regenerate them and update the external CRDs catalog:
  ```sh
  make manifests
  mise nauth:update-crds-catalog ../CRDs-catalog
  ```
  Follow [Updating CRDs-catalog](./docs/crds-catalog.md). Open the catalog PR from that checkout;
  do not commit its generated schema files to this repository.
- Replace relevant `Unreleased` documentation labels with `Since vX.Y.Z` according to the
  [version-label conventions](./CONTRIBUTING.md#version-labels).

Merge the release PR to create the GitHub Release and publish the operator image and both Helm charts.

### Release candidates

Create RCs manually from `main` while Release Please maintains the stable release PR:

1. On GitHub, draft a new release, create a tag such as `v0.8.0-rc.1` targeting `main`, and mark it as a prerelease.
   The tag identifies the exact commit to smoke-test. Generate or write the release notes and publish the prerelease.
2. Confirm the `Publish Prerelease` workflow publishes the operator image and both Helm charts, then smoke-test them.
3. If fixes are needed, merge them into `main` through normal PRs. Publish and verify another RC with the next tag,
   such as `v0.8.0-rc.2`.
4. Before the stable release, review changes on `main` since the verified RC and test another RC if needed. Confirm the
   stable Release Please PR proposes the intended version and includes all changes since the previous stable release.
   Complete the [stable release checklist](#stable-release-checklist), then merge the release PR.

The stable release rebuilds the image and charts from its own tag, so they are not necessarily byte-for-byte identical
to the RC artifacts. Stable tags use `vX.Y.Z`; RC tags use `vX.Y.Z-rc` or `vX.Y.Z-rc.N`. Use a `Release-As` commit footer
only when intentionally overriding the version Release Please calculates.

Install an RC explicitly:

```sh
helm upgrade --install nauth oci://ghcr.io/wirelesscar/nauth \
  --namespace nauth \
  --create-namespace \
  --version 0.8.0-rc.1
```
