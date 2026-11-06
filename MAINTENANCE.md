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

The `Release Please` workflow runs on pushes to `main` and `release/*`, and can also be started manually with
`workflow_dispatch`. The selected branch is the target branch: `main` uses the stable config, while `release/*` uses the
prerelease config. Release Please PRs are titled `chore(release): release ${version}`.
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

#### Initial release candidate `vX.Y.Z-rc`

After the changes intended for the release are merged to `main`, create a branch from `main` named
`release/X.Y.Z`. Release Please uses its prerelease configuration on this branch and creates the initial candidate
without an ordinal, for example `v0.8.0-rc`. Review and merge the release PR to create the GitHub prerelease and publish
the operator image and Helm charts. Keep this branch for subsequent candidates; its manifest records the current RC
version. While validating the RC line, do not merge changes to `main`; target release fixes at `release/X.Y.Z` and keep
the stable Release Please PR unmerged.

#### Subsequent release candidates `vX.Y.Z-rc.N`

When an RC needs a fix, open a PR targeting `release/X.Y.Z`. Keep the fix's Conventional Commit type as `fix` so Release
Please recognizes it.

Each merged fix triggers Release Please on the release branch. It creates the next release PR, such as
`vX.Y.Z-rc.1`, then `vX.Y.Z-rc.2`; review and merge each PR to publish that candidate.

#### Final release `vX.Y.Z`

Once the latest RC is verified and approved, promote its release branch to `main` before merging the stable Release
Please PR:

1. Confirm `main` has not changed since `release/X.Y.Z` was created. If a change had to be merged to `main`, bring it
   into the RC line and publish and verify another candidate before promoting.
2. Open a promotion PR with base `main` and compare `release/X.Y.Z`. Review the diff to confirm it brings the verified
   RC changes and release state into `main`, then merge it.
3. The push to `main` triggers Release Please to create or update the stable release PR. Wait for that PR to reflect the
   promotion; rerun the `Release Please` workflow on `main` if it does not update. Review its proposed `X.Y.Z` version
   and release notes, complete the [stable release checklist](#stable-release-checklist), then merge it manually.
4. Confirm the stable GitHub release and operator image and Helm chart publishing workflows succeed. Then delete
   `release/X.Y.Z`; deleting the branch does not delete its GitHub releases or tags.

The stable release workflow rebuilds the operator image and Helm charts from the stable release tag. This carries the
approved source changes forward, but does not guarantee byte-for-byte identical artifacts to the RC. Do not create tags
or GitHub Releases by hand.

Release tags use `vX.Y.Z-rc` for the initial candidate, then `vX.Y.Z-rc.N` for subsequent candidates. Stable releases
use `vX.Y.Z`. Use a `Release-As` commit footer only when intentionally overriding the version Release Please calculates.

Install an RC explicitly:

```sh
helm upgrade --install nauth oci://ghcr.io/wirelesscar/nauth \
  --namespace nauth \
  --create-namespace \
  --version 0.8.0-rc
```
