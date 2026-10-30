# Maintenance

This guide is the entry point for NAuth maintainers. When submitting changes, follow
[CONTRIBUTING.md](./CONTRIBUTING.md), including its commit, release-impact, and documentation conventions.

## Related guides

- [Issue triage](./docs/triage.md): assess community issues and apply triage labels.
- [CRDs-catalog](./docs/crds-catalog.md): publish changed CRD schemas to the external catalog.
- [Operator bootstrap](./operator-bootstrap/README.md): provision NATS operator credentials and rotate keys.

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

The `Release Please` workflow uses the repository's `GITHUB_TOKEN` and watches `main` for stable releases and
`release/*` branches for release candidates.
After Release Please creates the GitHub Release and tag, it calls the `Operator Release` workflow.
That workflow builds the operator image and both Helm charts from the release tag.
The leading `v` is stripped before publishing artifacts.

### Stable release checklist

Before merging a stable release PR:

- Review the proposed version, release notes, and migration guidance for breaking changes or deprecations.
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

Create a branch from `main` named `release/X.Y.Z`, for example `release/0.7.9`.
Release Please uses its prerelease configuration on that branch and creates release PRs with SemVer prerelease
versions such as `v0.7.9-rc.1`. Merge each release PR manually to create its GitHub prerelease and publish artifacts.
To request a specific version, add a `Release-As: X.Y.Z-rc.N` footer to a commit on the release branch.

Release tags use these formats:

- Stable: `vX.Y.Z`, for example `v0.7.9`.
- Release candidate: `vX.Y.Z-rc.N`, for example `v0.7.9-rc.1`.

Do not use `v0.7.9-rc-1`; use `v0.7.9-rc.1`.
Install an RC explicitly:

```sh
helm upgrade --install nauth oci://ghcr.io/wirelesscar/nauth \
  --namespace nauth \
  --create-namespace \
  --version 0.7.9-rc.1
```

To promote a tested RC line to stable, merge its release branch into `main`.
Then review and manually merge the stable release PR that Release Please creates.
Do not create tags or GitHub Releases by hand.
