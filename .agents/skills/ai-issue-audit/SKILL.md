---
name: ai-issue-audit
description: Use when auditing or triaging GitHub issues for this repository against the current codebase, especially when checking whether issues are still relevant, grouping related issues, identifying dependencies, or drafting a maintainer-facing issue audit
---

# AI Issue Audit

## Overview

Use this skill to produce a short, code-backed GitHub issue audit for this repository.

Core rule: treat local code as the source of truth for implementation status, and treat issue text as a claim to verify, not a fact to repeat.

## When to Use

Use this when asked to:
- summarize or triage GitHub issues for this repo
- check whether issues are still relevant, obsolete, or partly addressed
- cross-reference issues with current code
- group related issues and show dependencies
- suggest issue types for maintainers
- draft or refresh a Markdown audit document

Do not use this for:
- PR review
- implementation planning for a single issue

## Workflow

1. Collect the issue set.
   - Prefer open issues unless the user asks for closed or historical issues too.
   - Exclude pull requests.
   - For reading issue content, prefer the public HTTPS GitHub issues pages for this repo (`https://github.com/<owner>/<repo>/issues` and individual `https://github.com/<owner>/<repo>/issues/<number>` pages).
   - Do not rely on SSH remotes for issue reading. Treat the public HTTPS issue pages as the default source for issue text whenever you need to inspect or verify issue content.
   - If GitHub API or `gh` is unavailable, unauthenticated, or blocked, continue with the public HTTPS issue pages and issue HTML instead of stopping.

2. Check for an existing audit document before drafting output.
   - If the audit file already exists (for example `docs/open-issue-audit.md`), update that file in place.
   - Refresh the existing audit so it matches the current open issue set and the current codebase state.
   - Do not create a second audit file for the same scope unless the user explicitly asks for a separate historical snapshot.

3. Cross-reference each issue against current code and docs.
   - Discover the most relevant code paths and docs from the issue text before concluding anything.
   - Start with obvious entrypoints, API types, controller logic, service/core logic, config, README, and repo-specific triage docs if present.
   - Prefer direct code evidence over speculation.
   - Distinguish:
     - implemented
     - partially addressed
     - not implemented
     - stale design assumption

4. Assign a relevance verdict.
   - `still relevant`: the gap is still visible in code or behavior
   - `partially addressed`: direction exists but the issue is not fully resolved
   - `stale/needs design`: topic may still matter, but the issue text is partly outdated or too architectural to treat as a direct bug/feature request

5. Suggest maintainer triage labels using `docs/triage.md`.
   - Use exactly one issue type suggestion:
     - `type: bug`
     - `type: feature`
     - `type: docs`
     - `type: question`
   - Suggest one outcome when useful:
     - `accepted`
     - `needs-more-info`
     - `invalid`
     - `wontfix`
   - Do not invent severity labels unless the user asks.

6. Group related issues.
   - Group by subsystem or problem family, not by creation date.
   - Let grouping themes emerge from the issue set and the codebase.
   - Name groups after the actual problem area you discover during the audit, not a pre-baked taxonomy.

7. Show dependencies clearly.
   - Mark `explicit dependency` only when issue text directly references another issue or makes the relationship concrete.
   - Mark `inferred dependency` when the dependency comes from current code/design coupling.
   - Mark `umbrella/meta issue` for tracker issues like deprecation rollups.
   - If there is no hard dependency, say `no hard dependency; grouped by theme`.

8. Order grouped issues by dependency.
   - Put root bugs or enabling work first.
   - Put broader design alternatives after the root gap they respond to.
   - Put tracker/meta issues after the concrete issues they track unless the user specifically wants trackers first.

9. Mark the output as AI-assisted.
   - Add a clear disclaimer near the top that the audit is AI-produced and based on current code plus public GitHub issue content.

## Output Template

When producing a Markdown audit, prefer this structure.

If an audit file already exists for the same scope, refresh that file instead of creating a new one.

### Header

- Title
- AI disclaimer
- One short sentence about scope and date reviewed

### Summary Table

Use these columns:

| Issue | Relevance | Initial triage | Suggested issue type | Dependency | Code reality | Short review |

For `Short review`, format each cell like this:

`**Pros:** ...<br>**Cons:** ...<br>**Accuracy:** ...`

### Related Issue Groups

For each group include:
- group title
- one-line theme
- ordered issue list
- for each issue:
  - `Dependency: ...`
  - `Suggested issue type: ...`
  - `Note: ...`

### Closing Findings

Keep this short:
- clearest current bugs/gaps
- stale-but-not-invalid issues
- overlapping issue clusters that should be normalized before implementation

## Common Mistakes

- Treating issue text as current truth without checking code.
- Hardcoding code paths before reading the issue set.
- Reusing old grouping names instead of deriving them from the current backlog.
- Calling a tracker issue a dependency instead of an umbrella/meta issue.
- Claiming hard dependencies when the issues are only thematically related.
- Mixing PRs into the issue set.
- Omitting the AI disclaimer.
- Using long prose where a short table plus grouped list would be easier to scan.

## Validation

Before finalizing an audit:
- verify the issue set excludes PRs
- verify issue reading used the public HTTPS GitHub issues pages when inspecting issue content
- verify any existing audit file for the same scope was refreshed in place rather than duplicated
- verify each dependency is labeled explicit, inferred, umbrella/meta, or no hard dependency
- verify suggested issue types match `docs/triage.md`
- verify the output references code paths when making implementation claims
- verify the audit is clearly labeled as AI-assisted
