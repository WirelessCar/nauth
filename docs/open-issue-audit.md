# Open Issue Audit

> [!IMPORTANT]
> This is an AI audit based on the current codebase and publicly available GitHub issue content reviewed on 2026-03-20. It is intended as a maintainer aid, not as authoritative project planning or final issue triage.

Open issues reviewed against the current `main` branch on 2026-03-20. The issue set was refreshed from the public HTTPS GitHub issues pages for `WirelessCar/nauth` and currently contains 17 open issues. This is a code-backed audit, not a prioritization plan. Verdicts use:

- `still relevant`: not implemented, or the reported gap is still visible in code
- `partially addressed`: direction exists, but the issue is not fully closed by current code
- `stale/needs design`: valid topic, but the issue text is now partly architectural or predates newer APIs

Key code anchors used repeatedly:

- `internal/adapter/inbound/controller/account.go`
- `internal/adapter/inbound/controller/user.go`
- `internal/core/account.go`
- `internal/core/accountClaims.go`
- `internal/core/cluster.go`
- `internal/core/secret.go`
- `internal/core/user.go`

## Summary Table

| Issue | Relevance | Initial triage | Suggested issue type | Dependency | Code reality | Short review |
| --- | --- | --- | --- | --- | --- | --- |
| [#10 Support creation of NACK user in NAuth](https://github.com/WirelessCar/nauth/issues/10) | still relevant | accepted | `type: feature` | no hard dependency; grouped with `#138` | No NACK integration exists; account reconciliation does not create child `User` resources. ADR-3 explicitly accepts this direction. | **Pros:** strong operator value, already aligned with ADR.<br>**Cons:** adds coupling and more reconcile surface.<br>**Accuracy:** mostly accurate. |
| [#11 Separate Imports and Exports to separate CRDs](https://github.com/WirelessCar/nauth/issues/11) | stale/needs design | needs-more-info | `type: feature` | no hard dependency; grouped with `#43` and `#119` | `AccountSpec` still embeds `exports` and `imports`; no separate CRDs exist. | **Pros:** better composability.<br>**Cons:** large API break and the issue leans on older v1alpha2 thinking.<br>**Accuracy:** plausible problem statement, but the proposed shape is a design choice, not a current bug. |
| [#27 Accounts removed even though Jetstreams remain](https://github.com/WirelessCar/nauth/issues/27) | still relevant | accepted | `type: feature` | no hard dependency; adjacent to `#59` | Delete flow blocks only on `User` CRs, then deletes the account JWT and secrets; there is no JetStream inspection. | **Pros:** protects data and cluster health.<br>**Cons:** ownership and discovery of JetStream state are non-trivial.<br>**Accuracy:** valid concern, though current code removes the account rather than explicitly purging streams. |
| [#35 Accounts are not reconciled if missing in NATS](https://github.com/WirelessCar/nauth/issues/35) | still relevant | accepted | `type: feature` | no hard dependency; adjacent to `#59` and `#27` | Reconciliation is generation-driven; no remote drift detection or re-import happens unless spec changes. | **Pros:** restores declarative behavior.<br>**Cons:** adds more NATS calls and failure modes.<br>**Accuracy:** good. |
| [#43 Account imports do not validate exports from target account](https://github.com/WirelessCar/nauth/issues/43) | still relevant | accepted | `type: bug` | root issue for `#119` | Import building resolves only the target account ID; it does not verify the target export policy. | **Pros:** closes a real policy gap.<br>**Cons:** may overlap with later activation-token or private-export work.<br>**Accuracy:** strong. |
| [#59 Add lifecycle policy for nauth resources](https://github.com/WirelessCar/nauth/issues/59) | partially addressed | needs-more-info | `type: feature` | broad policy issue for lifecycle safety cluster | There is already an observe-only management policy label, but no broader add/update-only or takeover lifecycle model. | **Pros:** improves safety for destructive ops.<br>**Cons:** policy surface can grow quickly.<br>**Accuracy:** partly outdated because observe mode already exists. |
| [#95 Deleting a User should revoke it](https://github.com/WirelessCar/nauth/issues/95) | still relevant | accepted | `type: bug` | root issue for `#135`, `#140`, and indirectly `#132` | User deletion only removes the generated secret; it does not revoke the user in NATS or update any revocation list. | **Pros:** closes a real security gap.<br>**Cons:** implementation depends on signing-key design choices.<br>**Accuracy:** strong. |
| [#102 Sunset deprecated features from previous releases](https://github.com/WirelessCar/nauth/issues/102) | still relevant | accepted | `type: feature` | umbrella/meta issue tracking `#144` and other sunsets | Deprecations are documented and referenced in TODOs, but removals are not complete. | **Pros:** useful release tracker.<br>**Cons:** broad meta-issue, not very actionable alone.<br>**Accuracy:** true. |
| [#119 Support Export & Import using Activation Tokens](https://github.com/WirelessCar/nauth/issues/119) | still relevant | accepted | `type: feature` | inferred follow-on from `#43` | Current API has no activation-token input path for imports; current import/export handling is simpler than the issue asks for. | **Pros:** needed for private exports and imports.<br>**Cons:** introduces token lifecycle and UX complexity.<br>**Accuracy:** mostly right; “all exports are public” is slightly simplified. |
| [#132 Implement signing key rotation for NATS Accounts](https://github.com/WirelessCar/nauth/issues/132) | still relevant | accepted | `type: feature` | downstream from the chosen signing-key model (`#135` or `#140`) | The account signing key is persisted as a normal secret and reused; there is no rotation flow. | **Pros:** reduces operator burden.<br>**Cons:** rotation interacts with revocation and client credential rollover.<br>**Accuracy:** strong. |
| [#135 Separate signing key lifecycle from account](https://github.com/WirelessCar/nauth/issues/135) | still relevant | accepted | `type: feature` | inferred follow-on from `#95` | No `AccountSigningKey` CRD or separate signing-key lifecycle exists today. | **Pros:** cleaner long-term model than overloading `Account`.<br>**Cons:** larger API and overlaps with `#140`.<br>**Accuracy:** reasonable and concrete. |
| [#138 Support predictable secret names for external consumers](https://github.com/WirelessCar/nauth/issues/138) | still relevant | accepted | `type: feature` | no hard dependency; grouped with `#10` | Account secrets are hash-suffixed (`%s-ac-root-%s`, `%s-ac-sign-%s`); predictable old names remain only as deprecated legacy lookup. | **Pros:** much better for GitOps and external integrations.<br>**Cons:** reintroduces collision and orphan risks that hash naming fixed.<br>**Accuracy:** strong and grounded in current code. |
| [#140 Support Scoped Signing Keys (Roles/Users)](https://github.com/WirelessCar/nauth/issues/140) | stale/needs design | needs-more-info | `type: feature` | explicit relation to `#95`; design-dependent alternative to `#135` | There is no scoped-signing-key API today, only a TODO on `SigningKey` mentioning optional `UserScope`. | **Pros:** decouples authn from authz and reduces forced credential rotation.<br>**Cons:** large model change and revocation semantics remain tricky.<br>**Accuracy:** plausible, but still architecture-level and underspecified. |
| [#144 Sunset legacy implicit label-based secrets lookup](https://github.com/WirelessCar/nauth/issues/144) | partially addressed | accepted | `type: feature` | inferred follow-on from `#178`; tracked by `#102` | Explicit `NatsClusterRef`/`NatsCluster` support exists, but `NATS_URL` and implicit label-based secret lookup are still active and marked deprecated. | **Pros:** removes ambiguous legacy behavior.<br>**Cons:** needs a deliberate migration and breaking-change plan.<br>**Accuracy:** true. |
| [#178 reconcile NatsCluster resources and validate cluster connectivity](https://github.com/WirelessCar/nauth/issues/178) | still relevant | accepted | `type: feature` | root issue for the explicit cluster path; enables `#144` | `NatsCluster` exists only as a lookup target; there is no reconciler, status, or readiness condition, and connectivity is only checked indirectly when accounts use it. | **Pros:** better feedback and Argo CD behavior.<br>**Cons:** adds runtime NATS connectivity to reconciliation.<br>**Accuracy:** strong. |
| [#184 replace Ginkgo test suites with Testify](https://github.com/WirelessCar/nauth/issues/184) | still relevant | accepted | `type: feature` | no hard dependency; grouped by theme | Remaining Ginkgo/Gomega suites still exist under `internal/adapter/inbound/controller` and `internal/adapter/outbound/k8s`; several files still carry TODOs for this migration. | **Pros:** improves test consistency and editor support.<br>**Cons:** broad mechanical refactor with some churn risk.<br>**Accuracy:** strong, though the existing TODOs still reference `#183` rather than `#184`. |
| [#185 remove core/controller dependencies on adapter k8s package](https://github.com/WirelessCar/nauth/issues/185) | still relevant | accepted | `type: feature` | no hard dependency; grouped by theme | `internal/core/account.go`, `internal/core/accountClaims.go`, `internal/core/cluster.go`, `internal/core/secret.go`, and `internal/adapter/inbound/controller/account.go` still import `internal/adapter/outbound/k8s` for shared constants, with TODOs acknowledging the layering problem. | **Pros:** improves package boundaries and architecture clarity.<br>**Cons:** mostly structural work without direct user-visible behavior.<br>**Accuracy:** strong. |

## Related Issue Groups

Dependencies below are based on explicit issue references when present, otherwise on current code and design coupling.

### 1. User Revocation And Signing-Key Lifecycle

Theme: identity invalidation, signing-key ownership, and future-safe user authz.

1. [#95](https://github.com/WirelessCar/nauth/issues/95) `Deleting a User should revoke it`
   Dependency: root issue
   Suggested issue type: `type: bug`
   Note: Current code deletes only the generated secret.
2. [#135](https://github.com/WirelessCar/nauth/issues/135) `Separate signing key lifecycle from account`
   Dependency: inferred follow-on from `#95`
   Suggested issue type: `type: feature`
   Note: Introduces a cleaner signing-key lifecycle model that can support revocation and narrower blast radius.
3. [#140](https://github.com/WirelessCar/nauth/issues/140) `Support Scoped Signing Keys (Roles/Users)`
   Dependency: explicit relation to `#95`; inferred dependency on choosing the signing-key direction first
   Suggested issue type: `type: feature`
   Note: Architectural branch, not a hard next step.
4. [#132](https://github.com/WirelessCar/nauth/issues/132) `Implement signing key rotation for NATS Accounts`
   Dependency: inferred downstream work after the signing-key model is chosen
   Suggested issue type: `type: feature`
   Note: Rotation semantics depend on whether the repo keeps the current model, adopts `#135`, or adopts `#140`.

### 2. NATS Cluster Targeting And Legacy Sunset

Theme: make explicit `NatsCluster` usage complete, then remove implicit legacy behavior.

1. [#178](https://github.com/WirelessCar/nauth/issues/178) `reconcile NatsCluster resources and validate cluster connectivity`
   Dependency: root issue
   Suggested issue type: `type: feature`
   Note: Makes the explicit `NatsCluster` path operationally complete.
2. [#144](https://github.com/WirelessCar/nauth/issues/144) `Sunset legacy implicit label-based secrets lookup`
   Dependency: inferred follow-on from `#178`
   Suggested issue type: `type: feature`
   Note: Easier and safer once the explicit cluster path has status and validation.
3. [#102](https://github.com/WirelessCar/nauth/issues/102) `Sunset deprecated features from previous releases`
   Dependency: umbrella/meta issue tracking items such as `#144`
   Suggested issue type: `type: feature`
   Note: Not a concrete prerequisite on its own.

### 3. Cross-Account Authorization Model

Theme: correctness first, then richer private-sharing semantics, then optional API redesign.

1. [#43](https://github.com/WirelessCar/nauth/issues/43) `Account imports do not validate exports from target account`
   Dependency: root issue
   Suggested issue type: `type: bug`
   Note: Current code accepts imports without validating export authorization.
2. [#119](https://github.com/WirelessCar/nauth/issues/119) `Support Export & Import using Activation Tokens`
   Dependency: inferred follow-on from `#43`
   Suggested issue type: `type: feature`
   Note: Builds on the same cross-account authz area after the current correctness gap is closed.
3. [#11](https://github.com/WirelessCar/nauth/issues/11) `Separate Imports and Exports to separate CRDs`
   Dependency: no hard dependency; grouped by theme
   Suggested issue type: `type: feature`
   Note: Broader API redesign, not required to fix `#43` or implement `#119`.

### 4. Account Lifecycle Safety

Theme: prevent destructive or stale account state from violating declarative expectations.

1. [#59](https://github.com/WirelessCar/nauth/issues/59) `Add lifecycle policy for nauth resources`
   Dependency: broad policy issue
   Suggested issue type: `type: feature`
   Note: Partly addressed already by observe mode, but still the main umbrella for lifecycle controls.
2. [#27](https://github.com/WirelessCar/nauth/issues/27) `Accounts removed even though Jetstreams remain`
   Dependency: no hard dependency; grouped by theme
   Suggested issue type: `type: feature`
   Note: Concrete delete-safety behavior that could live under a wider lifecycle policy.
3. [#35](https://github.com/WirelessCar/nauth/issues/35) `Accounts are not reconciled if missing in NATS`
   Dependency: no hard dependency; adjacent state-integrity issue
   Suggested issue type: `type: feature`
   Note: Not blocked by `#59` or `#27`.

### 5. External Integrations And Secret Consumption

Theme: smoother integration with external controllers and GitOps consumers.

1. [#138](https://github.com/WirelessCar/nauth/issues/138) `Support predictable secret names for external consumers`
   Dependency: no hard dependency; grouped by theme
   Suggested issue type: `type: feature`
   Note: Improves account-secret consumption by external systems.
2. [#10](https://github.com/WirelessCar/nauth/issues/10) `Support creation of NACK user in NAuth`
   Dependency: no hard dependency; grouped by theme
   Suggested issue type: `type: feature`
   Note: Operator-managed credentials for NACK and similar integrations.

### 6. Test Modernization

Theme: standardize the remaining test surface on the repository's current non-Ginkgo style.

1. [#184](https://github.com/WirelessCar/nauth/issues/184) `replace Ginkgo test suites with Testify`
   Dependency: no hard dependency; grouped by theme
   Suggested issue type: `type: feature`
   Note: The migration has not happened yet; the remaining suites are still concentrated in controller and outbound-k8s tests.

### 7. Dependency Direction And Package Boundaries

Theme: remove adapter leakage from core and controller layers.

1. [#185](https://github.com/WirelessCar/nauth/issues/185) `remove core/controller dependencies on adapter k8s package`
   Dependency: no hard dependency; grouped by theme
   Suggested issue type: `type: feature`
   Note: This is directly visible in current imports and TODO comments, so the issue is current rather than aspirational.

## Notable Findings

- The clearest current bugs/gaps are [#43](https://github.com/WirelessCar/nauth/issues/43), [#95](https://github.com/WirelessCar/nauth/issues/95), and [#178](https://github.com/WirelessCar/nauth/issues/178). All three are directly visible in current code.
- [#59](https://github.com/WirelessCar/nauth/issues/59) is the best example of an issue that is not invalid, but is partly stale because `observe` mode already exists.
- The signing-key cluster around [#95](https://github.com/WirelessCar/nauth/issues/95), [#132](https://github.com/WirelessCar/nauth/issues/132), [#135](https://github.com/WirelessCar/nauth/issues/135), and [#140](https://github.com/WirelessCar/nauth/issues/140) is still unresolved. The issues are relevant, but they overlap and should probably be normalized into one implementable direction before coding.
- The deprecation cluster around [#102](https://github.com/WirelessCar/nauth/issues/102) and [#144](https://github.com/WirelessCar/nauth/issues/144) is valid backlog, not dead backlog. The code already acknowledges the removal work, but the sunset has not happened yet.
- Two new open issues landed on 2026-03-20: [#184](https://github.com/WirelessCar/nauth/issues/184) and [#185](https://github.com/WirelessCar/nauth/issues/185). Both are already backed by current code evidence and fit as active cleanup/refactor backlog rather than speculative design ideas.
