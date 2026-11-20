# Breaking-change labeler

This composite action maintains only the `breaking-change` label.
It checks the PR title and every commit message for Conventional Commit `!` headers or
`BREAKING CHANGE:` / `BREAKING-CHANGE:` commit footers.
Deprecation labels remain manually controlled.

## Usage

```yaml
- uses: ./.github/actions/label-breaking-change
  with:
    github-token: ${{ github.token }}
```

The required token needs `pull-requests: write`; the workflow also needs `contents: read` for checkout.
Use `pull_request_target` and check out trusted base-branch code.
The action reads PR metadata through the API and never executes PR code or commit messages.
It uses `actions/github-script` for the GitHub client and runner context.

The repository requires an active Actions event policy allowing `pull_request_target` for exactly
`.github/workflows/release-labels.yml`. Configure the policy under Settings > Actions > Policies;
it is a repository setting, not workflow YAML. See [GitHub's policy guide](https://docs.github.com/en/actions/how-tos/administer/control-workflow-execution).

The action creates the repository label if needed, preserves other labels, and removes the label when
no breaking-change declarations remain. If the PR title, head, or commit count changes during
classification, the action reads all metadata and commits again, for up to three total attempts.
It waits one second before the second attempt and two seconds before the third. Labels are updated
only after a consistent classification. API errors and incomplete data fail immediately; PRs that
keep changing fail after the third attempt.
See the [maintainer guide](../../../MAINTENANCE.md#release-classification) for release review.

## Development

With Node.js 24, run from the repository root:

```sh
node --check .github/actions/label-breaking-change/index.cjs
node --test .github/actions/label-breaking-change/index.test.cjs
```

Tests call the action's entry point with fake GitHub API responses and inspect label changes or failures.
They cover commit pagination, declaration changes, manual labels, and API failures.
