# Open issue audit

> [!IMPORTANT]
> This is an AI-assisted audit based on the current codebase and public GitHub issue content reviewed on 2026-05-13. It is intended as a maintainer aid, not authoritative project planning or final issue triage.

Open issues reviewed against `main` at `1f7ebe7` on 2026-05-13. The issue set was refreshed from GitHub and currently contains 21 open issues. Verdicts use:

- `still relevant`: not implemented, or the reported gap is still visible in code
- `partially addressed`: direction exists, but the issue is not fully closed by current code
- `stale/needs design`: valid topic, but the issue text is partly outdated or too architectural to treat as a direct bug/feature request

Key code anchors used repeatedly:

- `api/v1alpha1/account_types.go`
- `api/v1alpha1/account_export_types.go`
- `api/v1alpha1/account_import_types.go`
- `api/v1alpha1/natscluster_types.go`
- `internal/adapter/inbound/controller/account.go`
- `internal/adapter/inbound/controller/account_export.go`
- `internal/adapter/inbound/controller/account_import.go`
- `internal/adapter/inbound/controller/natscluster.go`
- `internal/adapter/inbound/controller/user.go`
- `internal/core/account.go`
- `internal/core/account_claims.go`
- `internal/core/cluster.go`
- `internal/core/secret.go`
- `internal/core/user.go`
- `Makefile`
- `mise.toml`
- `.github/workflows/*`

## Summary table

