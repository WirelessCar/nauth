const timers = require('node:timers/promises');

const BREAKING_HEADER = /^[a-z][a-z0-9-]*(\([A-Za-z0-9._/-]+\))?!: .+/;
const MAX_CLASSIFICATION_ATTEMPTS = 3;

class PullRequestChangedError extends Error {}

function hasBreakingChange(message) {
  const lines = message.replace(/\r\n/g, '\n').split('\n');
  if (BREAKING_HEADER.test(lines[0])) return true;
  let fence = null;
  for (const line of lines.slice(1)) {
    const delimiter = /^\s*(`{3,}|~{3,})/.exec(line)?.[1];
    if (delimiter) {
      if (!fence) fence = delimiter;
      else if (delimiter[0] === fence[0] && delimiter.length >= fence.length) fence = null;
      continue;
    }
    if (!fence && /^(BREAKING CHANGE|BREAKING-CHANGE):[ \t]+\S/.test(line)) return true;
  }
  return false;
}

async function classifyPullRequest({ github, repo, pull_number }) {
  const { data: pr } = await github.rest.pulls.get({ ...repo, pull_number });
  let breaking = BREAKING_HEADER.test(pr.title);
  const seen = new Set();
  let cursor = null;
  do {
    const result = await github.graphql(`query($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
      repository(owner: $owner, name: $repo) {
        pullRequest(number: $number) {
          headRefOid
          commits(first: 100, after: $cursor) {
            totalCount
            nodes { commit { oid message } }
            pageInfo { hasNextPage endCursor }
          }
        }
      }
    }`, { ...repo, number: pull_number, cursor });
    const pull = result.repository?.pullRequest;
    if (!pull) throw new Error('Could not read PR commit data.');
    if (pull.headRefOid !== pr.head.sha || pull.commits.totalCount !== pr.commits) {
      throw new PullRequestChangedError('PR commits changed during classification; rerun Release Labels.');
    }
    const commits = pull.commits;
    for (const node of commits.nodes) {
      if (!node?.commit?.oid || typeof node.commit.message !== 'string' || seen.has(node.commit.oid)) {
        throw new Error('Incomplete or duplicate PR commit data.');
      }
      seen.add(node.commit.oid);
      breaking ||= hasBreakingChange(node.commit.message);
    }
    if (commits.pageInfo.hasNextPage && (!commits.nodes.length || !commits.pageInfo.endCursor || commits.pageInfo.endCursor === cursor)) {
      throw new Error('PR commit pagination stalled.');
    }
    cursor = commits.pageInfo.hasNextPage ? commits.pageInfo.endCursor : null;
  } while (cursor);
  if (seen.size !== pr.commits || seen.size === 0) throw new Error('Could not read every PR commit.');
  const { data: latest } = await github.rest.pulls.get({ ...repo, pull_number });
  if (latest.head.sha !== pr.head.sha || latest.title !== pr.title) {
    throw new PullRequestChangedError('PR changed during classification; rerun Release Labels.');
  }
  return { breaking, pr: latest };
}

module.exports = async function labelBreakingChange({ github, context, core }) {
  if (context.eventName !== 'pull_request_target') throw new Error('Labeling requires pull_request_target metadata.');
  const repo = context.repo;
  const pull_number = context.payload.pull_request.number;
  let classification;
  for (let attempt = 1; attempt <= MAX_CLASSIFICATION_ATTEMPTS; attempt++) {
    try {
      classification = await classifyPullRequest({ github, repo, pull_number });
      break;
    } catch (error) {
      if (!(error instanceof PullRequestChangedError) || attempt === MAX_CLASSIFICATION_ATTEMPTS) throw error;
      core.info(`PR #${pull_number}: metadata changed during classification; retrying (${attempt + 1}/${MAX_CLASSIFICATION_ATTEMPTS}).`);
      await timers.setTimeout(attempt * 1000);
    }
  }
  const { breaking, pr } = classification;
  const labeled = pr.labels.some(label => label.name === 'breaking-change');
  if (breaking && !labeled) {
    try {
      await github.rest.issues.getLabel({ ...repo, name: 'breaking-change' });
    } catch (error) {
      if (error.status !== 404) throw error;
      await github.rest.issues.createLabel({ ...repo, name: 'breaking-change', color: 'B60205',
        description: 'Introduces a breaking change; review the linked migration guidance' });
    }
    await github.rest.issues.addLabels({ ...repo, issue_number: pull_number, labels: ['breaking-change'] });
  } else if (!breaking && labeled) {
    await github.rest.issues.removeLabel({ ...repo, issue_number: pull_number, name: 'breaking-change' });
  }
  core.info(`PR #${pull_number}: breaking change ${breaking ? 'declared' : 'not declared'}.`);
};
