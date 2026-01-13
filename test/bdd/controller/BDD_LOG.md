# BDD conversion log

## Converted tests

### Account controller
Source: `internal/controller/account_test.go`

| Original test | Feature scenario |
| --- | --- |
| should successfully reconcile the account | `test/bdd/controller/account_controller.feature` — Create account successfully |
| should fail to reconcile the account | `test/bdd/controller/account_controller.feature` — Create account fails |
| should not remove account from manager in observe mode | `test/bdd/controller/account_controller.feature` — Observe mode does not delete managed resources |
| should successfully remove the account marked for deletion | `test/bdd/controller/account_controller.feature` — Delete account successfully |
| should fail to remove the account when delete client fails | `test/bdd/controller/account_controller.feature` — Delete account fails |
| should import account in observe mode | `test/bdd/controller/account_controller.feature` — Observe mode import succeeds |
| should successfully reconcile the account when the operator version change | `test/bdd/controller/account_controller.feature` — Update account when operator version changes |

### User controller
Source: `internal/controller/user_test.go`

| Original test | Feature scenario |
| --- | --- |
| should successfully reconcile the user | `test/bdd/controller/user_controller.feature` — Create or update user successfully |
| should fails when trying to create a new user without a valid account | `test/bdd/controller/user_controller.feature` — Fail to create user without a valid account |
| should successfully remove the user marked for deletion | `test/bdd/controller/user_controller.feature` — Delete user successfully |
| should fail to remove the user when delete client fails | `test/bdd/controller/user_controller.feature` — Delete user fails |
| should successfully reconcile the user (update reconciliation context) | `test/bdd/controller/user_controller.feature` — Update user when operator version changes |

## New tests
- `test/bdd/controller/account_controller.feature` — Delete account is blocked when users still exist
- `test/bdd/controller/account_controller.feature` — Reconcile request for a missing account
- `test/bdd/controller/account_controller.feature` — No-op reconcile when nothing has changed
- `test/bdd/controller/account_controller.feature` — Finalizer is added on first reconcile
- `test/bdd/controller/user_controller.feature` — Reconcile request for a missing user
- `test/bdd/controller/user_controller.feature` — No-op reconcile when nothing has changed
- `test/bdd/controller/user_controller.feature` — Finalizer is added on first reconcile

## Feasibility notes
- The features map to existing envtest-based controller tests; step definitions can reuse the fake recorder, fake k8s client, and Ginkgo setup without exposing implementation details.
- Event assertions are feasible because the tests already read from `record.FakeRecorder` and validate condition status.
