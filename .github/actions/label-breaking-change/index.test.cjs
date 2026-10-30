const { test } = require('node:test');
const assert = require('node:assert/strict');
const labelBreakingChange = require('./index.cjs');

function fixture({ title = 'fix: repair', messages = ['fix: repair'], labels = [] } = {}) {
  const pr = { title, commits: messages.length, head: { sha: 'head' }, labels: labels.map(name => ({ name })) };
  const calls = [];
  const github = {
    rest: {
      pulls: { get: async () => ({ data: structuredClone(pr) }) },
      issues: {
        getLabel: async () => ({ data: { name: 'breaking-change' } }),
        createLabel: async () => {},
        addLabels: async ({ labels: added }) => { calls.push(['add', added]); pr.labels.push(...added.map(name => ({ name }))); },
        removeLabel: async ({ name }) => { calls.push(['remove', name]); pr.labels = pr.labels.filter(label => label.name !== name); },
      },
    },
    graphql: async (_query, { cursor }) => {
      const start = cursor ? Number(cursor) : 0;
      const end = Math.min(start + 100, messages.length);
      return { repository: { pullRequest: { headRefOid: pr.head.sha, commits: {
        totalCount: messages.length,
        nodes: messages.slice(start, end).map((message, i) => ({ commit: { oid: `c${start + i}`, message } })),
        pageInfo: { hasNextPage: end < messages.length, endCursor: String(end) },
      } } } };
    },
  };
  const run = () => labelBreakingChange({ github,
    context: { repo: { owner: 'example', repo: 'project' }, eventName: 'pull_request_target', payload: { pull_request: { number: 12 } } },
    core: { info() {} },
  });
  return { github, pr, calls, run };
}

test('a breaking PR title adds its label without changing manual labels', async () => {
  const mock = fixture({ title: 'fix(api)!: remove a field', labels: ['deprecation', 'help wanted'] });
  await mock.run();
  assert.deepEqual(mock.pr.labels.map(label => label.name), ['deprecation', 'help wanted', 'breaking-change']);
  assert.deepEqual(mock.calls, [['add', ['breaking-change']]]);
});

test('a breaking commit beyond 250 commits is detected through every API page', async () => {
  const messages = Array(301).fill('fix: repair');
  messages[300] = 'refactor(api)!: remove the legacy field';
  const mock = fixture({ messages });
  await mock.run();
  assert.deepEqual(mock.calls, [['add', ['breaking-change']]]);
});

test('commit footer declarations add the breaking-change label', async t => {
  for (const message of [
    'fix: replace\n\nBREAKING CHANGE: remove the old field',
    'fix: replace\r\n\r\nBREAKING-CHANGE: remove the old field',
  ]) {
    await t.test(message, async () => {
      const mock = fixture({ messages: [message] });
      await mock.run();
      assert.deepEqual(mock.calls, [['add', ['breaking-change']]]);
    });
  }
});

test('removing the declaration removes only the managed label and repeat runs do nothing', async () => {
  const mock = fixture({ labels: ['breaking-change', 'deprecation', 'type: feature', 'autorelease: pending'] });
  await mock.run();
  assert.deepEqual(mock.pr.labels.map(label => label.name), ['deprecation', 'type: feature', 'autorelease: pending']);
  assert.deepEqual(mock.calls, [['remove', 'breaking-change']]);
  await mock.run();
  assert.equal(mock.calls.length, 1);
});

test('documentation examples and deprecation footers do not declare a breaking change', async t => {
  for (const message of [
    'docs: explain! this syntax',
    'docs: explain\n\nAvoid BREAKING CHANGE: in examples.\n> BREAKING CHANGE: quoted example',
    'docs: explain\n\n```text\nBREAKING CHANGE: example only\n```',
    'docs: explain\n\n~~~text\nBREAKING-CHANGE: example only\n~~~',
    'feat: replace\n\nDeprecation: use the replacement',
  ]) {
    await t.test(message, async () => {
      const mock = fixture({ title: 'docs: explain the syntax', messages: [message], labels: ['deprecation'] });
      await mock.run();
      assert.deepEqual(mock.calls, []);
      assert.deepEqual(mock.pr.labels, [{ name: 'deprecation' }]);
    });
  }
});

test('incomplete or changing API data reports a failure without changing labels', async t => {
  for (const kind of ['missing', 'duplicate', 'head', 'title', 'stalled']) {
    await t.test(kind, async () => {
      const mock = fixture({ messages: Array(101).fill('fix: repair'), labels: ['breaking-change'] });
      const graphql = mock.github.graphql;
      mock.github.graphql = async (...args) => {
        const result = await graphql(...args);
        const pull = result.repository.pullRequest;
        if (kind === 'missing') pull.commits.nodes = [];
        if (kind === 'duplicate') pull.commits.nodes.push(pull.commits.nodes[0]);
        if (kind === 'head') pull.headRefOid = 'new-head';
        if (kind === 'title') mock.pr.title = 'fix!: changed during classification';
        if (kind === 'stalled') pull.commits.pageInfo.endCursor = args[1].cursor;
        return result;
      };
      await assert.rejects(mock.run(), /commit|changed|page/i);
      assert.deepEqual(mock.calls, []);
    });
  }
});

test('the repository label is created when absent before it is applied', async () => {
  const mock = fixture({ title: 'fix!: remove a field' });
  mock.github.rest.issues.getLabel = async () => { throw Object.assign(new Error('missing'), { status: 404 }); };
  mock.github.rest.issues.createLabel = async ({ name }) => { mock.calls.push(['create', name]); };
  await mock.run();
  assert.deepEqual(mock.calls, [['create', 'breaking-change'], ['add', ['breaking-change']]]);
});

test('only the metadata event can run the labeler', async () => {
  const mock = fixture({ title: 'fix!: remove a field' });
  await assert.rejects(labelBreakingChange({ github: mock.github,
    context: { eventName: 'pull_request' }, core: {} }), /pull_request_target/);
  assert.deepEqual(mock.calls, []);
});

test('GitHub permission errors are reported instead of silently skipping labeling', async t => {
  for (const operation of ['read commits', 'read label', 'apply label']) {
    await t.test(operation, async () => {
      const mock = fixture({ title: 'fix!: remove a field' });
      const denied = async () => { throw Object.assign(new Error('permission denied'), { status: 403 }); };
      if (operation === 'read commits') mock.github.graphql = denied;
      if (operation === 'read label') mock.github.rest.issues.getLabel = denied;
      if (operation === 'apply label') mock.github.rest.issues.addLabels = denied;
      await assert.rejects(mock.run(), /permission denied/);
      assert.deepEqual(mock.calls, []);
    });
  }
});
