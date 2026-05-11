# CONTRIBUTING
[fork]: /fork
[pr]: /compare
[code-of-conduct]: CODE_OF_CONDUCT.md

Thanks for contributing to NAuth.

> [!NOTE]
> The project is in an early phase. Contributing is still being streamlined.

Please note that this project is released with a [Contributor Code of Conduct][code-of-conduct]. By participating in this project you agree to abide by its terms.

## Issues and PRs

If you have suggestions for how this project could be improved, or want to report a bug, open an issue. Contributions and questions are welcome.

PRs are welcome too. If you're thinking of a large PR, open an issue first to talk about it. Look at the links below if you're not sure how to open a PR.

## Developer’s Certificate of Origin
The sign-off is a simple line at the end of the explanation for the commit, which certifies that you wrote it or otherwise have the right to pass it on as open source work. The rules are pretty simple: if you can certify the [Developer Certificate of Origin](https://developercertificate.org) then just add a line saying:
```
Signed-off-by: Random J Developer <random@developer.example.org>
```
This is easiest accomplished by committing using the `-s` flag:
```bash
git commit -s
```
If you need to add your sign off to a commit you have already made, please see this [article](https://docs.github.com/en/desktop/contributing-and-collaborating-using-github-desktop/managing-commits/amending-a-commit).

## Submitting a pull request

1. [Fork][fork] and clone the repository.
1. Create a new branch: `git checkout -b my-branch-name`.
1. Make your change, add tests, and make sure the tests still pass.
1. Push to your fork and [submit a pull request][pr].
1. Pat your self on the back and wait for your pull request to be reviewed and merged.

Here are a few things you can do that increase the likelihood of your pull request being accepted:

- Write and update tests.
- Keep your changes as focused as possible. If there are multiple changes you would like to make that are not dependent upon each other, consider submitting them as separate pull requests.
- Write a [good commit message](http://tbaggery.com/2008/04/19/a-note-about-git-commit-messages.html).

Work in Progress pull requests are also welcome to get feedback early on, or if there is something blocked you.

## Developer setup
NAuth was originally scaffolded with `kubebuilder` and still uses controller-gen markers and related conventions.
The repository layout and deployment workflow have since diverged from the default Kubebuilder structure.
`mise` is the canonical developer tool for installing the toolchain and running common project tasks.
The `Makefile` remains available as a compatibility layer for common contributor workflows.

You can use `mise` to setup the environment to the needed setup as well as run the required local environment. It
handles both tool installation and a convenient way to handle environments and tasks.

## Testing
Unit and integration tests run with `make test`. End-to-end coverage uses KUTTL scenarios under `test/e2e`.
Run them with:
```bash
make test-e2e
```
This target creates and deletes a Kind cluster as part of the run, so make sure Docker and `kubectl` are available.

### Local cluster setup
There are a couple of scripts to setup a complete local cluster with NATS as well as building and deploying the local NAuth build.
These scripts are provided as `mise` tasks, but are also possible to run standalone by running the shell scripts under `.mise-tasks`.

```bash
mise nauth:install
```

This installs both dependent resources such as `nats` but also adds a static provided `operator` which can be used
for testing.

### Trying out some examples
You can play around with permissions and such by applying examples and do publish and subscribe using the credentials.
By default, both an `example-account` and an `example-user` is created.

Open 3 different terminals to do port forwarding into your cluster and then do subscribe and publish.
```bash
mise nats:port-forward
mise nats:sub -- example-user foo.>
mise nats:pub -- example-user foo.test 'hello there'
```
You can of course do more advanced stuff, but this should get you started.

## Documentation
### Helm chart
Whenever updating Helm values, follow format of [`helm-docs`](https://github.com/norwoodj/helm-docs).

Then run:
```bash
mise nauth:generate-docs
```
(this runs `helm-docs` together with CRD reference generation)

### Version labels
The documentation website follows the current `main` branch. Mark release-sensitive features with:

- `Since vX.Y.Z` when the feature is available from a published release.
- `Unreleased` when the feature is documented on `main` before the next NAuth release.

Before tagging a release, replace relevant `Unreleased` labels with `Since vX.Y.Z`.

## Releasing
Releases are tag-driven and use [SemVer](https://semver.org/). Release candidate tags use SemVer
pre-release identifiers; the leading `v` is only the Git tag prefix and is stripped before publishing artifacts.

Release tags must use one of these formats:
- stable release: `vX.Y.Z`, for example `v0.7.0`
- release candidate: `vX.Y.Z-rc.N`, for example `v0.7.0-rc.1`

Do not use `v0.7.0-rc-1`; use `v0.7.0-rc.1`.

1. Create and push a new release tag:
   ```bash
   git tag v0.7.0-rc.1
   git push origin v0.7.0-rc.1
   ```
2. Create and publish the GitHub release from the GitHub UI.

The `Operator Release` workflow derives the release version from the tag (without the `v`) and uses it for:
- operator image tags/labels
- `charts/nauth/Chart.yaml` (`version` and `appVersion`) during packaging
- `charts/nauth-crds/Chart.yaml` (`version`) during packaging

Release candidates publish the same artifacts as stable releases. When creating an RC release in the GitHub UI:
- mark it as a pre-release
- do not mark it as the latest release

Install an RC explicitly:

```bash
helm upgrade --install nauth oci://ghcr.io/wirelesscar/nauth \
  --namespace nauth \
  --create-namespace \
  --version 0.7.0-rc.1
```

When promoting a tested RC line to a stable release, create the final stable tag:

```bash
git tag v0.7.0
git push origin v0.7.0
```

Release notes are generated from the latest previous non-RC release tag. For example, if `v0.6.3` is the latest stable
release and `v0.7.0-rc.1` plus `v0.7.0-rc.2` have been published, the final `v0.7.0` release notes start at `v0.6.3`,
not at `v0.7.0-rc.2`. When using generated release notes in the GitHub UI, select the latest previous non-RC release tag
as the previous tag before generating or publishing the notes.

## Resources

- [How to Contribute to Open Source](https://opensource.guide/how-to-contribute/)
- [Using Pull Requests](https://help.github.com/articles/about-pull-requests/)
- [GitHub Help](https://help.github.com)
