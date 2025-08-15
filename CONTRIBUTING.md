# CONTRIBUTING
[fork]: /fork
[pr]: /compare
[code-of-conduct]: CODE_OF_CONDUCT.md

We're glad you would like to take part of contributing to Nauth!

> [!NOTE]
> The project is in an early phase. Contributing will be streamlined further down the road.

Please note that this project is released with a [Contributor Code of Conduct][code-of-conduct]. By participating in this project you agree to abide by its terms.

## Issues and PRs

If you have suggestions for how this project could be improved, or want to report a bug, open an issue! We'd love all and any contributions. If you have questions, too, we'd love to hear them.

We'd also love PRs. If you're thinking of a large PR, we advise opening up an issue first to talk about it, though! Look at the links below if you're not sure how to open a PR.

## Submitting a pull request

1. [Fork][fork] and clone the repository.
1. Create a new branch: `git checkout -b my-branch-name`.
1. Make your change, add tests, and make sure the tests still pass.
1. Push to your fork and [submit a pull request][pr].
1. Pat your self on the back and wait for your pull request to be reviewed and merged.

Here are a few things you can do that will increase the likelihood of your pull request being accepted:

- Write and update tests.
- Keep your changes as focused as possible. If there are multiple changes you would like to make that are not dependent upon each other, consider submitting them as separate pull requests.
- Write a [good commit message](http://tbaggery.com/2008/04/19/a-note-about-git-commit-messages.html).

Work in Progress pull requests are also welcome to get feedback early on, or if there is something blocked you.

## Developer setup
Nauth is bootstrapped and using the usual `kubebuilder` makefiles. These will however be removed over time and transitioned into using [mise-en-place](mise.jdx.dev)

You can use `mise` to setup the environment to the needed setup as well as run the required local environment. It
handles both tool installation and a convenient way to handle environments and tasks.

### Local cluster setup
There are a couple of scripts to setup a complete local cluster with NATS as well as building and deploying the local NAuth build.
These scripts are provided as `mise` tasks, but are also possible to run standalone by running the shell scripts under `.mise-tasks`.

```bash
mise run install
```

This will install both dependent resources such as `nats` but also adds a static provided `operator` which can be used
for testing.

### Trying out some examples
You can play around with permissions and such by applying examples and do publish and subscribe using the credentials.
By default, both an `example-account` and an `example-user` is created.

Open 3 different terminals to do port forwarding into your cluster and then do subscribe and publish.
```bash
mise nats:pf
mise nats:sub -- example-user foo.>
mise nats:pub -- example-user foo.test 'hello there'
```
You can of course do more advanced stuff, but this should get you started.

## Releasing
When building a release for the operator:

- Update the `.image_version` to new version
- Update the `chart/Chart.yaml` with updated `version` & `appVersion`

Make sure to follow valid [SemVer](https://semver.org) rules.

## Resources

- [How to Contribute to Open Source](https://opensource.guide/how-to-contribute/)
- [Using Pull Requests](https://help.github.com/articles/about-pull-requests/)
- [GitHub Help](https://help.github.com)