| Issue | Relevance | Initial triage | Suggested issue type | Dependency | Code reality | Short review |
| --- | --- | --- | --- | --- | --- | --- |
| [#10 Support creation of NACK user in NAuth](https://github.com/WirelessCar/nauth/issues/10) | still relevant | accepted | `type: feature` | no hard dependency; grouped by theme | ADR-3 accepts this direction, but account reconciliation does not create child `User` resources or NACK-specific credentials. Existing examples still model NACK users explicitly. | **Pros:** strong operator value for JetStream/NACK users.<br>**Cons:** adds cross-resource ownership and permission packaging.<br>**Accuracy:** accurate. |
| [#11 Separate Imports and Exports to separate CRDs](https://github.com/WirelessCar/nauth/issues/11) | partially addressed | accepted | `type: feature` | root issue for import/export split; grouped with `#43` and `#119` | `AccountExport` and `AccountImport` CRDs, controllers, chart CRDs, and docs now exist. Inline `Account.spec.exports` and `Account.spec.imports` still exist, and the issue checklist is stale. | **Pros:** the main CRD split is implemented.<br>**Cons:** inline compatibility and contract validation remain.<br>**Accuracy:** issue is now partly outdated. |
| [#35 Accounts are not reconciled if missing in NATS](https://github.com/WirelessCar/nauth/issues/35) | partially addressed | accepted | `type: feature` | no hard dependency; adjacent to `#235` | `status.claimsHash` now avoids unnecessary uploads, but normal reconciliation still does not periodically fetch the remote account JWT or repair remote drift unless Kubernetes events enqueue the account. | **Pros:** preserves declarative intent.<br>**Cons:** periodic remote checks add NATS load and failure modes.<br>**Accuracy:** still true, with hash groundwork added. |
| [#43 Account imports do not validate exports from target account](https://github.com/WirelessCar/nauth/issues/43) | still relevant | accepted | `type: bug` | root issue for `#119` | `AccountImport` resolves importing/exporting account IDs and validates import syntax, but it does not load an `AccountExport` or verify exporter policy. ADR-5 option 4 remains target design, not current behavior. | **Pros:** closes a real authorization gap.<br>**Cons:** overlaps with the future contract model.<br>**Accuracy:** strong. |
| [#59 Add lifecycle policy for nauth resources](https://github.com/WirelessCar/nauth/issues/59) | partially addressed | needs-more-info | `type: feature` | broad policy issue; grouped with account lifecycle gaps | Observe mode exists, account deletion now blocks users/imports/exports and JetStream streams, and `NatsCluster` deletion blocks bound accounts. There is still no CRD-level add/update-only, adoption, or takeover lifecycle policy. | **Pros:** useful safety umbrella.<br>**Cons:** broad enough to need a sharper policy model.<br>**Accuracy:** partly outdated but not invalid. |
| [#95 Deleting a User should revoke it](https://github.com/WirelessCar/nauth/issues/95) | still relevant | accepted | `type: bug` | root issue for `#135`, `#140`, and `#132` | `UserManager.Delete` deletes only the Kubernetes credential secret. It does not update account revocations or invalidate the user JWT in NATS. | **Pros:** clear security gap.<br>**Cons:** final design depends on signing-key direction.<br>**Accuracy:** strong. |
| [#102 Sunset deprecated features from previous releases](https://github.com/WirelessCar/nauth/issues/102) | partially addressed | accepted | `type: feature` | umbrella/meta issue | `NATS_URL` and the old operator secret lookup path are gone, and `#144` is closed. Deprecated `UserStatus.UserClaims.AccountName` and deprecated account secret-name lookup still remain. | **Pros:** useful release hygiene tracker.<br>**Cons:** checklist now mixes closed and still-open sunset work.<br>**Accuracy:** mostly accurate, but needs refresh. |
| [#119 Support Export & Import using Activation Tokens](https://github.com/WirelessCar/nauth/issues/119) | still relevant | accepted | `type: feature` | inferred follow-on from `#43`; design-coupled to `#135`/`#140` | Current `AccountExport`/`AccountImport` child CRDs do not model activation tokens. `toJWTImport` has no token path, and private export/import workflow is not implemented. | **Pros:** needed for private cross-account sharing.<br>**Cons:** requires token lifecycle and signing-key decisions.<br>**Accuracy:** strong. |
| [#132 Implement signing key rotation for NATS Accounts](https://github.com/WirelessCar/nauth/issues/132) | still relevant | accepted | `type: feature` | inferred downstream from `#135`/`#140` | The account signing key is created once as a normal secret and reused. There is no rotation schedule, dual-key transition window, or client credential rollover flow. | **Pros:** reduces long-lived signing-key risk.<br>**Cons:** rotation interacts with revocation and mounted credentials.<br>**Accuracy:** strong. |
| [#135 Separate signing key lifecycle from account](https://github.com/WirelessCar/nauth/issues/135) | still relevant | accepted | `type: feature` | inferred root for `#132` and `#138`; overlaps with `#140` | No `AccountSigningKey` CRD exists. `AccountManager` creates a single signing key secret, and `SignUserJWT` always signs users with that default key. | **Pros:** clean MVP path for explicit signing keys.<br>**Cons:** needs API and migration design.<br>**Accuracy:** strong. |
| [#138 Support predictable secret names for external consumers](https://github.com/WirelessCar/nauth/issues/138) | stale/needs design | needs-more-info | `type: feature` | inferred dependency on `#135`/`#140` | Account root/signing secrets are hash-suffixed. Deprecated fixed names are only used as a legacy lookup fallback; there is no declarative custom secret name. Discussion has shifted toward `AccountSigningKey`. | **Pros:** real GitOps/external-consumer need.<br>**Cons:** original field-level solution may conflict with signing-key CRD design.<br>**Accuracy:** use case is accurate; solution needs design. |
| [#140 Support AccountSigningKey resources for scoped signing keys and implicit user authorization](https://github.com/WirelessCar/nauth/issues/140) | stale/needs design | needs-more-info | `type: feature` | explicit relation to `#95` and `#135`; broad design issue | There is no scoped-signing-key API. `SigningKey` has only `key` plus TODOs for scope mapping, and `User` cannot reference a signing key. | **Pros:** aligns with NATS scoped signing-key primitives.<br>**Cons:** large model change with open revocation/migration questions.<br>**Accuracy:** valid but architectural. |
| [#185 remove core/controller dependencies on adapter k8s package](https://github.com/WirelessCar/nauth/issues/185) | still relevant | accepted | `type: feature` | root issue for architecture boundary cleanup | `internal/core/secret.go`, `internal/core/user.go`, tests, and `internal/adapter/inbound/controller/account.go` still import or depend on `internal/adapter/outbound/k8s` concepts. | **Pros:** clear dependency-direction cleanup.<br>**Cons:** mostly structural work.<br>**Accuracy:** strong. |
| [#190 ci: enforce hexagonal dependency rules as a PR check](https://github.com/WirelessCar/nauth/issues/190) | still relevant | accepted | `type: feature` | inferred downstream from `#185`, `#196`, and `#228` | No dependency-rule check exists in `.golangci.yml` or workflows. Current `main` also has no `ARCHITECTURE.md`, so the referenced rule source needs to land or be moved before hard enforcement. | **Pros:** prevents boundary regressions.<br>**Cons:** needs an executable rule source and temporary exceptions.<br>**Accuracy:** mostly accurate; doc reference is off current main. |
| [#196 refactor: decouple secret handling from Kubernetes Secret and label concepts](https://github.com/WirelessCar/nauth/issues/196) | still relevant | accepted | `type: feature` | inferred companion to `#185` | `outbound.SecretClient` exposes Kubernetes-style secret/label operations. Core secret handling builds Kubernetes labels, depends on k8s constants, and parses `corev1.SecretList`. | **Pros:** makes the port honest and domain-oriented.<br>**Cons:** touches shared secret storage paths.<br>**Accuracy:** strong. |
| [#214 refactor: tighten CRD reconciliation write paths and RBAC for Account/User controllers](https://github.com/WirelessCar/nauth/issues/214) | still relevant | accepted | `type: feature` | root issue for safer controller write paths | Account/User RBAC markers and Helm RBAC still grant `create`, `delete`, and `update` on parent CRs. Both controllers still use full-object `Update` for finalizers/labels and work around status mutation. | **Pros:** concrete least-privilege improvement.<br>**Cons:** requires careful status/metadata test coverage.<br>**Accuracy:** strong. |
| [#223 Consolidate repo workflows by making Mise the canonical task runner](https://github.com/WirelessCar/nauth/issues/223) | still relevant | accepted | `type: feature` | no hard dependency; grouped by maintenance theme | `Makefile` remains the primary task surface; `mise.toml` has only a few tasks, CI still invokes `make test` and `make test-e2e`, and docs still mix make and mise. | **Pros:** reduces workflow drift.<br>**Cons:** needs CI/docs/tooling sweep.<br>**Accuracy:** strong. |
| [#228 Untangle the contract between inbound controllers and outbound Kubernetes adapters](https://github.com/WirelessCar/nauth/issues/228) | partially addressed | accepted | `type: feature` | inferred follow-on from `#185`; grouped with boundary work | `AccountExportReader` is no longer visible, but `AccountReader` still exists as an outbound-style contract and `AccountReconciler` depends directly on `k8s.AccountReader`. | **Pros:** improves controller ownership of Kubernetes-owned lookups.<br>**Cons:** current issue body is partly stale.<br>**Accuracy:** direction is right; scope needs refresh. |
| [#235 Reconcile Accounts when referenced NatsCluster configuration changes](https://github.com/WirelessCar/nauth/issues/235) | still relevant | accepted | `type: feature` | no hard dependency; follows closed `#178` work | `NatsCluster` now has a validator reconciler, but `AccountReconciler` does not watch `NatsCluster`, referenced `Secret`, or referenced `ConfigMap` changes. Account binding uses cluster UID, not effective URL/signing/SYS identities. | **Pros:** fixes stale account JWTs after cluster input changes.<br>**Cons:** URL-change safety needs deliberate behavior.<br>**Accuracy:** strong. |
| [#245 Align Account JetStream defaults with nats-io/jwt](https://github.com/WirelessCar/nauth/issues/245) | still relevant | accepted | `type: feature` | explicit dependency on `#246` release state | `jetStreamEnabled` now exists, and `#246` prepared the code. Current behavior still defaults unset JetStream to unlimited via a TODO in `newAccountClaimsBuilder`, preserving legacy behavior until the breaking flip. | **Pros:** aligns NAuth with upstream JWT semantics.<br>**Cons:** breaking business behavior for existing accounts.<br>**Accuracy:** strong. |
| [#274 Dependency Dashboard](https://github.com/WirelessCar/nauth/issues/274) | still relevant | accepted | `type: feature` | umbrella/meta issue for dependency updates | Renovate tracks pending or scheduled updates across GitHub Actions, Go modules, Helm charts, local images, Mise tools, and www dependencies. This is a bot-maintained maintenance tracker, not a code defect. | **Pros:** useful dependency visibility.<br>**Cons:** not actionable as one implementation issue.<br>**Accuracy:** accurate as dashboard state. |

## Related issue groups

Dependencies below are based on explicit issue references when present, otherwise on current code and design coupling.

### 1. Import/export contract model

Theme: finish the split, then make cross-account authorization correct.

1. [#11](https://github.com/WirelessCar/nauth/issues/11) `Separate Imports and Exports to separate CRDs`
   Dependency: root issue for the CRD split
   Suggested issue type: `type: feature`
   Note: The CRDs and controllers exist, but inline compatibility and checklist cleanup remain.
2. [#43](https://github.com/WirelessCar/nauth/issues/43) `Account imports do not validate exports from target account`
   Dependency: root correctness bug
   Suggested issue type: `type: bug`
   Note: Current `AccountImport` does not verify exporter-side policy or an `AccountExport` contract.
3. [#119](https://github.com/WirelessCar/nauth/issues/119) `Support Export & Import using Activation Tokens`
   Dependency: inferred follow-on from `#43`; design-coupled to signing keys
   Suggested issue type: `type: feature`
   Note: Private export/import support is still absent from the CRD model.

### 2. User revocation and signing-key lifecycle

Theme: invalidate users correctly, then model signing keys explicitly.

1. [#95](https://github.com/WirelessCar/nauth/issues/95) `Deleting a User should revoke it`
   Dependency: root issue
   Suggested issue type: `type: bug`
   Note: Deleting a `User` deletes only the generated secret.
2. [#135](https://github.com/WirelessCar/nauth/issues/135) `Separate signing key lifecycle from account`
   Dependency: inferred follow-on from `#95`
   Suggested issue type: `type: feature`
   Note: This is the narrower `AccountSigningKey` MVP path.
3. [#140](https://github.com/WirelessCar/nauth/issues/140) `Support AccountSigningKey resources for scoped signing keys and implicit user authorization`
   Dependency: explicit relation to `#95` and `#135`; broader design issue
   Suggested issue type: `type: feature`
   Note: Scoped signing keys need an architecture decision before implementation.
4. [#138](https://github.com/WirelessCar/nauth/issues/138) `Support predictable secret names for external consumers`
   Dependency: inferred dependency on `#135`/`#140`
   Suggested issue type: `type: feature`
   Note: The need is real, but the implementation should not fight the signing-key CRD direction.
5. [#132](https://github.com/WirelessCar/nauth/issues/132) `Implement signing key rotation for NATS Accounts`
   Dependency: inferred downstream work after signing-key model choice
   Suggested issue type: `type: feature`
   Note: Rotation depends on whether NAuth keeps one default key or introduces explicit/scoped keys.

### 3. Account state, cluster changes, and lifecycle safety

Theme: keep rendered account state aligned with NATS and make destructive changes deliberate.

1. [#235](https://github.com/WirelessCar/nauth/issues/235) `Reconcile Accounts when referenced NatsCluster configuration changes`
   Dependency: no hard dependency; follows the closed `NatsCluster` reconciler work from `#178`
   Suggested issue type: `type: feature`
   Note: Account enqueueing on effective cluster-input changes is still missing.
2. [#35](https://github.com/WirelessCar/nauth/issues/35) `Accounts are not reconciled if missing in NATS`
   Dependency: no hard dependency; adjacent drift-repair issue
   Suggested issue type: `type: feature`
   Note: `claimsHash` helps compare desired local state but does not detect remote deletion by itself.
3. [#245](https://github.com/WirelessCar/nauth/issues/245) `Align Account JetStream defaults with nats-io/jwt`
   Dependency: explicit dependency on the release state of `#246`
   Suggested issue type: `type: feature`
   Note: Code is prepared, but the breaking default flip has not happened.
4. [#59](https://github.com/WirelessCar/nauth/issues/59) `Add lifecycle policy for nauth resources`
   Dependency: broad policy issue
   Suggested issue type: `type: feature`
   Note: Current safety checks are specific, not a general lifecycle policy.
5. [#102](https://github.com/WirelessCar/nauth/issues/102) `Sunset deprecated features from previous releases`
   Dependency: umbrella/meta issue
   Suggested issue type: `type: feature`
   Note: Some tracked items are closed; deprecated status and secret-lookup paths remain.

### 4. Architecture boundaries and controller hygiene

Theme: reduce adapter leakage before enforcing architecture rules.

1. [#214](https://github.com/WirelessCar/nauth/issues/214) `refactor: tighten CRD reconciliation write paths and RBAC for Account/User controllers`
   Dependency: root issue for safer controller write paths
   Suggested issue type: `type: feature`
   Note: Parent CR RBAC and full-object updates are still too broad.
2. [#185](https://github.com/WirelessCar/nauth/issues/185) `remove core/controller dependencies on adapter k8s package`
   Dependency: root boundary issue
   Suggested issue type: `type: feature`
   Note: Direct imports and TODOs are still present.
3. [#196](https://github.com/WirelessCar/nauth/issues/196) `refactor: decouple secret handling from Kubernetes Secret and label concepts`
   Dependency: inferred companion to `#185`
   Suggested issue type: `type: feature`
   Note: Secret handling is the biggest visible adapter leak in core.
4. [#228](https://github.com/WirelessCar/nauth/issues/228) `Untangle the contract between inbound controllers and outbound Kubernetes adapters`
   Dependency: inferred follow-on from `#185`
   Suggested issue type: `type: feature`
   Note: Partly addressed, but `AccountReader` coupling remains.
5. [#190](https://github.com/WirelessCar/nauth/issues/190) `ci: enforce hexagonal dependency rules as a PR check`
   Dependency: inferred downstream from `#185`, `#196`, and `#228`
   Suggested issue type: `type: feature`
   Note: Enforcement should either wait for cleanup or explicitly allow temporary exceptions.

### 5. Repository workflow and dependency maintenance

Theme: reduce operational drift in local, CI, and dependency workflows.

1. [#223](https://github.com/WirelessCar/nauth/issues/223) `Consolidate repo workflows by making Mise the canonical task runner`
   Dependency: no hard dependency; grouped by maintenance theme
   Suggested issue type: `type: feature`
   Note: Make remains the real entrypoint in CI and docs.
2. [#274](https://github.com/WirelessCar/nauth/issues/274) `Dependency Dashboard`
   Dependency: umbrella/meta issue
   Suggested issue type: `type: feature`
   Note: Keep it open as a Renovate dashboard, not as a normal implementation ticket.

### 6. NACK integration

Theme: package JetStream controller credentials for account consumers.

1. [#10](https://github.com/WirelessCar/nauth/issues/10) `Support creation of NACK user in NAuth`
   Dependency: no hard dependency; grouped by theme
   Suggested issue type: `type: feature`
   Note: ADR-3 already accepts the direction, but no controller support exists.

## Notable findings

- The open issue set changed from 17 to 21. Closed issues from the previous audit are removed: `#27`, `#144`, `#178`, and `#184`.
- The previous audit was wrong for `#11` now: `AccountExport` and `AccountImport` CRDs exist, with controllers and generated chart CRDs.
- The clearest current bugs/gaps are `#43`, `#95`, and `#235`.
- `#59`, `#102`, `#138`, `#140`, and `#228` are not dead backlog, but their text or scope needs maintainer cleanup before implementation.
- Several open issues use `type: maintenance`, but `docs/triage.md` only defines `type: bug`, `type: feature`, `type: docs`, and `type: question`. This audit maps maintenance/refactor work to `type: feature` to follow the current triage guide.
